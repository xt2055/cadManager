package converter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/cadtext"
	"cadguanliq/internal/storage"
)

type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 1
	PriorityHigh   Priority = 2
)

type Job struct {
	Attachment attachment.Attachment
	Priority   Priority
	mu         sync.Mutex
	waiters    []chan error
}

func (j *Job) addWaiter(ch chan error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.waiters = append(j.waiters, ch)
}

func (j *Job) broadcast(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, ch := range j.waiters {
		select {
		case ch <- err:
		default:
		}
	}
}

type Service struct {
	repo       attachment.Repository
	storage    storage.ObjectStorage
	dwg2dxfBin string
	caxaBin    string
	caxaMu     sync.Mutex
	flightMu   sync.Mutex
	highQueue  chan *Job
	normQueue  chan *Job
	lowQueue   chan *Job
	inFlight   sync.Map
	stopChan   chan struct{}
}

func NewService(repo attachment.Repository, objStorage storage.ObjectStorage, tools ...string) *Service {
	bin := "./tools/exb2dxf/ok/dwg2dxf.exe"
	caxaBin := ""
	if len(tools) > 0 && strings.TrimSpace(tools[0]) != "" {
		bin = strings.TrimSpace(tools[0])
	}
	if len(tools) > 1 {
		caxaBin = strings.TrimSpace(tools[1])
	}
	return &Service{
		repo:       repo,
		storage:    objStorage,
		dwg2dxfBin: bin,
		caxaBin:    caxaBin,
		highQueue:  make(chan *Job, 100),
		normQueue:  make(chan *Job, 500),
		lowQueue:   make(chan *Job, 2000),
		stopChan:   make(chan struct{}),
	}
}

func (s *Service) Start(ctx context.Context) {
	log.Println("[CAD Converter] 转换服务已启动...")
	go s.workerLoop(ctx)
	go s.cronScanner(ctx)
}

func (s *Service) Stop() {
	close(s.stopChan)
}

// PushJob 加入转换队列并返回可等待的 channel
func (s *Service) PushJob(att attachment.Attachment, p Priority) <-chan error {
	done := make(chan error, 1)
	key := att.StorageKey

	s.flightMu.Lock()
	defer s.flightMu.Unlock()

	if existing, loaded := s.inFlight.Load(key); loaded {
		if job, ok := existing.(*Job); ok {
			job.addWaiter(done)
			return done
		}
	}

	job := &Job{
		Attachment: att,
		Priority:   p,
		waiters:    []chan error{done},
	}

	s.inFlight.Store(key, job)

	switch p {
	case PriorityHigh:
		select {
		case s.highQueue <- job:
		default:
			s.normQueue <- job
		}
	case PriorityNormal:
		select {
		case s.normQueue <- job:
		default:
			s.lowQueue <- job
		}
	case PriorityLow:
		select {
		case s.lowQueue <- job:
		default:
		}
	}
	return done
}

func (s *Service) workerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		var job *Job
		select {
		case job = <-s.highQueue:
		default:
			select {
			case job = <-s.highQueue:
			case job = <-s.normQueue:
			default:
				select {
				case job = <-s.highQueue:
				case job = <-s.normQueue:
				case job = <-s.lowQueue:
				case <-time.After(500 * time.Millisecond):
					continue
				}
			}
		}

		if job != nil {
			err := s.processOne(ctx, job.Attachment)
			s.flightMu.Lock()
			s.inFlight.Delete(job.Attachment.StorageKey)
			job.broadcast(err)
			s.flightMu.Unlock()
		}
	}
}

func (s *Service) processOne(ctx context.Context, att attachment.Attachment) error {
	ext := filepathExt(att.StorageKey)
	if ext == "" {
		ext = filepathExt(att.Name)
	}

	// 当前转换队列只负责生成 MLightCAD 使用的 DWG，不再以 DXF 是否存在作为完成条件。
	dwgKey := strings.TrimSuffix(att.StorageKey, ext) + ".dwg"
	if reader, info, err := s.storage.Open(ctx, dwgKey); err == nil {
		reader.Close()
		if info.Size > 0 {
			return nil
		}
		_ = s.storage.Delete(ctx, dwgKey)
	}

	// 已废弃：旧队列曾检查并生成 DXF，代码保留以便后续回溯。
	// dxfKey := strings.TrimSuffix(att.StorageKey, ext) + ".dxf"
	// if reader, info, err := s.storage.Open(ctx, dxfKey); err == nil {
	// 	reader.Close()
	// 	if info.Size > 0 && !strings.EqualFold(ext, ".exb") {
	// 		return nil
	// 	}
	// 	_ = s.storage.Delete(ctx, dxfKey)
	// }
	// if strings.EqualFold(ext, ".dxf") {
	// 	return errors.New("DXF 文件为空，无法生成预览")
	// }

	tempDir := os.TempDir()
	nowNano := time.Now().UnixNano()
	tempDwg := filepath.Join(tempDir, fmt.Sprintf("caxa_out_%d.dwg", nowNano))
	defer os.Remove(tempDwg)
	defer os.Remove(tempDwg + ".done")
	// 已废弃：DXF 临时文件仅由旧转换流程使用，现已停用。
	// tempDxf := filepath.Join(tempDir, fmt.Sprintf("caxa_out_%d.dxf", nowNano))
	// defer os.Remove(tempDxf)

	if strings.EqualFold(ext, ".exb") {
		if err := s.ensureCaxaRunning(ctx); err != nil {
			return err
		}

		reader, _, err := s.storage.Open(ctx, att.StorageKey)
		if err != nil {
			return fmt.Errorf("读取原始 EXB 失败: %w", err)
		}
		defer reader.Close()

		tempExb := filepath.Join(tempDir, fmt.Sprintf("caxa_in_%d.exb", nowNano))
		defer os.Remove(tempExb)

		outFile, err := os.Create(tempExb)
		if err != nil {
			return err
		}
		if _, err := ioCopy(outFile, reader); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()

		jobFile := filepath.Join(tempDir, "caxa_exb_jobs.txt")
		if err := waitForJobFileFree(jobFile, 15*time.Second); err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%s\n", tempExb, tempDwg)
		if err := os.WriteFile(jobFile, []byte(line), 0o644); err != nil {
			return fmt.Errorf("写入 CAXA 调度任务失败: %w", err)
		}

		doneFile := tempDwg + ".done"
		success := false
		for i := 0; i < 120; i++ {
			time.Sleep(500 * time.Millisecond)
			if _, err := os.Stat(doneFile); err == nil {
				statusBytes, _ := os.ReadFile(doneFile)
				if strings.TrimSpace(string(statusBytes)) == "OK" {
					success = true
				}
				break
			}
		}

		if !success {
			return errors.New("CAXA 转换超时或返回失败")
		}

		dwgKey := strings.TrimSuffix(att.StorageKey, ext) + ".dwg"
		dwgReader, err := os.Open(tempDwg)
		if err != nil {
			return fmt.Errorf("打开生成 DWG 失败: %w", err)
		}
		_, putErr := s.storage.Put(ctx, dwgKey, dwgReader, "application/acad")
		dwgReader.Close()
		if putErr != nil {
			return fmt.Errorf("保存 DWG 附件失败: %w", putErr)
		}
	} else if strings.EqualFold(ext, ".dwg") {
		reader, _, err := s.storage.Open(ctx, att.StorageKey)
		if err != nil {
			return fmt.Errorf("读取原始 DWG 失败: %w", err)
		}
		defer reader.Close()

		outFile, err := os.Create(tempDwg)
		if err != nil {
			return err
		}
		if _, err := ioCopy(outFile, reader); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()
	} else {
		return nil
	}

	// 已废弃：转换队列不再执行 DWG -> DXF，也不再写入 DXF 对象。
	// if err := s.convertDwgToDxf(ctx, tempDwg, tempDxf); err != nil {
	// 	return fmt.Errorf("DWG 转 DXF 失败: %w", err)
	// }
	// dxfReader, err := os.Open(tempDxf)
	// if err != nil {
	// 	return fmt.Errorf("打开生成 DXF 失败: %w", err)
	// }
	// defer dxfReader.Close()
	// _, err = s.storage.Put(ctx, dxfKey, dxfReader, "application/dxf")
	// if err != nil {
	// 	return fmt.Errorf("保存 DXF 附件失败: %w", err)
	// }

	log.Printf("[CAD Converter] 成功生成 DWG: %s -> %s", att.StorageKey, dwgKey)
	return nil
}

// ConvertToExb 将 DWG 或 DXF 转换为 EXB 格式并存入对象存储，返回生成的 exbStorageKey
func (s *Service) ConvertToExb(ctx context.Context, att attachment.Attachment) (string, error) {
	ext := filepathExt(att.StorageKey)
	if ext == "" {
		ext = filepathExt(att.Name)
	}
	if strings.EqualFold(ext, ".exb") {
		return att.StorageKey, nil
	}
	if !strings.EqualFold(ext, ".dwg") && !strings.EqualFold(ext, ".dxf") {
		return "", fmt.Errorf("只支持 DWG/DXF 格式转换为 EXB: %s", att.Name)
	}

	if err := s.ensureCaxaRunning(ctx); err != nil {
		return "", err
	}

	tempDir := os.TempDir()
	nowNano := time.Now().UnixNano()
	tempIn := filepath.Join(tempDir, fmt.Sprintf("caxa_in_%d%s", nowNano, ext))
	tempExb := filepath.Join(tempDir, fmt.Sprintf("caxa_out_%d.exb", nowNano))
	defer os.Remove(tempIn)
	defer os.Remove(tempExb)
	defer os.Remove(tempExb + ".done")

	reader, _, err := s.storage.Open(ctx, att.StorageKey)
	if err != nil {
		return "", fmt.Errorf("读取源文件失败: %w", err)
	}
	defer reader.Close()

	inFile, err := os.Create(tempIn)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	if _, err := ioCopy(inFile, reader); err != nil {
		inFile.Close()
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	inFile.Close()
	if strings.EqualFold(ext, ".dxf") {
		if err := cadtext.NormalizeDxfFileForCaxa(tempIn); err != nil {
			return "", err
		}
	}

	if err := s.runCaxaJob(ctx, tempIn, tempExb); err != nil {
		return "", err
	}

	exbKey := strings.TrimSuffix(att.StorageKey, ext) + ".exb"
	exbReader, err := os.Open(tempExb)
	if err != nil {
		return "", fmt.Errorf("打开生成的 EXB 失败: %w", err)
	}
	defer exbReader.Close()

	_, putErr := s.storage.Put(ctx, exbKey, exbReader, "application/octet-stream")
	if putErr != nil {
		return "", fmt.Errorf("保存 EXB 文件失败: %w", putErr)
	}

	log.Printf("[CAD Converter] 成功转换并保存 EXB: %s -> %s", att.StorageKey, exbKey)
	return exbKey, nil
}

// ConvertPathToExb 将本地 DWG/DXF 文件转换为本地 EXB 文件，供上传前识别图纸内容使用。
func (s *Service) ConvertPathToExb(ctx context.Context, inputPath, outputPath string) error {
	if err := s.ensureCaxaRunning(ctx); err != nil {
		return err
	}
	return s.runCaxaJob(ctx, inputPath, outputPath)
}

// ConvertPathToDwg 将本地 DXF/DWG/EXB 文件转换为本地 DWG 文件。
// 在线编辑器在浏览器内只能产出 DXF，保存时由后端通过 CAXA 调度转换为 DWG 归档。
func (s *Service) ConvertPathToDwg(ctx context.Context, inputPath, outputPath string) error {
	if err := s.ensureCaxaRunning(ctx); err != nil {
		return err
	}
	return s.runCaxaJob(ctx, inputPath, outputPath)
}

// runCaxaJob 向 CAXA 调度器提交一条「输入路径|输出路径」任务并等待完成，
// 输出格式由输出文件的扩展名决定（.exb/.dwg 等）。
func (s *Service) runCaxaJob(ctx context.Context, inputPath, outputPath string) error {
	jobFile := filepath.Join(os.TempDir(), "caxa_exb_jobs.txt")
	if err := waitForJobFileFree(jobFile, 30*time.Second); err != nil {
		return err
	}
	line := fmt.Sprintf("%s|%s\n", inputPath, outputPath)
	if err := os.WriteFile(jobFile, []byte(line), 0o644); err != nil {
		return fmt.Errorf("写入 CAXA 调度任务失败: %w", err)
	}

	doneFile := outputPath + ".done"
	for i := 0; i < 180; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		time.Sleep(500 * time.Millisecond)
		if _, err := os.Stat(doneFile); err == nil {
			statusBytes, _ := os.ReadFile(doneFile)
			if strings.TrimSpace(string(statusBytes)) == "OK" {
				return nil
			}
			return errors.New("CAXA 转换任务失败")
		}
	}
	return errors.New("CAXA 转换任务超时")
}

// EnsureDwg 确保 EXB 已转换为可供浏览器 CAD 引擎读取的 DWG，并返回实际存储键。
func (s *Service) EnsureDwg(ctx context.Context, att attachment.Attachment) (string, error) {
	ext := filepathExt(att.StorageKey)
	if ext == "" {
		ext = filepathExt(att.Name)
	}
	if strings.EqualFold(ext, ".dwg") {
		return att.StorageKey, nil
	}
	if !strings.EqualFold(ext, ".exb") {
		return "", fmt.Errorf("文件格式不支持 DWG 渲染源: %s", att.Name)
	}

	dwgKey := strings.TrimSuffix(att.StorageKey, ext) + ".dwg"
	if reader, info, err := s.storage.Open(ctx, dwgKey); err == nil {
		reader.Close()
		if info.Size > 0 {
			return dwgKey, nil
		}
		_ = s.storage.Delete(ctx, dwgKey)
	}

	done := s.PushJob(att, PriorityHigh)
	select {
	case err := <-done:
		if err != nil {
			return "", err
		}
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(60 * time.Second):
		return "", errors.New("EXB 转换 DWG 超时")
	}

	reader, info, err := s.storage.Open(ctx, dwgKey)
	if err != nil {
		return "", fmt.Errorf("EXB 转换后未生成 DWG: %w", err)
	}
	reader.Close()
	if info.Size == 0 {
		return "", errors.New("EXB 转换后生成的 DWG 为空")
	}
	return dwgKey, nil
}

func waitForJobFileFree(path string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("CAXA 正在处理其他转换任务，任务队列等待超时: %s", path)
		case <-ticker.C:
		}
	}
}

// 已废弃：转换队列不再执行 DWG -> DXF。原实现保留为注释，便于回溯。
// func (s *Service) convertDwgToDxf(ctx context.Context, dwgPath, dxfPath string) error {
// 	binPath, err := resolveToolPath(s.dwg2dxfBin, "dwg2dxf.exe")
// 	if err != nil {
// 		return err
// 	}
// 	cmd := exec.CommandContext(ctx, binPath, "-y", "-o", dxfPath, dwgPath)
// 	cmd.Dir = filepath.Dir(binPath)
// 	output, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("%w: %s", err, string(output))
// 	}
// 	if info, err := os.Stat(dxfPath); err != nil || info.Size() == 0 {
// 		return fmt.Errorf("dwg2dxf 生成的文件为空或不存在: %s", string(output))
// 	}
// 	return nil
// }

func (s *Service) ensureCaxaRunning(ctx context.Context) error {
	s.caxaMu.Lock()
	defer s.caxaMu.Unlock()

	if isCaxaRunning() {
		return nil
	}

	binPath, err := ResolveCaxaPath(s.caxaBin)
	if err != nil {
		return err
	}

	log.Printf("[CAD Converter] CAXA 未运行，正在自动启动: %s", binPath)
	if err := exec.Command(binPath).Start(); err != nil {
		return fmt.Errorf("启动 CAXA 失败: %w", err)
	}

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if isCaxaRunning() {
			log.Println("[CAD Converter] CAXA 已启动，开始提交转换任务")
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 CAXA 启动被取消: %w", ctx.Err())
		case <-deadline.C:
			return errors.New("CAXA 启动超时，请检查 CAXA CAD 安装状态")
		case <-ticker.C:
		}
	}
}

func resolveToolPath(configured, name string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		for _, path := range candidatePaths(configured) {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("找不到 %s，请设置 CAD_DWG2DXF_BIN", name)
}

func ResolveCaxaPath(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		for _, path := range candidatePaths(configured) {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		return "", fmt.Errorf("找不到 CAD_CAXA_BIN 指定的 CAXA 程序: %s", configured)
	}
	if path, err := exec.LookPath("CDRAFT_M.exe"); err == nil {
		return path, nil
	}
	if path := caxaScanCache(); path != "" {
		return path, nil
	}
	return "", errors.New("找不到 CAXA CAD，请设置 CAD_CAXA_BIN")
}

// caxaScanCache 进程内缓存：磁盘与注册表扫描只执行一次，避免每次打开图纸重复全盘查找。
var caxaScanCache = sync.OnceValue(func() string {
	if path := scanCaxaRegistry(); path != "" {
		return path
	}
	return scanCaxaDiskInstalls()
})

// scanCaxaRegistry 通过注册表 Uninstall 键定位 CAXA CAD 安装位置（含 32 位程序视图），
// 覆盖非默认安装路径（自定义盘符/目录）的场景。
func scanCaxaRegistry() string {
	roots := []string{
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
		`HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	}
	best := ""
	for _, root := range roots {
		listing, err := exec.Command("reg", "query", root, "/s", "/f", "CAXA", "/d").CombinedOutput()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(listing), "\r\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "HKEY_") {
				continue
			}
			detail, err := exec.Command("reg", "query", line, "/v", "InstallLocation").CombinedOutput()
			if err != nil {
				continue
			}
			installDir := regStringValue(string(detail))
			if installDir == "" {
				continue
			}
			for _, match := range globCaxaBin(installDir) {
				if best == "" || slices.Compare(numericSegments(best), numericSegments(match)) < 0 {
					best = match
				}
			}
		}
	}
	return best
}

// regStringValue 从 reg query 输出中提取 REG_SZ 字符串值。
func regStringValue(output string) string {
	for _, line := range strings.Split(output, "\r\n") {
		if idx := strings.Index(line, "REG_SZ"); idx >= 0 {
			return strings.TrimSpace(line[idx+len("REG_SZ"):])
		}
	}
	return ""
}

// globCaxaBin 在安装目录下按有限深度查找 Bin64\CDRAFT_M.exe。
func globCaxaBin(installDir string) []string {
	var matches []string
	for depth := 0; depth <= 3; depth++ {
		segments := make([]string, depth)
		for i := range segments {
			segments[i] = "*"
		}
		pattern := filepath.Join(append([]string{installDir}, append(segments, "Bin64", "CDRAFT_M.exe")...)...)
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	return matches
}

// scanCaxaDiskInstalls 兜底磁盘扫描：Program Files（含 64/32 位视图）与常见盘符根目录下的 CAXA 目录，
// 返回版本号最新的安装（如 CAXA\CAXA CAD\2022\Bin64\CDRAFT_M.exe）。
func scanCaxaDiskInstalls() string {
	roots := []string{
		os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles(x86)"),
		`C:\`, `D:\`, `E:\`, `F:\`,
	}
	best := ""
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		caxaRoot := filepath.Join(root, "CAXA")
		if _, err := os.Stat(caxaRoot); err != nil {
			continue
		}
		for _, match := range globCaxaBin(caxaRoot) {
			if best == "" || slices.Compare(numericSegments(best), numericSegments(match)) < 0 {
				best = match
			}
		}
	}
	return best
}

// numericSegments extracts the digit runs from a path so install versions can
// be compared numerically (e.g. "...\CAXA CAD\2022\Bin64" -> [2022, 64]).
func numericSegments(path string) []int {
	fields := strings.FieldsFunc(path, func(r rune) bool { return !unicode.IsDigit(r) })
	segs := make([]int, 0, len(fields))
	for _, field := range fields {
		if n, err := strconv.Atoi(field); err == nil {
			segs = append(segs, n)
		}
	}
	return segs
}

func candidatePaths(configured string) []string {
	if filepath.IsAbs(configured) {
		return []string{configured}
	}

	paths := make([]string, 0, 4)
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, configured))
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		paths = append(paths,
			filepath.Join(executableDir, configured),
			filepath.Join(executableDir, "..", configured),
			filepath.Join(executableDir, "..", "..", configured),
		)
	}
	return paths
}

func isCaxaRunning() bool {
	output, err := exec.Command("tasklist", "/FI", "IMAGENAME eq CDRAFT_M.exe", "/FO", "CSV", "/NH").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(output)), "cdraft_m.exe")
}

func filepathExt(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	index := strings.LastIndexByte(value, '.')
	if index < 0 {
		return ""
	}
	return value[index:]
}

func (s *Service) cronScanner(ctx context.Context) {
	// 启动后先立即执行一次扫描
	s.scanMissingDwg(ctx)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.scanMissingDwg(ctx)
		}
	}
}

func (s *Service) scanMissingDwg(ctx context.Context) {
	list, err := s.repo.ListAllCad(ctx)
	if err != nil {
		log.Printf("[CAD Converter] 定时扫描 DWG 列表失败: %v", err)
		return
	}

	for _, att := range list {
		ext := filepathExt(att.StorageKey)
		if ext == "" {
			ext = filepathExt(att.Name)
		}
		if !strings.EqualFold(ext, ".exb") {
			continue
		}
		dwgKey := strings.TrimSuffix(att.StorageKey, ext) + ".dwg"
		if _, _, err := s.storage.Open(ctx, dwgKey); err != nil {
			// DWG 还不存在，加入低优先级后台转换队列
			s.PushJob(att, PriorityLow)
		}
	}
}

// scanMissingDxf 保留旧版 DXF 扫描逻辑，仅作为迁移参考，不再执行。
// func (s *Service) scanMissingDxf(ctx context.Context) {
// 	list, err := s.repo.ListAllCad(ctx)
// 	if err != nil {
// 		log.Printf("[CAD Converter] 定时扫描 CAD 列表失败: %v", err)
// 		return
// 	}
// 	for _, att := range list {
// 		ext := filepathExt(att.StorageKey)
// 		if ext == "" {
// 			ext = filepathExt(att.Name)
// 		}
// 		if strings.EqualFold(ext, ".dxf") {
// 			continue
// 		}
// 		dxfKey := strings.TrimSuffix(att.StorageKey, ext) + ".dxf"
// 		if _, _, err := s.storage.Open(ctx, dxfKey); err != nil {
// 			s.PushJob(att, PriorityLow)
// 		}
// 	}
// }

func ioCopy(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			nw, ew := dst.Write(buf[:n])
			total += int64(nw)
			if ew != nil {
				return total, ew
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return total, err
		}
	}
	return total, nil
}

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
	// caxaJobMu serializes every caller of CAXA's shared file-based protocol.
	// Some synchronous operations do not go through workerLoop.
	caxaJobMu sync.Mutex
	flightMu  sync.Mutex
	highQueue chan *Job
	normQueue chan *Job
	lowQueue  chan *Job
	inFlight  sync.Map
	stopChan  chan struct{}
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

// CaxaBin 返回 .env 配置的 CAXA 程序路径（可能为空），供编辑会话向客户端
// 提供可选提示；客户端启动 CAXA 与服务端无关，不会因此阻塞。
func (s *Service) CaxaBin() string {
	return s.caxaBin
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

// versionDwgKey 计算附件 v1.0 初始版本的目标存储键：
// {目录}/history/{文件名去后缀}/v1.0/{文件名去后缀}.dwg
// 转换产物直接进入版本目录，不再堆放在原始文件旁边，原始文件永不被覆盖。
func versionDwgKey(att attachment.Attachment) string {
	folder := filepath.ToSlash(filepath.Dir(att.StorageKey))
	base := filepath.Base(filepath.ToSlash(att.StorageKey))
	ext := filepathExt(base)
	baseNoExt := strings.TrimSuffix(base, ext)
	return filepath.ToSlash(filepath.Join(folder, "history", baseNoExt, "v1.0", baseNoExt+".dwg"))
}

func (s *Service) processOne(ctx context.Context, att attachment.Attachment) error {
	ext := filepathExt(att.StorageKey)
	if ext == "" {
		ext = filepathExt(att.Name)
	}
	if !strings.EqualFold(ext, ".exb") && !strings.EqualFold(ext, ".dwg") && !strings.EqualFold(ext, ".dxf") {
		return nil
	}

	// 目标产物：v1.0 版本目录内的 DWG。已存在且非空则任务完成。
	dwgKey := versionDwgKey(att)
	if reader, info, err := s.storage.Open(ctx, dwgKey); err == nil {
		reader.Close()
		if info.Size > 0 {
			return nil
		}
		_ = s.storage.Delete(ctx, dwgKey)
	}

	tempDir := os.TempDir()
	nowNano := time.Now().UnixNano()
	tempDwg := filepath.Join(tempDir, fmt.Sprintf("caxa_out_%d.dwg", nowNano))
	defer removeCaxaTempFiles(tempDwg, tempDwg+".done")

	switch {
	case strings.EqualFold(ext, ".exb"):
		if err := s.ensureCaxaRunning(ctx); err != nil {
			return err
		}

		reader, _, err := s.storage.Open(ctx, att.StorageKey)
		if err != nil {
			return fmt.Errorf("读取原始 EXB 失败: %w", err)
		}
		defer reader.Close()

		tempExb := filepath.Join(tempDir, fmt.Sprintf("caxa_in_%d.exb", nowNano))
		defer removeCaxaTempFiles(tempExb)

		outFile, err := os.Create(tempExb)
		if err != nil {
			return err
		}
		if _, err := ioCopy(outFile, reader); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()

		if err := s.runCaxaJob(ctx, tempExb, tempDwg); err != nil {
			return err
		}

		dwgReader, err := os.Open(tempDwg)
		if err != nil {
			return fmt.Errorf("打开生成 DWG 失败: %w", err)
		}
		_, putErr := s.storage.Put(ctx, dwgKey, dwgReader, "application/acad")
		dwgReader.Close()
		if putErr != nil {
			return fmt.Errorf("保存 DWG 附件失败: %w", putErr)
		}
	case strings.EqualFold(ext, ".dxf"):
		if err := s.ensureCaxaRunning(ctx); err != nil {
			return err
		}

		reader, _, err := s.storage.Open(ctx, att.StorageKey)
		if err != nil {
			return fmt.Errorf("读取原始 DXF 失败: %w", err)
		}
		defer reader.Close()

		tempDxf := filepath.Join(tempDir, fmt.Sprintf("caxa_in_%d.dxf", nowNano))
		defer removeCaxaTempFiles(tempDxf)

		outFile, err := os.Create(tempDxf)
		if err != nil {
			return err
		}
		if _, err := ioCopy(outFile, reader); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()
		if err := cadtext.NormalizeDxfFileForCaxa(tempDxf); err != nil {
			return err
		}

		if err := s.runCaxaJob(ctx, tempDxf, tempDwg); err != nil {
			return err
		}

		dwgReader, err := os.Open(tempDwg)
		if err != nil {
			return fmt.Errorf("打开生成 DWG 失败: %w", err)
		}
		_, putErr := s.storage.Put(ctx, dwgKey, dwgReader, "application/acad")
		dwgReader.Close()
		if putErr != nil {
			return fmt.Errorf("保存 DWG 附件失败: %w", putErr)
		}
	default:
		// DWG 本身已是目标格式：复制一份到 v1.0 版本目录登记为初始版本。
		reader, _, err := s.storage.Open(ctx, att.StorageKey)
		if err != nil {
			return fmt.Errorf("读取原始 DWG 失败: %w", err)
		}
		defer reader.Close()

		if _, err := s.storage.Put(ctx, dwgKey, reader, "application/acad"); err != nil {
			return fmt.Errorf("复制 DWG 到版本目录失败: %w", err)
		}
	}

	log.Printf("[CAD Converter] 成功生成 v1.0 版本 DWG: %s -> %s", att.StorageKey, dwgKey)
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
	defer removeCaxaTempFiles(tempIn, tempExb, tempExb+".done")

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
	s.caxaJobMu.Lock()
	defer s.caxaJobMu.Unlock()

	jobFile := filepath.Join(os.TempDir(), "caxa_exb_jobs.txt")
	if err := waitForCaxaJobFile(jobFile, 10*time.Second); err != nil {
		return err
	}

	doneFile := outputPath + ".done"
	if err := removeCaxaDoneSignal(doneFile); err != nil {
		return err
	}
	if err := publishCaxaJob(jobFile, inputPath, outputPath); err != nil {
		return err
	}

	nudged := false
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
		// CAXA 已启动但停在空界面时插件不消费任务：10 秒后补开一次哨兵图纸激活插件。
		if !nudged && i == 20 {
			nudged = true
			s.nudgeSentinel(ctx)
		}
	}
	return errors.New("CAXA 转换任务超时")
}

// EnsureDwg 确保 EXB/DXF/DWG 在 v1.0 版本目录中有对应的 DWG，并返回该版本存储键。
func (s *Service) EnsureDwg(ctx context.Context, att attachment.Attachment) (string, error) {
	ext := filepathExt(att.StorageKey)
	if ext == "" {
		ext = filepathExt(att.Name)
	}
	if !strings.EqualFold(ext, ".exb") && !strings.EqualFold(ext, ".dwg") && !strings.EqualFold(ext, ".dxf") {
		return "", fmt.Errorf("文件格式不支持 DWG 渲染源: %s", att.Name)
	}

	dwgKey := versionDwgKey(att)
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

const caxaJobStaleAfter = 5 * time.Second

// waitForCaxaJobFile waits for the plugin to claim its shared dispatch file.
// A file that survives this long belongs to a failed earlier dispatch, not an
// active request in this process, because runCaxaJob holds caxaJobMu.
func waitForCaxaJobFile(path string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("检查 CAXA 调度任务失败: %w", err)
		}
		if time.Since(info.ModTime()) >= caxaJobStaleAfter {
			stalePath := fmt.Sprintf("%s.stale.%d", path, time.Now().UnixNano())
			if err := os.Rename(path, stalePath); err == nil {
				log.Printf("[CAD Converter] 隔离未被 CAXA 消费的遗留任务: %s -> %s", path, stalePath)
				return nil
			} else if os.IsNotExist(err) {
				continue
			} else {
				lastErr = err
			}
		}
		select {
		case <-deadline.C:
			if lastErr != nil {
				return fmt.Errorf("CAXA 调度任务文件仍被占用，请处理 CAXA 弹窗或重启 CAXA 后重试: %w", lastErr)
			}
			return fmt.Errorf("CAXA 正在接收上一项转换任务，队列等待超时: %s", path)
		case <-ticker.C:
		}
	}
}

// publishCaxaJob writes a complete task beside the watched file then renames
// it into place, so the plugin never observes a partially-written task.
func publishCaxaJob(jobFile, inputPath, outputPath string) error {
	pendingFile := fmt.Sprintf("%s.pending.%d", jobFile, time.Now().UnixNano())
	line := fmt.Sprintf("%s|%s\n", inputPath, outputPath)
	if err := os.WriteFile(pendingFile, []byte(line), 0o644); err != nil {
		return fmt.Errorf("写入 CAXA 调度任务失败: %w", err)
	}
	if err := os.Rename(pendingFile, jobFile); err != nil {
		_ = os.Remove(pendingFile)
		return fmt.Errorf("提交 CAXA 调度任务失败: %w", err)
	}
	return nil
}

func removeCaxaDoneSignal(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理上次 CAXA 完成标记失败（文件可能仍被占用）: %w", err)
	}
	return nil
}

// Windows can retain a CAXA document handle briefly after its .done marker is
// created. Retry cleanup so stale caxa_in_* files do not build up indefinitely.
func removeCaxaTempFiles(paths ...string) {
	pending := append([]string(nil), paths...)
	for attempt := 0; len(pending) > 0 && attempt < 5; attempt++ {
		next := pending[:0]
		for _, path := range pending {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				next = append(next, path)
			}
		}
		pending = next
		if len(pending) > 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	for _, path := range pending {
		log.Printf("[CAD Converter] 临时文件仍被占用，稍后可安全删除: %s", path)
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
	// CAXA 插件只在打开图纸后才消费转换任务文件，启动时必须同时打开一张哨兵图纸，
	// 否则任务会一直无人处理，直到 60 秒转换超时。
	sentinel := s.ensureSentinelDrawing(ctx)
	var startErr error
	if sentinel != "" {
		log.Printf("[CAD Converter] 随 CAXA 打开哨兵图纸以激活转换插件: %s", sentinel)
		startErr = exec.Command(binPath, sentinel).Start()
	} else {
		startErr = exec.Command(binPath).Start()
	}
	if startErr != nil {
		return fmt.Errorf("启动 CAXA 失败: %w", startErr)
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

// ensureSentinelDrawing 从对象存储挑一个最小的 DWG（无 DWG 则 EXB）复制为哨兵图纸，
// 供自动启动 CAXA 时打开以激活转换插件。文件缓存在临时目录，可重复使用。
func (s *Service) ensureSentinelDrawing(ctx context.Context) string {
	for _, ext := range []string{".dwg", ".exb"} {
		sentinel := filepath.Join(os.TempDir(), "caxa_sentinel"+ext)
		if info, err := os.Stat(sentinel); err == nil && info.Size() > 0 {
			return sentinel
		}
	}

	list, err := s.repo.ListAllCad(ctx)
	if err != nil || len(list) == 0 {
		return ""
	}
	var best attachment.Attachment
	found := false
	for _, ext := range []string{".dwg", ".exb"} {
		for _, att := range list {
			attExt := filepathExt(att.StorageKey)
			if attExt == "" {
				attExt = filepathExt(att.Name)
			}
			if !strings.EqualFold(attExt, ext) {
				continue
			}
			if !found || att.Size < best.Size {
				best = att
				found = true
			}
		}
		if found {
			break
		}
	}
	if !found {
		return ""
	}
	ext := strings.ToLower(filepathExt(best.StorageKey))
	if ext == "" {
		ext = strings.ToLower(filepathExt(best.Name))
	}
	sentinel := filepath.Join(os.TempDir(), "caxa_sentinel"+ext)
	reader, _, err := s.storage.Open(ctx, best.StorageKey)
	if err != nil {
		return ""
	}
	defer reader.Close()
	file, err := os.Create(sentinel)
	if err != nil {
		return ""
	}
	if _, err := ioCopy(file, reader); err != nil {
		file.Close()
		_ = os.Remove(sentinel)
		return ""
	}
	file.Close()
	return sentinel
}

// nudgeSentinel 让已运行的 CAXA 再打开一次哨兵图纸：插件只在打开图纸后消费转换任务。
// 注意不能因为 isCaxaRunning() 为 true 就跳过——CAXA 常见停在空界面（进程在、插件未激活），
// 此时恰恰需要补开哨兵图纸，否则任务永远无人处理。
func (s *Service) nudgeSentinel(ctx context.Context) {
	binPath, err := ResolveCaxaPath(s.caxaBin)
	if err != nil {
		return
	}
	sentinel := s.ensureSentinelDrawing(ctx)
	if sentinel == "" {
		return
	}
	log.Printf("[CAD Converter] 转换任务迟迟未完成，尝试打开哨兵图纸激活插件: %s", sentinel)
	_ = exec.Command(binPath, sentinel).Start()
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
		log.Printf("[CAD Converter] 定时扫描 CAD 列表失败: %v", err)
		return
	}

	for _, att := range list {
		ext := filepathExt(att.StorageKey)
		if ext == "" {
			ext = filepathExt(att.Name)
		}
		if !strings.EqualFold(ext, ".exb") && !strings.EqualFold(ext, ".dwg") && !strings.EqualFold(ext, ".dxf") {
			continue
		}
		// 扫描依据：附件是否缺少 v1.0 版本目录文件（初始版本 DWG），
		// 缺少则进入低优先级后台队列补建；不再扫描“原始文件旁边有没有 DWG”。
		dwgKey := versionDwgKey(att)
		if _, _, err := s.storage.Open(ctx, dwgKey); err != nil {
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

package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	ringCapacity   = 1000
	rotateSize     = 10 * 1024 * 1024
	fileTimeLayout = "20060102"
	stdTimeLayout  = "2006/01/02 15:04:05"
)

var (
	mu          sync.Mutex
	ring        []entry
	ringCursor  int
	ringFilled  bool
	logDir      string
	fileHandle  *os.File
	fileWritten int64
	fileStamp   string
	fileSerial  int
	backupStderr io.Writer
)

type entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type LogLine struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type LogFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

// Setup 初始化全局日志：接管标准库 log 输出到 stderr + 内存环形缓冲 + 按天/大小轮转文件。
func Setup(dir string) error {
	mu.Lock()
	defer mu.Unlock()
	if dir == "" {
		dir = "./logs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	logDir = dir
	if ring == nil {
		ring = make([]entry, ringCapacity)
	}
	if backupStderr == nil {
		backupStderr = os.Stderr
		log.SetOutput(&writer{})
		log.SetFlags(0) // 时间由 writer 自行记录，避免二次解析
	}
	return nil
}

func Infof(format string, args ...any)  { log.Printf("[INFO]  %s", fmt.Sprintf(format, args...)) }
func Warnf(format string, args ...any)  { log.Printf("[WARN]  %s", fmt.Sprintf(format, args...)) }
func Errorf(format string, args ...any) { log.Printf("[ERROR] %s", fmt.Sprintf(format, args...)) }

type writer struct{}

func (instance *writer) Write(p []byte) (int, error) {
	text := strings.TrimRight(string(p), "\n")
	now := time.Now()
	level := "INFO"
	message := text
	if trimmed := strings.TrimSpace(text); strings.HasPrefix(trimmed, "[ERROR]") {
		level, message = "ERROR", strings.TrimSpace(strings.TrimPrefix(trimmed, "[ERROR]"))
	} else if strings.HasPrefix(trimmed, "[WARN]") {
		level, message = "WARN", strings.TrimSpace(strings.TrimPrefix(trimmed, "[WARN]"))
	}

	mu.Lock()
	defer mu.Unlock()
	remember(now, level, message)
	line := fmt.Sprintf("%s %s", now.Format(stdTimeLayout), text)
	if backupStderr != nil {
		fmt.Fprintln(backupStderr, line)
	}
	writeFile(now, line)
	return len(p), nil
}

func remember(now time.Time, level string, message string) {
	ring[ringCursor] = entry{Time: now, Level: level, Message: message}
	ringCursor++
	if ringCursor >= ringCapacity {
		ringCursor = 0
		ringFilled = true
	}
}

func writeFile(now time.Time, line string) {
	if logDir == "" {
		return
	}
	stamp := now.Format(fileTimeLayout)
	if fileHandle == nil || stamp != fileStamp || fileWritten >= rotateSize {
		rotateLocked(stamp)
	}
	if fileHandle == nil {
		return
	}
	if count, err := fileHandle.WriteString(line + "\n"); err == nil {
		fileWritten += int64(count)
	}
}

func rotateLocked(stamp string) {
	if fileHandle != nil {
		_ = fileHandle.Close()
		fileHandle = nil
	}
	if stamp != fileStamp {
		fileStamp = stamp
		fileSerial = 0
	} else {
		fileSerial++
	}
	name := fmt.Sprintf("cadguanliq-%s.log", stamp)
	if fileSerial > 0 {
		name = fmt.Sprintf("cadguanliq-%s-%d.log", stamp, fileSerial)
	}
	handle, err := os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	if info, err := handle.Stat(); err == nil {
		fileWritten = info.Size()
		if fileWritten >= rotateSize {
			_ = handle.Close()
			fileSerial++
			name = fmt.Sprintf("cadguanliq-%s-%d.log", stamp, fileSerial)
			handle, err = os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return
			}
			fileWritten = 0
		}
	}
	fileHandle = handle
}

// Query 返回内存缓冲中最近的日志（可按关键字过滤），时间正序。
func Query(lines int, keyword string) []LogLine {
	mu.Lock()
	defer mu.Unlock()
	if ring == nil {
		return []LogLine{}
	}
	if lines <= 0 || lines > ringCapacity {
		lines = ringCapacity
	}
	snapshot := make([]entry, 0, ringCapacity)
	if ringFilled {
		snapshot = append(snapshot, ring[ringCursor:]...)
	}
	snapshot = append(snapshot, ring[:ringCursor]...)
	result := make([]LogLine, 0, lines)
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	for index := len(snapshot) - 1; index >= 0 && len(result) < lines; index-- {
		item := snapshot[index]
		if keyword != "" && !strings.Contains(strings.ToLower(item.Message), keyword) {
			continue
		}
		result = append(result, LogLine{
			Time:    item.Time.Format("2006-01-02 15:04:05"),
			Level:   item.Level,
			Message: item.Message,
		})
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

// Files 列出日志目录中的日志文件（新文件在前）。
func Files() []LogFile {
	mu.Lock()
	dir := logDir
	mu.Unlock()
	if dir == "" {
		return []LogFile{}
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return []LogFile{}
	}
	result := make([]LogFile, 0, len(items))
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".log") {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		result = append(result, LogFile{
			Name:    item.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Name > result[right].Name
	})
	return result
}

// OpenFile 返回指定日志文件的只读句柄（校验文件名，防目录穿越）。
func OpenFile(name string) (*os.File, error) {
	mu.Lock()
	dir := logDir
	mu.Unlock()
	if dir == "" || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, ".log") {
		return nil, fmt.Errorf("非法日志文件名")
	}
	return os.Open(filepath.Join(dir, name))
}

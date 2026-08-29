package exb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cadguanliq/internal/pythonenv"
)

var ErrInvalidFile = errors.New("无效的 EXB 文件")

type Result struct {
	Format     string            `json:"format"`
	TitleBlock map[string]string `json:"titleBlock"`
}

type pythonOutput struct {
	Format string `json:"format"`
	Fields struct {
		TitleBlock map[string]string `json:"title_block"`
	} `json:"fields"`
}

type Parser struct {
	scriptPath string
}

func NewParser(scriptPath string) *Parser {
	if strings.TrimSpace(scriptPath) == "" {
		scriptPath = findScriptPath()
	}
	return &Parser{scriptPath: scriptPath}
}

func Parse(data []byte) (Result, error) {
	tempFile, err := os.CreateTemp("", "cadguanliq-parse-*.exb")
	if err != nil {
		return Result{}, fmt.Errorf("创建临时 EXB 文件失败: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return Result{}, fmt.Errorf("写入临时 EXB 文件失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return Result{}, fmt.Errorf("关闭临时 EXB 文件失败: %w", err)
	}

	return NewParser("").ParseFile(context.Background(), tempFile.Name())
}

func (parser *Parser) ParseFile(ctx context.Context, filePath string) (Result, error) {
	absolutePath, err := filepath.Abs(filePath)
	if err != nil {
		return Result{}, fmt.Errorf("解析 EXB 路径失败: %w", err)
	}
	if _, err := os.Stat(absolutePath); err != nil {
		return Result{}, fmt.Errorf("EXB 文件不存在: %w", err)
	}

	scriptPath := parser.scriptPath
	if scriptPath == "" {
		scriptPath = findScriptPath()
	}
	scriptAbsolute, err := filepath.Abs(scriptPath)
	if err != nil {
		return Result{}, fmt.Errorf("解析 Python 探针路径失败: %w", err)
	}
	if _, err := os.Stat(scriptAbsolute); err != nil {
		return Result{}, fmt.Errorf("Python 探针文件不存在: %s", scriptAbsolute)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := buildCommand(ctx, scriptAbsolute, absolutePath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return Result{}, fmt.Errorf("执行 EXB 解析失败: %s", errText)
	}

	var output pythonOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return Result{}, fmt.Errorf("解析 Python 输出失败 (输出: %q): %w", stdout.String(), err)
	}

	titleBlock := make(map[string]string)
	for key, value := range output.Fields.TitleBlock {
		if strings.TrimSpace(value) != "" {
			titleBlock[key] = strings.TrimSpace(value)
		}
	}

	return Result{
		Format:     output.Format,
		TitleBlock: titleBlock,
	}, nil
}

// ExtractPreviewImage 提取 EXB 文件中的内嵌 BMP 预览位图数据
func (parser *Parser) ExtractPreviewImage(ctx context.Context, filePath string) ([]byte, error) {
	absolutePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("解析 EXB 路径失败: %w", err)
	}
	if _, err := os.Stat(absolutePath); err != nil {
		return nil, fmt.Errorf("EXB 文件不存在: %w", err)
	}

	tempOut, err := os.CreateTemp("", "cadguanliq-preview-*.bmp")
	if err != nil {
		return nil, fmt.Errorf("创建临时预览图文件失败: %w", err)
	}
	tempOutPath := tempOut.Name()
	_ = tempOut.Close()
	defer os.Remove(tempOutPath)

	scriptPath := parser.scriptPath
	if scriptPath == "" {
		scriptPath = findScriptPath()
	}
	scriptAbsolute, err := filepath.Abs(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("解析 Python 探针路径失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if _, err := exec.LookPath("uv"); err == nil {
		cmd = exec.CommandContext(ctx, "uv", "run", "--with", "olefile", "python", scriptAbsolute, absolutePath, "--extract-preview", tempOutPath)
	} else {
		cmd = exec.CommandContext(ctx, "python", scriptAbsolute, absolutePath, "--extract-preview", tempOutPath)
	}
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return nil, fmt.Errorf("提取 EXB 预览图失败: %s", errText)
	}

	data, err := os.ReadFile(tempOutPath)
	if err != nil {
		return nil, fmt.Errorf("读取预览图输出失败: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("提取的预览图为空")
	}

	return data, nil
}

func ExtractPreview(data []byte) ([]byte, error) {
	tempFile, err := os.CreateTemp("", "cadguanliq-preview-src-*.exb")
	if err != nil {
		return nil, fmt.Errorf("创建临时 EXB 文件失败: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return nil, fmt.Errorf("写入临时 EXB 文件失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("关闭临时 EXB 文件失败: %w", err)
	}

	return NewParser("").ExtractPreviewImage(context.Background(), tempFile.Name())
}

func buildCommand(ctx context.Context, scriptPath, filePath string) *exec.Cmd {
	var cmd *exec.Cmd
	if venvPython := pythonenv.Interpreter(""); venvPython != "" {
		cmd = exec.CommandContext(ctx, venvPython, scriptPath, filePath)
	} else if _, err := exec.LookPath("uv"); err == nil {
		cmd = exec.CommandContext(ctx, "uv", "run", "--with", "olefile", "python", scriptPath, filePath)
	} else {
		cmd = exec.CommandContext(ctx, "python", scriptPath, filePath)
	}
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	return cmd
}

func findScriptPath() string {
	candidates := []string{
		"tools/exb_probe.py",
		"../tools/exb_probe.py",
		"../../tools/exb_probe.py",
		"../../../tools/exb_probe.py",
	}
	for _, item := range candidates {
		if abs, err := filepath.Abs(item); err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				return abs
			}
		}
	}
	return "tools/exb_probe.py"
}

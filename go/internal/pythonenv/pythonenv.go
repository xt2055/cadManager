package pythonenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const venvDirName = ".venv"

// Interpreter 返回优先使用的 Python 解释器路径：
// <root>/.venv/Scripts/python.exe → CAD_PYTHON_BIN → uv → 系统 python（返回可执行名，由调用方 LookPath）。
func Interpreter(root string) string {
	if root == "" {
		root = "."
	}
	venvPython := filepath.Join(root, venvDirName, "Scripts", "python.exe")
	if _, err := os.Stat(venvPython); err == nil {
		return venvPython
	}
	if custom := strings.TrimSpace(os.Getenv("CAD_PYTHON_BIN")); custom != "" {
		return custom
	}
	return ""
}

// Status 描述当前 Python 环境状态（供向导与系统状态使用）。
type Status struct {
	VenvPath     string `json:"venvPath"`
	VenvReady    bool   `json:"venvReady"`
	Interpreter  string `json:"interpreter"`
	Version      string `json:"version"`
	SystemPython string `json:"systemPython"`
	UvAvailable  bool   `json:"uvAvailable"`
	PipReady     bool   `json:"pipReady"`
	Error        string `json:"error,omitempty"`
}

// Inspect 检测系统 python / uv / venv 现状（不创建）。
func Inspect(root string) Status {
	result := Status{VenvPath: filepath.Join(orDot(root), venvDirName)}
	if _, err := os.Stat(filepath.Join(result.VenvPath, "Scripts", "python.exe")); err == nil {
		result.VenvReady = true
		result.Interpreter = filepath.Join(result.VenvPath, "Scripts", "python.exe")
	}
	if version, err := runVersion(result.Interpreter); err == nil {
		result.Version = version
		result.PipReady = true
	} else if result.VenvReady {
		result.Error = err.Error()
	}
	if version, err := runVersion(""); err == nil && result.Version == "" {
		result.SystemPython = version
	} else if err != nil {
		result.SystemPython = "未检测到"
	}
	if _, err := exec.LookPath("uv"); err == nil {
		result.UvAvailable = true
	}
	if result.Interpreter == "" {
		if custom := strings.TrimSpace(os.Getenv("CAD_PYTHON_BIN")); custom != "" {
			result.Interpreter = custom
		} else if result.UvAvailable {
			result.Interpreter = "uv run --with olefile python"
		} else {
			result.Interpreter = "python"
		}
	}
	return result
}

// Ensure 确保 venv 存在并安装 requirements（已就绪则直接返回）。
func Ensure(ctx context.Context, root string) (Status, error) {
	status := Inspect(root)
	if status.VenvReady && status.PipReady {
		return status, nil
	}
	if strings.TrimSpace(os.Getenv("CAD_PYTHON_BIN")) != "" && !status.VenvReady {
		return status, fmt.Errorf("已配置 CAD_PYTHON_BIN，跳过 venv 创建")
	}
	if err := os.MkdirAll(orDot(root), 0o755); err != nil {
		return status, fmt.Errorf("创建部署目录失败: %w", err)
	}
	python, err := basePython()
	if err != nil {
		return status, err
	}
	venvCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(venvCtx, python, "-m", "venv", status.VenvPath).CombinedOutput(); err != nil {
		return status, fmt.Errorf("创建虚拟环境失败: %v: %s", err, tail(out))
	}
	venvPython := filepath.Join(status.VenvPath, "Scripts", "python.exe")
	requirements := requirementsPath(root)
	if _, err := os.Stat(requirements); err == nil {
		installCtx, installCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer installCancel()
		args := []string{"-m", "pip", "install", "-r", requirements, "--disable-pip-version-check"}
		if mirror := strings.TrimSpace(os.Getenv("CAD_PYPI_MIRROR")); mirror != "" {
			args = append(args, "-i", mirror)
		}
		if out, err := exec.CommandContext(installCtx, venvPython, args...).CombinedOutput(); err != nil {
			return status, fmt.Errorf("安装 Python 依赖失败: %v: %s", err, tail(out))
		}
	}
	return Inspect(root), nil
}

func basePython() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CAD_PYTHON_BIN")); custom != "" {
		return custom, nil
	}
	for _, name := range []string{"python", "python3", "py"} {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("未检测到系统 Python，请安装 Python 3.9+ 并勾选 Add to PATH")
}

func runVersion(interpreter string) (string, error) {
	if interpreter == "" {
		found, err := basePython()
		if err != nil {
			return "", err
		}
		interpreter = found
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, interpreter, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("无法执行 %s: %v", interpreter, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func requirementsPath(root string) string {
	candidates := []string{
		filepath.Join(orDot(root), "tools", "requirements.txt"),
		"tools/requirements.txt",
		"../tools/requirements.txt",
		"../../tools/requirements.txt",
		"../../../tools/requirements.txt",
	}
	for _, item := range candidates {
		if abs, err := filepath.Abs(item); err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				return abs
			}
		}
	}
	return "tools/requirements.txt"
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// MajorMinor 提取 "Python 3.12.4" 中的主次版本号，供兼容性提示。
func MajorMinor(version string) string {
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return version
	}
	return match[1] + "." + match[2]
}

func orDot(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return root
}

func tail(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 400 {
		return text[len(text)-400:]
	}
	return text
}

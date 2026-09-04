// Package setup 提供首次运行安装向导：内嵌网页 + 分步初始化 API。
// 触发条件：工作目录下不存在 .env 配置文件。向导完成后写入配置并自动重启进入正常模式。
package setup

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"cadguanliq/database"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/pythonenv"
	"cadguanliq/internal/response"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed wizard.html
var wizardHTML string

type databaseInput struct {
	Host     string `json:"host"`
	Port     uint16 `json:"port"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password"`
	SSLMode  string `json:"sslMode"`
}

type state struct {
	mu         sync.Mutex
	database   databaseInput
	databaseOK bool
	adminOK    bool
	pythonOK   bool
	server     *http.Server
}

var shared = &state{}

// Run 启动安装向导模式（阻塞直至向导完成并触发重启）。
func Run(cfg config.Config) error {
	if err := os.MkdirAll("data", 0o755); err == nil {
		_ = os.WriteFile(filepath.Join("data", "setup.pending"), []byte(time.Now().Format(time.RFC3339)), 0o644)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/api/setup/status", handleStatus(cfg))
	mux.HandleFunc("/api/setup/database", handleDatabase)
	mux.HandleFunc("/api/setup/admin", handleAdmin)
	mux.HandleFunc("/api/setup/python", handlePython)
	mux.HandleFunc("/api/setup/finish", handleFinish(cfg))
	server := &http.Server{Addr: cfg.Addr, Handler: mux}
	shared.server = server

	url := fmt.Sprintf("http://127.0.0.1%s/setup", displayAddr(cfg.Addr))
	log.Printf("[安装向导] 检测到首次运行，请访问 %s 完成初始化", url)
	openBrowser(url)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-time.After(500 * time.Millisecond)
	return nil
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	return ":" + addr
}

func handleRoot(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/setup" && request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write([]byte(wizardHTML))
}

func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		if err := exec.Command("cmd", "/c", "start", "", url).Start(); err == nil {
			return
		}
	}
	log.Printf("[安装向导] 无法自动打开浏览器，请手动访问: %s", url)
}

func handleStatus(cfg config.Config) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pythonStatus := pythonenv.Inspect(".")
		caxaPath, caxaErr := converter.ResolveCaxaPath(cfg.CaxaBin)
		_, port, _ := net.SplitHostPort(request.Host)
		response.WriteData(writer, http.StatusOK, map[string]any{
			"platform":    runtime.GOOS + "/" + runtime.GOARCH,
			"python":      pythonStatus,
			"caxa":        map[string]any{"available": caxaErr == nil, "path": caxaPath, "error": errorText(caxaErr)},
			"databaseOK":  shared.databaseOK,
			"adminOK":     shared.adminOK,
			"pythonOK":    shared.pythonOK,
			"listenPort":  port,
			"storageRoot": cfg.StorageRoot,
		})
	}
}

func handleDatabase(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	input, err := decode[databaseInput](request)
	if err != nil {
		response.WriteError(writer, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	if input.Host == "" || input.Name == "" || input.User == "" {
		response.WriteError(writer, http.StatusBadRequest, "主机、数据库名与用户名不能为空")
		return
	}
	if input.Port == 0 {
		input.Port = 5432
	}
	if input.SSLMode == "" {
		input.SSLMode = "disable"
	}
	applied, skipped, err := prepareDatabase(request.Context(), input)
	if err != nil {
		response.WriteError(writer, http.StatusBadRequest, err.Error())
		return
	}
	shared.mu.Lock()
	shared.database = input
	shared.databaseOK = true
	shared.mu.Unlock()
	log.Printf("[安装向导] 数据库初始化完成: %s:%d/%s（迁移 %d，跳过 %d）", input.Host, input.Port, input.Name, applied, skipped)
	response.WriteData(writer, http.StatusOK, map[string]any{"applied": applied, "skipped": skipped})
}

// prepareDatabase 确保目标库存在并按序执行迁移。
func prepareDatabase(ctx context.Context, input databaseInput) (applied int, skipped int, err error) {
	maintenanceDSN := dsn(input.Host, input.Port, "postgres", input.User, input.Password, input.SSLMode)
	adminConn, err := pgx.Connect(ctx, maintenanceDSN)
	if err != nil {
		return 0, 0, fmt.Errorf("连接 PostgreSQL 失败（请检查主机/账号/密码）: %w", err)
	}
	var exists bool
	if scanErr := adminConn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, input.Name).Scan(&exists); scanErr != nil {
		_ = adminConn.Close(ctx)
		return 0, 0, fmt.Errorf("查询数据库列表失败: %w", scanErr)
	}
	if !exists {
		if _, execErr := adminConn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, input.Name)); execErr != nil {
			_ = adminConn.Close(ctx)
			return 0, 0, fmt.Errorf("创建数据库 %s 失败: %w", input.Name, execErr)
		}
	}
	_ = adminConn.Close(ctx)

	targetDSN := dsn(input.Host, input.Port, input.Name, input.User, input.Password, input.SSLMode)
	poolConfig, err := pgxpool.ParseConfig(targetDSN)
	if err != nil {
		return 0, 0, fmt.Errorf("解析连接串失败: %w", err)
	}
	poolConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return 0, 0, fmt.Errorf("连接目标数据库失败: %w", err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return 0, 0, fmt.Errorf("初始化迁移记录表失败: %w", err)
	}
	entries, err := fsGlob()
	if err != nil {
		return 0, 0, err
	}
	sort.Strings(entries)
	for _, entry := range entries {
		version := strings.TrimSuffix(filepath.Base(entry), ".sql")
		var already bool
		if scanErr := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&already); scanErr != nil {
			return applied, skipped, scanErr
		}
		if already {
			skipped++
			continue
		}
		content, readErr := database.MigrationFS.ReadFile(entry)
		if readErr != nil {
			return applied, skipped, readErr
		}
		migrationSQL, parseErr := migrationSQLForPGX(content)
		if parseErr != nil {
			return applied, skipped, fmt.Errorf("解析迁移 %s 失败: %w", version, parseErr)
		}
		if _, execErr := pool.Exec(ctx, migrationSQL); execErr != nil {
			return applied, skipped, fmt.Errorf("执行迁移 %s 失败: %w", version, execErr)
		}
		if _, execErr := pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); execErr != nil {
			return applied, skipped, execErr
		}
		applied++
	}
	return applied, skipped, nil
}

// migrationSQLForPGX removes the psql-only error handling directive used when
// the same files are executed manually. Unknown psql commands are rejected so
// they cannot be silently ignored by the application migration path.
func migrationSQLForPGX(content []byte) (string, error) {
	lines := strings.Split(string(content), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `\`) {
			if trimmed == `\set ON_ERROR_STOP on` {
				continue
			}
			return "", fmt.Errorf("不支持的 psql 指令 %q", trimmed)
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n"), nil
}

func fsGlob() ([]string, error) {
	entries, err := fs.Glob(database.MigrationFS, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("未找到数据库迁移脚本")
	}
	return entries, nil
}

func dsn(host string, port uint16, name string, user string, password string, sslMode string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, urlEscape(password), host, port, name, sslMode)
}

func urlEscape(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, " ", "%20")
	value = strings.ReplaceAll(value, "@", "%40")
	value = strings.ReplaceAll(value, ":", "%3A")
	value = strings.ReplaceAll(value, "/", "%2F")
	return value
}

func handleAdmin(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, err := decode[map[string]string](request)
	if err != nil {
		response.WriteError(writer, http.StatusBadRequest, "请求格式错误")
		return
	}
	account := strings.TrimSpace(payload["account"])
	password := payload["password"]
	displayName := strings.TrimSpace(payload["displayName"])
	if account == "" || len(password) < 6 {
		response.WriteError(writer, http.StatusBadRequest, "账号不能为空，密码至少 6 位")
		return
	}
	if displayName == "" {
		displayName = account
	}
	if !shared.databaseOK {
		response.WriteError(writer, http.StatusBadRequest, "请先完成数据库初始化")
		return
	}
	shared.mu.Lock()
	input := shared.database
	shared.mu.Unlock()
	pool, err := openPool(input)
	if err != nil {
		response.WriteError(writer, http.StatusBadRequest, err.Error())
		return
	}
	defer pool.Close()
	authService := auth.NewService(auth.NewPGRepository(pool))
	if _, err = authService.CreateUser(request.Context(), auth.UserInput{
		Account:     account,
		DisplayName: displayName,
		Password:    password,
		Status:      "active",
		Roles:       []string{"admin"},
	}); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "创建管理员失败: "+err.Error())
		return
	}
	shared.mu.Lock()
	shared.adminOK = true
	shared.mu.Unlock()
	log.Printf("[安装向导] 管理员账号已创建: %s", account)
	response.WriteData(writer, http.StatusOK, map[string]bool{"ok": true})
}

func handlePython(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	status, err := pythonenv.Ensure(request.Context(), ".")
	shared.mu.Lock()
	shared.pythonOK = err == nil && status.PipReady
	shared.mu.Unlock()
	if err != nil {
		response.WriteError(writer, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("[安装向导] Python 虚拟环境就绪: %s", status.VenvPath)
	response.WriteData(writer, http.StatusOK, status)
}

func handleFinish(cfg config.Config) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		shared.mu.Lock()
		databaseReady := shared.databaseOK
		input := shared.database
		shared.mu.Unlock()
		if !databaseReady {
			response.WriteError(writer, http.StatusBadRequest, "请先完成数据库初始化")
			return
		}
		caxaPath, _ := converter.ResolveCaxaPath(cfg.CaxaBin)
		lines := []string{
			"# 由安装向导自动生成 " + time.Now().Format("2006-01-02 15:04:05"),
			"CAD_SERVER_ADDR=" + cfg.Addr,
			"CAD_ALLOWED_ORIGINS=*",
			fmt.Sprintf("CAD_DB_HOST=%s", input.Host),
			fmt.Sprintf("CAD_DB_PORT=%d", input.Port),
			fmt.Sprintf("CAD_DB_NAME=%s", input.Name),
			fmt.Sprintf("CAD_DB_USER=%s", input.User),
			fmt.Sprintf("CAD_DB_PASSWORD=%s", input.Password),
			fmt.Sprintf("CAD_DB_SSL_MODE=%s", input.SSLMode),
			"CAD_STORAGE_ROOT=" + cfg.StorageRoot,
			"CAD_LOG_DIR=" + cfg.LogDir,
			"CAD_UPDATES_DIR=" + cfg.UpdatesDir,
			"CAD_SMB_ENABLED=false",
		}
		if caxaPath != "" {
			lines = append(lines, "CAD_CAXA_BIN="+caxaPath)
		}
		if err := os.WriteFile(".env", []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "写入配置失败: "+err.Error())
			return
		}
		_ = os.Remove(filepath.Join("data", "setup.pending"))
		log.Printf("[安装向导] 配置已写入 .env，正在重启服务进入正常模式")
		response.WriteData(writer, http.StatusOK, map[string]bool{"ok": true})
		go func() {
			time.Sleep(800 * time.Millisecond)
			RestartSelf(shared.server)
		}()
	}
}

// RestartSelf 优雅关闭当前 HTTP 服务并以相同参数重启自身（供向导完成与后续在线更新复用）。
func RestartSelf(server *http.Server) {
	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = server.Shutdown(shutdownCtx)
		cancel()
		time.Sleep(200 * time.Millisecond)
	}
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[重启] 获取可执行文件路径失败: %v", err)
		os.Exit(1)
	}
	workingDir, _ := os.Getwd()
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("[重启] 启动新进程失败: %v", err)
		os.Exit(1)
	}
	log.Printf("[重启] 新进程已启动 (PID %d)，当前进程退出", cmd.Process.Pid)
	os.Exit(0)
}

func openPool(input databaseInput) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn(input.Host, input.Port, input.Name, input.User, input.Password, input.SSLMode))
	if err != nil {
		return nil, fmt.Errorf("解析连接串失败: %w", err)
	}
	poolConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	return pool, nil
}

func decode[T any](request *http.Request) (T, error) {
	var result T
	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return result, err
	}
	return result, nil
}

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

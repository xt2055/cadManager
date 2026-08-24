package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr            string
	AllowedOrigins  []string
	UpdateVersion   string
	UpdateNotes     string
	UpdatePublished string
	UpdateURL       string
	UpdateMandatory bool
	StorageRoot     string
	MaxUploadBytes  int64
	Database        DatabaseConfig
}

type DatabaseConfig struct {
	Host            string
	Port            uint16
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	HealthTimeoutMS int
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		Addr:            getenv("CAD_SERVER_ADDR", ":8080"),
		AllowedOrigins:  splitList(getenv("CAD_ALLOWED_ORIGINS", "*")),
		UpdateVersion:   getenv("UPDATE_VERSION", "0.1.0"),
		UpdateNotes:     getenv("UPDATE_NOTES", "暂无更新说明"),
		UpdatePublished: os.Getenv("UPDATE_PUBLISHED_AT"),
		UpdateURL:       os.Getenv("UPDATE_DOWNLOAD_URL"),
		UpdateMandatory: getenvBool("UPDATE_MANDATORY", false),
		StorageRoot:     getenv("CAD_STORAGE_ROOT", "./storage/attachments"),
		MaxUploadBytes:  int64(getenvInt("CAD_MAX_UPLOAD_MB", 100)) * 1024 * 1024,
		Database: DatabaseConfig{
			Host:            getenv("CAD_DB_HOST", "127.0.0.1"),
			Port:            uint16(getenvInt("CAD_DB_PORT", 5432)),
			Name:            getenv("CAD_DB_NAME", "cadguanliq"),
			User:            getenv("CAD_DB_USER", "cadguanliq_app"),
			Password:        os.Getenv("CAD_DB_PASSWORD"),
			SSLMode:         getenv("CAD_DB_SSL_MODE", "disable"),
			MaxConns:        int32(getenvInt("CAD_DB_MAX_CONNS", 10)),
			MinConns:        int32(getenvInt("CAD_DB_MIN_CONNS", 2)),
			HealthTimeoutMS: getenvInt("CAD_DB_HEALTH_TIMEOUT_MS", 1500),
		},
	}
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitList(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{"*"}
	}
	return result
}

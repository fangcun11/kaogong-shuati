package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr          string
	DBPath        string
	WebDir        string
	TikuPath      string
	MaterialsPath string
	IndexPath     string
	SecureCookies bool
	SessionTTL    time.Duration
	AdminUser     string
	AdminPassword string
	Secret        string
	ImportContent bool
	MCPEnabled    bool
}

func Load() Config {
	return Config{
		Addr:          env("KAOGONG_ADDR", "127.0.0.1:8080"),
		DBPath:        env("KAOGONG_DB", "data/kaogong.db"),
		WebDir:        env("KAOGONG_WEB_DIR", "public"),
		TikuPath:      env("KAOGONG_TIKU", "tiku.db"),
		MaterialsPath: env("KAOGONG_MATERIALS", "materials.db"),
		IndexPath:     env("KAOGONG_INDEX", "tiku-index.db"),
		SecureCookies: envBool("KAOGONG_SECURE_COOKIES", false),
		SessionTTL:    envDuration("KAOGONG_SESSION_TTL", 30*24*time.Hour),
		AdminUser:     env("KAOGONG_ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("KAOGONG_ADMIN_PASSWORD"),
		Secret:        os.Getenv("KAOGONG_SECRET"),
		ImportContent: envBool("KAOGONG_IMPORT", false),
		MCPEnabled:    envBool("KAOGONG_MCP", false),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

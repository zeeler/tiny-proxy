package config

import (
	"os"
	"strconv"
)

// Config holds all configuration from environment variables.
type Config struct {
	ProxyPort       string
	DeepSeekAPIKey  string
	DeepSeekBaseURL string
	DeepSeekModel   string
	ReasoningEffort string
	StoreTTL        int
	StoreMax        int
	LogLevel        string
}

// LoadEnv loads configuration from environment variables with defaults.
func LoadEnv() *Config {
	return &Config{
		ProxyPort: getEnv("PROXY_PORT", "3688"),
		DeepSeekAPIKey:  os.Getenv("DEEPSEEK_API_KEY"),
		DeepSeekBaseURL: getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"),
		DeepSeekModel:   getEnv("DEEPSEEK_MODEL", "deepseek-v4-flash"),
		ReasoningEffort: getEnv("REASONING_EFFORT", "high"),
		StoreTTL:        getEnvInt("STORE_TTL", 3600),
		StoreMax:        getEnvInt("STORE_MAX", 500),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

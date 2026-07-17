package config

import (
	"os"
	"strconv"
)

// ProviderConfig holds configuration for a single upstream LLM provider.
type ProviderConfig struct {
	Name    string // provider key: "deepseek", "glm", etc.
	APIKey  string
	BaseURL string
	Model   string
}

// Enabled returns true if the provider has an API key configured.
func (p *ProviderConfig) Enabled() bool { return p.APIKey != "" }

// Config holds all configuration from environment variables.
type Config struct {
	ProxyPort string
	StoreTTL  int
	StoreMax  int
	LogLevel  string
	Providers []*ProviderConfig // in registration order, only enabled
}

// LoadEnv loads configuration from environment variables with defaults.
func LoadEnv() *Config {
	all := allProviders()
	var enabled []*ProviderConfig
	for _, p := range all {
		if p.Enabled() {
			enabled = append(enabled, p)
		}
	}
	return &Config{
		ProxyPort: getEnv("PROXY_PORT", "3688"),
		StoreTTL:  getEnvInt("STORE_TTL", 3600),
		StoreMax:  getEnvInt("STORE_MAX", 500),
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		Providers: enabled,
	}
}

// allProviders defines every supported provider, loaded from env vars.
func allProviders() []*ProviderConfig {
	return []*ProviderConfig{
		{
			Name:    "deepseek",
			APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
			BaseURL: getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"),
			Model:   getEnv("DEEPSEEK_MODEL", "deepseek-v4-flash"),
		},
		{
			Name:    "glm",
			APIKey:  os.Getenv("GLM_API_KEY"),
			BaseURL: getEnv("GLM_BASE_URL", "https://open.bigmodel.cn/api/paas/v4"),
			Model:   getEnv("GLM_MODEL", "glm-4-flash"),
		},
		{
			Name:    "kimi",
			APIKey:  os.Getenv("KIMI_API_KEY"),
			BaseURL: getEnv("KIMI_BASE_URL", "https://api.moonshot.cn/v1"),
			Model:   getEnv("KIMI_MODEL", "moonshot-v1-8k"),
		},
		{
			Name:    "minimax",
			APIKey:  os.Getenv("MINIMAX_API_KEY"),
			BaseURL: getEnv("MINIMAX_BASE_URL", "https://api.minimax.chat/v1"),
			Model:   getEnv("MINIMAX_MODEL", "abab6.5s-chat"),
		},
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

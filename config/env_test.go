package config

import (
	"os"
	"testing"
)

func TestLoadEnvDefaults(t *testing.T) {
	os.Unsetenv("PROXY_PORT")
	os.Unsetenv("DEEPSEEK_MODEL")
	os.Unsetenv("REASONING_EFFORT")
	os.Unsetenv("STORE_TTL")
	os.Unsetenv("STORE_MAX")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("DEEPSEEK_BASE_URL")

	cfg := LoadEnv()

	if cfg.ProxyPort != "3688" {
		t.Errorf("ProxyPort = %q, want 3688", cfg.ProxyPort)
	}
	if cfg.DeepSeekModel != "deepseek-v4-flash" {
		t.Errorf("DeepSeekModel = %q", cfg.DeepSeekModel)
	}
	if cfg.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q", cfg.ReasoningEffort)
	}
	if cfg.StoreTTL != 3600 {
		t.Errorf("StoreTTL = %d", cfg.StoreTTL)
	}
	if cfg.StoreMax != 500 {
		t.Errorf("StoreMax = %d", cfg.StoreMax)
	}
	if cfg.DeepSeekBaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("DeepSeekBaseURL = %q", cfg.DeepSeekBaseURL)
	}
}

func TestLoadEnvCustom(t *testing.T) {
	os.Setenv("PROXY_PORT", "4000")
	os.Setenv("DEEPSEEK_API_KEY", "sk-test")
	defer func() {
		os.Unsetenv("PROXY_PORT")
		os.Unsetenv("DEEPSEEK_API_KEY")
	}()

	cfg := LoadEnv()
	if cfg.ProxyPort != "4000" {
		t.Errorf("ProxyPort = %q", cfg.ProxyPort)
	}
	if cfg.DeepSeekAPIKey != "sk-test" {
		t.Errorf("DeepSeekAPIKey = %q", cfg.DeepSeekAPIKey)
	}
}

package config

import (
	"os"
	"testing"
)

func TestLoadEnvDefaults(t *testing.T) {
	os.Unsetenv("PROXY_PORT")
	os.Unsetenv("STORE_TTL")
	os.Unsetenv("STORE_MAX")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("DEEPSEEK_API_KEY")
	os.Unsetenv("GLM_API_KEY")
	os.Unsetenv("KIMI_API_KEY")
	os.Unsetenv("MINIMAX_API_KEY")

	cfg := LoadEnv()

	if cfg.ProxyPort != "3688" {
		t.Errorf("ProxyPort = %q, want 3688", cfg.ProxyPort)
	}
	if cfg.StoreTTL != 3600 {
		t.Errorf("StoreTTL = %d", cfg.StoreTTL)
	}
	if cfg.StoreMax != 500 {
		t.Errorf("StoreMax = %d", cfg.StoreMax)
	}
	// No API keys set → no providers
	if len(cfg.Providers) != 0 {
		t.Errorf("expected 0 providers with no keys, got %d", len(cfg.Providers))
	}
}

func TestLoadEnvWithProviders(t *testing.T) {
	os.Setenv("PROXY_PORT", "4000")
	os.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")
	os.Setenv("GLM_API_KEY", "sk-glm")
	defer func() {
		os.Unsetenv("PROXY_PORT")
		os.Unsetenv("DEEPSEEK_API_KEY")
		os.Unsetenv("GLM_API_KEY")
	}()

	cfg := LoadEnv()
	if cfg.ProxyPort != "4000" {
		t.Errorf("ProxyPort = %q", cfg.ProxyPort)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "deepseek" || cfg.Providers[0].APIKey != "sk-deepseek" {
		t.Errorf("first provider: name=%q apiKey=%q", cfg.Providers[0].Name, cfg.Providers[0].APIKey)
	}
	if cfg.Providers[1].Name != "glm" || cfg.Providers[1].APIKey != "sk-glm" {
		t.Errorf("second provider: name=%q apiKey=%q", cfg.Providers[1].Name, cfg.Providers[1].APIKey)
	}
}

func TestProviderDefaults(t *testing.T) {
	os.Setenv("DEEPSEEK_API_KEY", "sk-ds")
	os.Setenv("KIMI_API_KEY", "sk-kimi")
	os.Setenv("KIMI_MODEL", "my-custom-model")
	defer func() {
		os.Unsetenv("DEEPSEEK_API_KEY")
		os.Unsetenv("KIMI_API_KEY")
		os.Unsetenv("KIMI_MODEL")
	}()

	cfg := LoadEnv()

	for _, p := range cfg.Providers {
		switch p.Name {
		case "deepseek":
			if p.Model != "deepseek-v4-flash" {
				t.Errorf("deepseek default model = %q", p.Model)
			}
			if p.BaseURL != "https://api.deepseek.com/v1" {
				t.Errorf("deepseek base url = %q", p.BaseURL)
			}
		case "kimi":
			if p.Model != "my-custom-model" {
				t.Errorf("kimi custom model = %q", p.Model)
			}
		}
	}
}

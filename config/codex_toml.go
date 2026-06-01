package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultCodexConfigPath returns ~/.codex/config.toml.
func DefaultCodexConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

// DefaultCodexAuthPath returns ~/.codex/auth.json.
func DefaultCodexAuthPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "auth.json")
}

// BackupConfig creates a .bak copy of the config file.
func BackupConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	return os.WriteFile(path+".bak", data, 0644)
}

// SetupCodexConfig ensures config.toml has the tiny-proxy provider.
func SetupCodexConfig(path, port, authKey string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	result := string(data)
	section := "[model_providers.tiny-proxy]"

	if strings.Contains(result, section) {
		result = updateKeyInSection(result, "base_url",
			"http://127.0.0.1:"+port+"/v1", section)
		result = updateKeyInSection(result, "experimental_bearer_token",
			authKey, section)
	} else {
		result += fmt.Sprintf(`
[model_providers.tiny-proxy]
name = "tiny-proxy"
base_url = "http://127.0.0.1:%s/v1"
wire_api = "responses"
requires_openai_auth = true
experimental_bearer_token = "%s"
`, port, authKey)
	}

	result = setTopLevel(result, "model_provider", "tiny-proxy")
	result = setTopLevel(result, "model_reasoning_effort", "high")

	return os.WriteFile(path, []byte(result), 0644)
}

// RestoreConfig restores config.toml from .bak and removes the backup.
func RestoreConfig(path string) error {
	backupPath := path + ".bak"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("no backup at %s: %w", backupPath, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	return os.Remove(backupPath)
}

// DryRunSetupCodex returns what SetupCodexConfig would write, without modifying files.
func DryRunSetupCodex(path, port, authKey string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}

	result := string(data)
	section := "[model_providers.tiny-proxy]"

	if strings.Contains(result, section) {
		result = updateKeyInSection(result, "base_url",
			"http://127.0.0.1:"+port+"/v1", section)
		result = updateKeyInSection(result, "experimental_bearer_token",
			authKey, section)
	} else {
		result += fmt.Sprintf(`
[model_providers.tiny-proxy]
name = "tiny-proxy"
base_url = "http://127.0.0.1:%s/v1"
wire_api = "responses"
requires_openai_auth = true
experimental_bearer_token = "%s"
`, port, authKey)
	}

	result = setTopLevel(result, "model_provider", "tiny-proxy")
	result = setTopLevel(result, "model_reasoning_effort", "high")
	return result, nil
}

// UpdateAuthJSON writes the proxy auth key to auth.json.
func UpdateAuthJSON(path, authKey string) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	content := fmt.Sprintf(`{
  "OPENAI_API_KEY": "%s"
}
`, authKey)
	return os.WriteFile(path, []byte(content), 0600)
}

// --- helpers ---

func setTopLevel(content, key, value string) string {
	prefix := key + " = "
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = key + ` = "` + value + `"`
			return strings.Join(lines, "\n")
		}
	}
	return key + ` = "` + value + `"` + "\n" + content
}

func updateKeyInSection(content, key, value, section string) string {
	lines := strings.Split(content, "\n")
	inSection := false
	prefix := key + " = "

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == section {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			return strings.Join(lines, "\n")
		}
		if inSection && strings.HasPrefix(trimmed, prefix) {
			lines[i] = key + ` = "` + value + `"`
			return strings.Join(lines, "\n")
		}
	}
	return strings.Join(lines, "\n")
}

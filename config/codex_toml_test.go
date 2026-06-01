package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `model = "gpt-5"
`
	os.WriteFile(path, []byte(content), 0644)

	if err := BackupConfig(path); err != nil {
		t.Fatal(err)
	}

	backupPath := path + ".bak"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal("backup file should exist:", err)
	}
	if string(data) != content {
		t.Errorf("backup = %q, want %q", string(data), content)
	}
}

func TestSetupCodexConfigNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`model = "gpt-5"
`), 0644)

	if err := SetupCodexConfig(path, "3688", "sk-test"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	result := string(data)

	checks := []string{
		`model_provider = "tiny-proxy"`,
		`[model_providers.tiny-proxy]`,
		`wire_api = "responses"`,
		`base_url = "http://127.0.0.1:3688/v1"`,
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected %q in config, got:\n%s", check, result)
		}
	}
}

func TestSetupCodexConfigUpdateExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`[model_providers.tiny-proxy]
name = "tiny-proxy"
base_url = "http://127.0.0.1:3688/v1"
wire_api = "responses"
requires_openai_auth = true
experimental_bearer_token = "old-key"
`), 0644)

	if err := SetupCodexConfig(path, "4000", "new-key"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	result := string(data)
	if !strings.Contains(result, `base_url = "http://127.0.0.1:4000/v1"`) {
		t.Error("base_url should be updated")
	}
	if !strings.Contains(result, `experimental_bearer_token = "new-key"`) {
		t.Error("bearer token should be updated")
	}
}

func TestRestoreConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `model = "gpt-5"
`
	os.WriteFile(path, []byte(content), 0644)

	BackupConfig(path)
	os.WriteFile(path, []byte(`model = "changed"`), 0644)

	if err := RestoreConfig(path); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Errorf("restored = %q, want %q", string(data), content)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("backup should be removed after restore")
	}
}

func TestRestoreNoBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := RestoreConfig(path); err == nil {
		t.Error("expected error when no backup exists")
	}
}

func TestUpdateAuthJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	if err := UpdateAuthJSON(path, "sk-key"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "OPENAI_API_KEY") {
		t.Error("should contain OPENAI_API_KEY")
	}
	if !strings.Contains(s, "sk-key") {
		t.Error("should contain the key value")
	}
}

func TestDryRunSetup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`model = "gpt-5"
`), 0644)

	result, err := DryRunSetupCodex(path, "3688", "sk-test")
	if err != nil {
		t.Fatal(err)
	}

	// File should NOT be modified
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "gpt-5") {
		t.Error("dry-run should not modify the file")
	}

	// Result should contain the proposed changes
	if !strings.Contains(result, "tiny-proxy") {
		t.Error("dry-run result should contain proposed changes")
	}
}

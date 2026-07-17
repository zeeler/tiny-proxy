package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/zeeler/codex-miniproxy/config"
	"github.com/zeeler/codex-miniproxy/proxy"
)

var (
	port    = flag.String("port", "", "Proxy listen port (overrides PROXY_PORT env)")
	setup   = flag.Bool("setup", false, "Setup Codex config only, don't start proxy")
	dryRun  = flag.Bool("dry-run", false, "Preview config changes without modifying files")
	restore = flag.Bool("restore", false, "Restore original Codex config from backup")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `tiny-proxy — local proxy for Codex/ChatGPT App to use Chinese LLMs

Usage:
  tiny-proxy [flags]

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), `
Environment Variables:
  Each provider needs its own API key. At least one is required.

  Provider     API Key Env          Model Env            Base URL Env
  ─────────    ────────────         ──────────           ────────────
  DeepSeek     DEEPSEEK_API_KEY     DEEPSEEK_MODEL       DEEPSEEK_BASE_URL
  GLM (智谱)   GLM_API_KEY          GLM_MODEL            GLM_BASE_URL
  Kimi (月暗)  KIMI_API_KEY         KIMI_MODEL           KIMI_BASE_URL
  Qwen (通义)  QWEN_API_KEY         QWEN_MODEL           QWEN_BASE_URL
  MiniMax      MINIMAX_API_KEY      MINIMAX_MODEL        MINIMAX_BASE_URL
  Doubao (豆包) DOUBAO_API_KEY      DOUBAO_MODEL         DOUBAO_BASE_URL
  Seed Code    SEEDCODE_API_KEY     SEEDCODE_MODEL       SEEDCODE_BASE_URL

  Proxy settings:
    PROXY_PORT      Listen port (default: 3688)
    STORE_TTL       Session TTL in seconds (default: 3600)
    STORE_MAX       Max stored sessions (default: 500)
    LOG_LEVEL       Log level (default: info)

Examples:
  # DeepSeek only
  export DEEPSEEK_API_KEY=sk-xxx
  tiny-proxy

  # Multiple providers
  export DEEPSEEK_API_KEY=sk-xxx
  export GLM_API_KEY=xxx
  export KIMI_API_KEY=xxx
  tiny-proxy
`)
	}
}

func main() {
	flag.Parse()

	cfg := config.LoadEnv()
	if *port != "" {
		cfg.ProxyPort = *port
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// Handle restore command
	if *restore {
		restoreConfig()
		return
	}

	// Handle setup-only mode
	if *setup {
		setupConfig(cfg, *dryRun)
		return
	}

	// Validate at least one provider configured
	if len(cfg.Providers) == 0 {
		log.Fatal("[FATAL] No API keys configured. Set at least one *_API_KEY env var (e.g. DEEPSEEK_API_KEY).")
	}
	names := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		names[i] = p.Name
	}
	log.Printf("[INFO] providers: %s", strings.Join(names, ", "))

	// Default mode: setup config + start proxy
	configPath := config.DefaultCodexConfigPath()
	if configPath != "" {
		// Backup existing config (ok if it doesn't exist yet)
		if err := config.BackupConfig(configPath); err != nil {
			log.Printf("[WARN] cannot backup config: %v", err)
		}
		// Setup runs independently — config/codex_toml.go handles missing files
		placeholderKey := "not-required"
		if err := config.SetupCodexConfig(configPath, cfg.ProxyPort, placeholderKey); err != nil {
			log.Printf("[WARN] cannot setup config: %v", err)
		} else {
			log.Printf("[INFO] Codex config updated: %s", configPath)
		}

		authPath := config.DefaultCodexAuthPath()
		if err := config.UpdateAuthJSON(authPath, placeholderKey); err != nil {
			log.Printf("[WARN] cannot update auth.json: %v", err)
		}
	}

	// Start proxy server
	srv := proxy.NewServer(cfg)
	log.Fatal(srv.Start())
}

func setupConfig(cfg *config.Config, dryRun bool) {
	configPath := config.DefaultCodexConfigPath()
	if configPath == "" {
		log.Fatal("[FATAL] cannot determine Codex config path")
	}

	if dryRun {
		result, err := config.DryRunSetupCodex(configPath, cfg.ProxyPort, "not-required")
		if err != nil {
			log.Fatalf("[FATAL] dry-run failed: %v", err)
		}
		fmt.Println("=== Proposed config.toml changes ===")
		fmt.Println(result)
		fmt.Println("=== End (no files modified) ===")
		return
	}

	if err := config.BackupConfig(configPath); err != nil {
		log.Printf("[WARN] cannot backup config (will proceed without backup): %v", err)
	}
	if err := config.SetupCodexConfig(configPath, cfg.ProxyPort, "not-required"); err != nil {
		log.Fatalf("[FATAL] setup failed: %v", err)
	}
	fmt.Printf("Codex config updated: %s\n", configPath)
	fmt.Printf("Use 'tiny-proxy --restore' to revert.\n")
}

func restoreConfig() {
	configPath := config.DefaultCodexConfigPath()
	if configPath == "" {
		log.Fatal("[FATAL] cannot determine Codex config path")
	}
	if err := config.RestoreConfig(configPath); err != nil {
		log.Fatalf("[FATAL] restore failed: %v", err)
	}
	fmt.Printf("Config restored from backup: %s\n", configPath)
}

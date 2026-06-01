package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/terry/tiny-proxy/config"
	"github.com/terry/tiny-proxy/proxy"
)

var (
	port    = flag.String("port", "", "Proxy listen port (overrides PROXY_PORT env)")
	setup   = flag.Bool("setup", false, "Setup Codex config only, don't start proxy")
	dryRun  = flag.Bool("dry-run", false, "Preview config changes without modifying files")
	restore = flag.Bool("restore", false, "Restore original Codex config from backup")
)

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

	// Validate required config
	if cfg.DeepSeekAPIKey == "" {
		log.Fatal("[FATAL] DEEPSEEK_API_KEY is required. Set it via environment variable.")
	}

	// Default mode: setup config + start proxy
	configPath := config.DefaultCodexConfigPath()
	if configPath != "" {
		if err := config.BackupConfig(configPath); err != nil {
			log.Printf("[WARN] cannot backup config: %v", err)
		} else {
			if err := config.SetupCodexConfig(configPath, cfg.ProxyPort, cfg.ProxyAuthKey); err != nil {
				log.Printf("[WARN] cannot setup config: %v", err)
			} else {
				log.Printf("[INFO] Codex config updated: %s", configPath)
			}
		}

		authPath := config.DefaultCodexAuthPath()
		if err := config.UpdateAuthJSON(authPath, cfg.ProxyAuthKey); err != nil {
			log.Printf("[WARN] cannot update auth.json: %v", err)
		}
	}

	// Start proxy server
	srv := proxy.NewServer(cfg)
	log.Printf("[INFO] Auth key (for Codex auth.json): %s", cfg.ProxyAuthKey)
	log.Fatal(srv.Start())
}

func setupConfig(cfg *config.Config, dryRun bool) {
	configPath := config.DefaultCodexConfigPath()
	if configPath == "" {
		log.Fatal("[FATAL] cannot determine Codex config path")
	}

	if dryRun {
		result, err := config.DryRunSetupCodex(configPath, cfg.ProxyPort, cfg.ProxyAuthKey)
		if err != nil {
			log.Fatalf("[FATAL] dry-run failed: %v", err)
		}
		fmt.Println("=== Proposed config.toml changes ===")
		fmt.Println(result)
		fmt.Println("=== End (no files modified) ===")
		return
	}

	if err := config.BackupConfig(configPath); err != nil {
		log.Fatalf("[FATAL] backup failed: %v", err)
	}
	if err := config.SetupCodexConfig(configPath, cfg.ProxyPort, cfg.ProxyAuthKey); err != nil {
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

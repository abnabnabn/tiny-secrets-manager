package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tiny-secrets-manager/internal/api"
	"tiny-secrets-manager/internal/config"
	"tiny-secrets-manager/internal/server"
	"tiny-secrets-manager/internal/store"
	"tiny-secrets-manager/public"

	"golang.org/x/crypto/bcrypt"
)

var Version = "dev"

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "--hash" {
		hash, err := bcrypt.GenerateFromPassword([]byte(os.Args[2]), 14)
		if err != nil {
			panic(err)
		}
		_, _ = os.Stdout.Write(hash)
		_, _ = os.Stdout.WriteString("\n")
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	var configPath string
	var insecureFlag bool
	var adminUserFlag string
	var adminPassFlag string
	var adminTokenFlag string
	var masterKeyFlag string
	var listenFlag string
	var dbPathFlag string
	var backupTargetFlag string
	var recoveryKeyFlag string
	var seedOnlyFlag bool

	flag.BoolVar(&insecureFlag, "insecure", false, "Disable secure mode")
	flag.StringVar(&adminUserFlag, "admin-user", "", "Admin username")
	flag.StringVar(&adminPassFlag, "admin-pass", "", "Admin password")
	flag.StringVar(&adminTokenFlag, "admin-token", "", "Admin API token")
	flag.StringVar(&masterKeyFlag, "master-key", "", "Master key")
	flag.StringVar(&listenFlag, "listen", "", "Listen address")
	flag.StringVar(&dbPathFlag, "db-path", "", "Database path")
	flag.StringVar(&backupTargetFlag, "backup-target", "", "Backup target")
	flag.StringVar(&recoveryKeyFlag, "recovery-key", "", "Recovery key")
	flag.BoolVar(&seedOnlyFlag, "seed-only", false, "Seed the database and exit immediately")

	flag.Parse()

	if flag.NArg() > 0 {
		configPath = flag.Arg(0)
	}

	var cfg *config.Config
	var err error

	cfg, err = server.Bootstrap(logger, configPath)

	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	if insecureFlag {
		cfg.Insecure = true
	}
	if masterKeyFlag != "" {
		cfg.MasterKey = masterKeyFlag
	}
	if listenFlag != "" {
		cfg.Listen = listenFlag
	}
	if dbPathFlag != "" {
		cfg.DBPath = dbPathFlag
	}

	recoveryKey := recoveryKeyFlag
	if recoveryKey == "" {
		recoveryKey = os.Getenv("TSM_RECOVERY_KEY")
	}

	db, err := store.New(cfg.DBPath, cfg.MasterKey, recoveryKey, logger)
	if err != nil {
		logger.Error("failed to init store", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := server.SeedAdminUser(context.Background(), db, adminUserFlag, adminPassFlag, adminTokenFlag); err != nil {
		logger.Error("failed to seed admin user", "err", err)
		os.Exit(1)
	}

	if err := seedBackupTarget(context.Background(), db, backupTargetFlag); err != nil {
		logger.Error("failed to seed backup target", "err", err)
		os.Exit(1)
	}

	if seedOnlyFlag {
		logger.Info("database seeded successfully, exiting due to -seed-only flag")
		return
	}

	if err := runServer(cfg, db, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}

func seedBackupTarget(ctx context.Context, db *store.Store, target string) error {
	if target == "" {
		target = os.Getenv("TSM_BACKUP_TARGET")
	}
	if target == "" {
		return nil
	}
	trimmed := strings.TrimSpace(target)
	if strings.HasPrefix(trimmed, "-") {
		return fmt.Errorf("invalid backup target: cannot start with a dash")
	}
	if err := db.PutSetting(ctx, "backup_target", trimmed); err != nil {
		return fmt.Errorf("failed to seed backup target: %w", err)
	}
	return nil
}

func runServer(cfg *config.Config, db *store.Store, logger *slog.Logger) error {
	logo := `
  _____ _                 _____                    _       
 |_   _(_)_ __ _   _     / ____|                  | |      
   | | | | '_ \ | | |   | (___   ___  ___ _ __ ___| |_ ___ 
   | | | | | | | |_| |   \___ \ / _ \/ __| '__/ _ \ __/ __|
   | | |_| | | |\__, |   ____) |  __/ (__| | |  __/ |_\__ \
   \_/   |_| |_| __/ |  |_____/ \___|\___|_|  \___|\__|___/
                |___/                                      
                                        Manager
`
	fmt.Println(logo)
	fmt.Printf("  Version: %s\n", Version)

	if cfg.Insecure {
		fmt.Println("  ========================================================")
		fmt.Println("  WARNING: Server is running in INSECURE mode.")
		fmt.Println("           HTTPS enforcement and secure cookies are disabled.")
		fmt.Println("           Do NOT use this mode in production!")
		fmt.Println("  ========================================================")
		fmt.Println()
	}

	srv := api.NewServer(db, cfg, logger, Version)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	cliDir := os.Getenv("TSM_CLI_DIR")
	if cliDir == "" {
		cliDir = "./cli"
	}
	// #nosec G304 G703 - cliDir is configurable by the admin via environment variable
	if stat, err := os.Stat(cliDir); err == nil && stat.IsDir() {
		mux.Handle("/cli/", http.StripPrefix("/cli/", http.FileServer(http.Dir(cliDir))))
	}

	mux.Handle("/", http.FileServer(http.FS(public.FS)))
	httpServer := &http.Server{
		Addr:         cfg.Listen,
		Handler:      http.TimeoutHandler(srv.SecurityMiddleware(mux), 15*time.Second, "request timed out"),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("listening", "addr", cfg.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen error", "err", err)
			os.Exit(1)
		}
	}()

	tickerCtx, tickerCancel := context.WithCancel(context.Background())
	defer tickerCancel()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-ticker.C:
				deleted, err := db.DeleteExpiredRoles(tickerCtx)
				if err != nil {
					logger.Error("failed to delete expired roles", "err", err)
				} else if deleted > 0 {
					logger.Info("cleaned up expired roles", "count", deleted)
				}
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}

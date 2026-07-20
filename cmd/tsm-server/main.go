package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tiny-secrets-manager/internal/api"
	"tiny-secrets-manager/internal/config"
	"tiny-secrets-manager/internal/store"
	"tiny-secrets-manager/public"

	"golang.org/x/crypto/bcrypt"
)

var Version = "dev"

type serverFlags struct {
	configPath   string
	insecure     bool
	adminUser    string
	adminPass    string
	adminToken   string
	masterKey    string
	listen       string
	dbPath       string
	backupTarget string
	recoveryKey  string
	seedOnly     bool
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func bootstrap(logger *slog.Logger, configPath string) (*config.Config, error) {
	if configPath == "" {
		configPath = "config.json"
	}

	// 1. Check if we have an existing config
	if _, err := os.Stat(configPath); err == nil {
		return config.Load(configPath)
	}

	// 2. No config found - Auto-generate infrastructure
	logger.Info("no configuration found, initiating self-bootstrap...")

	mKey := make([]byte, 32)
	_, _ = rand.Read(mKey)

	cfg := &config.Config{
		MasterKey: base64.StdEncoding.EncodeToString(mKey),
		Listen:    "0.0.0.0:8090",
		DBPath:    "tsm.db",
	}

	out, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configPath, out, 0600); err != nil {
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	logger.Info("infrastructure configuration generated", "path", configPath)
	return config.Load(configPath)
}

func handleHashCmd() bool {
	if len(os.Args) >= 3 && os.Args[1] == "--hash" {
		hash, err := bcrypt.GenerateFromPassword([]byte(os.Args[2]), 14)
		if err != nil {
			panic(err)
		}
		_, _ = os.Stdout.Write(hash)
		_, _ = os.Stdout.WriteString("\n")
		return true
	}
	return false
}

func initLogger() *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	return logger
}

func parseFlags() *serverFlags {
	var f serverFlags

	flag.BoolVar(&f.insecure, "insecure", false, "Disable secure mode")
	flag.StringVar(&f.adminUser, "admin-user", "", "Admin username")
	flag.StringVar(&f.adminPass, "admin-pass", "", "Admin password")
	flag.StringVar(&f.adminToken, "admin-token", "", "Admin API token")
	flag.StringVar(&f.masterKey, "master-key", "", "Master key")
	flag.StringVar(&f.listen, "listen", "", "Listen address")
	flag.StringVar(&f.dbPath, "db-path", "", "Database path")
	flag.StringVar(&f.backupTarget, "backup-target", "", "Backup target")
	flag.StringVar(&f.recoveryKey, "recovery-key", "", "Recovery key")
	flag.BoolVar(&f.seedOnly, "seed-only", false, "Seed the database and exit immediately")

	flag.Parse()

	if flag.NArg() > 0 {
		f.configPath = flag.Arg(0)
	}

	return &f
}

func setupConfigAndStore(logger *slog.Logger, flags *serverFlags) (*config.Config, *store.Store, error) {
	cfg, err := bootstrap(logger, flags.configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	if flags.insecure {
		cfg.Insecure = true
	}
	if flags.masterKey != "" {
		cfg.MasterKey = flags.masterKey
	}
	if flags.listen != "" {
		cfg.Listen = flags.listen
	}
	if flags.dbPath != "" {
		cfg.DBPath = flags.dbPath
	}

	recoveryKey := flags.recoveryKey
	if recoveryKey == "" {
		recoveryKey = os.Getenv("TSM_RECOVERY_KEY")
	}

	db, err := store.New(cfg.DBPath, cfg.MasterKey, recoveryKey, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init store: %w", err)
	}

	return cfg, db, nil
}

func seedDatabase(ctx context.Context, db *store.Store, flags *serverFlags, logger *slog.Logger) error {
	if err := seedAdminUser(ctx, db, flags.adminUser, flags.adminPass, flags.adminToken); err != nil {
		return fmt.Errorf("failed to seed admin user: %w", err)
	}

	backupTarget := flags.backupTarget
	if backupTarget == "" {
		backupTarget = os.Getenv("TSM_BACKUP_TARGET")
	}
	if backupTarget != "" {
		if err := db.PutSetting(ctx, "backup_target", backupTarget); err != nil {
			logger.Error("failed to seed backup target", "err", err)
		}
	}

	return nil
}

func main() {
	if handleHashCmd() {
		return
	}

	logger := initLogger()
	flags := parseFlags()

	cfg, db, err := setupConfigAndStore(logger, flags)
	if err != nil {
		logger.Error("setup failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := seedDatabase(context.Background(), db, flags, logger); err != nil {
		logger.Error("seeding failed", "err", err)
		os.Exit(1)
	}

	if flags.seedOnly {
		logger.Info("database seeded successfully, exiting due to -seed-only flag")
		return
	}

	if err := runServer(cfg, db, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}

func seedAdminUser(ctx context.Context, db *store.Store, adminUser, adminPass, adminToken string) error {
	adminCount, err := db.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("failed to count admins: %w", err)
	}
	if adminCount > 0 {
		return nil
	}

	user := adminUser
	if user == "" {
		user = os.Getenv("TSM_ADMIN_USER")
	}
	if user == "" {
		user = "admin"
	}

	pass := adminPass
	if pass == "" {
		pass = os.Getenv("TSM_ADMIN_PASS")
	}
	if pass == "" {
		pass = generateRandomString(12)
	}

	token := adminToken
	if token == "" {
		token = os.Getenv("TSM_ADMIN_TOKEN")
	}
	if token == "" {
		token = generateRandomString(32)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), 14)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := db.PutAdmin(ctx, user, string(hash)); err != nil {
		return fmt.Errorf("failed to create admin: %w", err)
	}

	tokenHash := sha256.Sum256([]byte(token))
	pJSON, _ := json.Marshal([]config.Policy{{Prefix: "*", Methods: []string{"*"}}})
	if err := db.PutRole(ctx, "admin", tokenHash[:], pJSON, true, false, nil); err != nil {
		return fmt.Errorf("failed to create admin role: %w", err)
	}

	fmt.Println("\n" + `========================================================================`)
	fmt.Println(`                        INITIAL SETUP COMPLETE                          `)
	fmt.Println(`========================================================================`)
	fmt.Printf("  Username: %s\n", user)
	fmt.Printf("  Password: %s\n", pass)
	fmt.Printf("  Admin API Token: %s\n", token)
	fmt.Println("")
	fmt.Println(`  [IMPORTANT] These credentials have been seeded into the database.`)
	fmt.Println(`              This is the ONLY time the password and token will be shown.`)
	fmt.Println(`========================================================================`)

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

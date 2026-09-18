// Package server contains the startup logic for the Tiny Secrets Manager server,
// extracted from cmd/tsm-server so it can be independently unit tested.
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"tiny-secrets-manager/internal/config"
	"tiny-secrets-manager/internal/store"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the work factor for password hashing. It can be lowered in
// tests to avoid the intentional slowness of the production cost factor.
var BcryptCost = 14

// generateRandomString returns a cryptographically random URL-safe base64 string
// of at least n bytes of entropy.
func generateRandomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Bootstrap loads the configuration from configPath, or creates a new one if it
// does not exist. An empty configPath defaults to "config.json".
func Bootstrap(logger *slog.Logger, configPath string) (*config.Config, error) {
	if configPath == "" {
		configPath = "config.json"
	}

	// Config already exists - just load it.
	if _, err := os.Stat(configPath); err == nil {
		return config.Load(configPath)
	}

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

// SeedAdminUser creates an initial admin user if none exist in the database.
// Credentials are resolved in order: explicit arguments -> environment variables ->
// sensible defaults (username "admin", random password and token).
// The generated credentials are printed once to stdout and never stored in plaintext.
func SeedAdminUser(ctx context.Context, db *store.Store, adminUser, adminPass, adminToken string) error {
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

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), BcryptCost)
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

	fmt.Println("")
	fmt.Println("========================================================================")
	fmt.Println("                        INITIAL SETUP COMPLETE                          ")
	fmt.Println("========================================================================")
	fmt.Printf("  Username: %s\n", user)
	fmt.Printf("  Password: %s\n", pass)
	fmt.Printf("  Admin API Token: %s\n", token)
	fmt.Println("")
	fmt.Println("  [IMPORTANT] These credentials have been seeded into the database.")
	fmt.Println("              This is the ONLY time the password and token will be shown.")
	fmt.Println("========================================================================")

	return nil
}

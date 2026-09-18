package server_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"tiny-secrets-manager/internal/server"
	"tiny-secrets-manager/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// Lower bcrypt cost from 14 to MinCost (4) so tests run in milliseconds
	// rather than seconds. Production code retains cost 14.
	server.BcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}


// newTestStore creates an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	masterKeyB64 := base64.StdEncoding.EncodeToString(key)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	st, err := store.New(dbPath, masterKeyB64, "", logger)
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

// ---- Bootstrap tests -------------------------------------------------------

func TestBootstrap_ExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	// Write a pre-existing config.
	existing := map[string]string{
		"master_key": "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXQ=",
		"listen":     "127.0.0.1:9999",
		"db_path":    "existing.db",
	}
	data, _ := json.Marshal(existing)
	require.NoError(t, os.WriteFile(cfgPath, data, 0600))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg, err := server.Bootstrap(logger, cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9999", cfg.Listen)
	assert.Equal(t, "existing.db", cfg.DBPath)
}

func TestBootstrap_CreatesNewConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	// No file exists yet.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg, err := server.Bootstrap(logger, cfgPath)
	require.NoError(t, err)

	// Should return a valid config with defaults.
	assert.Equal(t, "0.0.0.0:8090", cfg.Listen)
	assert.Equal(t, "tsm.db", cfg.DBPath)
	assert.NotEmpty(t, cfg.MasterKey, "master key should be auto-generated")

	// The file should now exist on disk.
	_, statErr := os.Stat(cfgPath)
	assert.NoError(t, statErr, "config file should have been written to disk")
}

func TestBootstrap_GeneratedMasterKeyIsRandom(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg1, err := server.Bootstrap(logger, filepath.Join(t.TempDir(), "config.json"))
	require.NoError(t, err)
	cfg2, err := server.Bootstrap(logger, filepath.Join(t.TempDir(), "config.json"))
	require.NoError(t, err)

	assert.NotEqual(t, cfg1.MasterKey, cfg2.MasterKey, "each bootstrap should generate a unique master key")
}

func TestBootstrap_WrittenConfigIsReadable(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg1, err := server.Bootstrap(logger, cfgPath)
	require.NoError(t, err)

	// Load the same path a second time - should return identical values.
	cfg2, err := server.Bootstrap(logger, cfgPath)
	require.NoError(t, err)

	assert.Equal(t, cfg1.MasterKey, cfg2.MasterKey)
	assert.Equal(t, cfg1.Listen, cfg2.Listen)
	assert.Equal(t, cfg1.DBPath, cfg2.DBPath)
}

// ---- SeedAdminUser tests ---------------------------------------------------

func TestSeedAdminUser_CreatesAdminWhenNoneExist(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	err := server.SeedAdminUser(ctx, db, "testadmin", "testpass", "testtoken")
	require.NoError(t, err)

	count, err := db.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSeedAdminUser_SkipsWhenAdminsExist(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	// Seed once.
	require.NoError(t, server.SeedAdminUser(ctx, db, "admin1", "pass1", "token1"))

	// Seed again - should be a no-op.
	require.NoError(t, server.SeedAdminUser(ctx, db, "admin2", "pass2", "token2"))

	count, err := db.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "second seed call should not create another admin")
}

func TestSeedAdminUser_UsesEnvVarCredentials(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	t.Setenv("TSM_ADMIN_USER", "envuser")
	t.Setenv("TSM_ADMIN_PASS", "envpass")
	t.Setenv("TSM_ADMIN_TOKEN", "envtoken123456789012345678901234")

	// Pass empty strings - should fall back to env vars.
	err := server.SeedAdminUser(ctx, db, "", "", "")
	require.NoError(t, err)

	count, err := db.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSeedAdminUser_DefaultsToAdminUsername(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	// No explicit user, no env var - should default to "admin".
	t.Setenv("TSM_ADMIN_USER", "")
	err := server.SeedAdminUser(ctx, db, "", "somepass", "sometoken")
	require.NoError(t, err)

	count, err := db.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSeedAdminUser_GeneratesRandomCredentialsWhenNoneProvided(t *testing.T) {
	db1 := newTestStore(t)
	db2 := newTestStore(t)
	ctx := context.Background()

	// Unset env vars so auto-generation is triggered.
	t.Setenv("TSM_ADMIN_PASS", "")
	t.Setenv("TSM_ADMIN_TOKEN", "")

	require.NoError(t, server.SeedAdminUser(ctx, db1, "admin", "", ""))
	require.NoError(t, server.SeedAdminUser(ctx, db2, "admin", "", ""))

	// Both stores should have an admin (different random credentials).
	count1, _ := db1.CountAdmins(ctx)
	count2, _ := db2.CountAdmins(ctx)
	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}

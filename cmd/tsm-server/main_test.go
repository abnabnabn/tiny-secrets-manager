package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"tiny-secrets-manager/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	masterKey := make([]byte, 32)
	masterKeyB64 := base64.StdEncoding.EncodeToString(masterKey)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	st, err := store.New(dbPath, masterKeyB64, "", logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	return st
}

func TestSeedBackupTarget(t *testing.T) {
	ctx := context.Background()

	t.Run("valid target trimmed and stored", func(t *testing.T) {
		db := newTestStore(t)
		err := seedBackupTarget(ctx, db, "  /var/backups  ")
		require.NoError(t, err)

		val, err := db.GetSetting(ctx, "backup_target")
		require.NoError(t, err)
		assert.Equal(t, "/var/backups", val)
	})

	t.Run("dash-prefixed target rejected", func(t *testing.T) {
		db := newTestStore(t)
		err := seedBackupTarget(ctx, db, "-oProxyCommand=touch /tmp/pwn")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot start with a dash")

		val, err := db.GetSetting(ctx, "backup_target")
		require.NoError(t, err)
		assert.Empty(t, val)
	})

	t.Run("fallback to env variable", func(t *testing.T) {
		db := newTestStore(t)
		os.Setenv("TSM_BACKUP_TARGET", " user@remote:/path ")
		t.Cleanup(func() { os.Unsetenv("TSM_BACKUP_TARGET") })

		err := seedBackupTarget(ctx, db, "")
		require.NoError(t, err)

		val, err := db.GetSetting(ctx, "backup_target")
		require.NoError(t, err)
		assert.Equal(t, "user@remote:/path", val)
	})

	t.Run("empty target does nothing", func(t *testing.T) {
		db := newTestStore(t)
		os.Unsetenv("TSM_BACKUP_TARGET")

		err := seedBackupTarget(ctx, db, "")
		require.NoError(t, err)

		val, err := db.GetSetting(ctx, "backup_target")
		require.NoError(t, err)
		assert.Empty(t, val)
	})
}

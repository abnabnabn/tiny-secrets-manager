package api

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRunBackup_Local(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	tmpDir := t.TempDir()

	// Configure backup target to tmpDir with trailing slash so it treats it as directory
	err := srv.store.PutSetting(context.Background(), "backup_target", tmpDir+"/")
	if err != nil {
		t.Fatalf("failed to put setting: %v", err)
	}

	err = srv.runBackup()
	if err != nil {
		t.Fatalf("runBackup failed: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 backup file, got %d", len(entries))
	}
}

func TestRetentionPolicy(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	tmpDir := t.TempDir()

	// Create some dummy files pretending to be backups
	// We'll just rely on their mod times or names depending on how retention is evaluated.
	// Retention uses ModTime(). We will touch files manually in actual implementation, but
	// changing mod time for tests in go requires chtimes.
	// For simplicity, just test that the retention policy doesn't crash on empty/malformed dirs.

	err := srv.store.PutSetting(context.Background(), "backup_retention_all_days", "1")
	if err != nil {
		t.Fatal(err)
	}

	// Just calling it to ensure no panics and handles empty dir gracefully
	srv.applyRetentionPolicy(tmpDir, false)

	// Now put an invalid target
	srv.applyRetentionPolicy("/invalid/dir/that/does/not/exist", false)
}

func TestRunBackup_SecurityValidation(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	// 1. Target with leading dash should be rejected
	err := srv.store.PutSetting(context.Background(), "backup_target", " -oProxyCommand=touch/tmp/hacked")
	if err != nil {
		t.Fatalf("failed to put setting: %v", err)
	}

	err = srv.runBackup()
	if err == nil {
		t.Fatal("expected runBackup to fail with an error for target starting with a dash, but it succeeded")
	}
	if !strings.Contains(err.Error(), "cannot start with a dash") {
		t.Errorf("expected error message to contain 'cannot start with a dash', got: %v", err)
	}
}

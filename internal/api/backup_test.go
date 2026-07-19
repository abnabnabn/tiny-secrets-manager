package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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

	// Put retention settings
	err := srv.store.PutSetting(context.Background(), "backup_retention_all_days", "1")
	if err != nil {
		t.Fatal(err)
	}
	err = srv.store.PutSetting(context.Background(), "backup_retention_daily_days", "2")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	// Define test cases: filename, modification time, shouldExist after policy run
	type mockBackup struct {
		name        string
		modTime     time.Time
		isDir       bool
		shouldExist bool
	}

	baseDaily := now.AddDate(0, 0, -2)
	baseOld := now.AddDate(0, 0, -4)

	mocks := []mockBackup{
		// 1. Keep All (age < 1 day)
		{
			name:        "tsm_backup_recent.db",
			modTime:     now.Add(-2 * time.Hour),
			shouldExist: true,
		},
		// 2. Daily bucket range (1 < age <= 3 days)
		// We have two files on the same day.
		// file_daily_1 (10:00:00) is older, should be deleted.
		// file_daily_2 (11:00:00) is newer, should be kept.
		{
			name:        "tsm_backup_daily1.db",
			modTime:     time.Date(baseDaily.Year(), baseDaily.Month(), baseDaily.Day(), 10, 0, 0, 0, now.Location()),
			shouldExist: false,
		},
		{
			name:        "tsm_backup_daily2.db",
			modTime:     time.Date(baseDaily.Year(), baseDaily.Month(), baseDaily.Day(), 11, 0, 0, 0, now.Location()),
			shouldExist: true,
		},
		// 3. Old backup (age > 3 days) -> should be deleted
		{
			name:        "tsm_backup_old.db",
			modTime:     time.Date(baseOld.Year(), baseOld.Month(), baseOld.Day(), 12, 0, 0, 0, now.Location()),
			shouldExist: false,
		},
		// 4. Ignored files
		// Non-backup name pattern -> should be untouched (exist) even if old
		{
			name:        "not_backup.db",
			modTime:     time.Date(baseOld.Year(), baseOld.Month(), baseOld.Day(), 12, 0, 0, 0, now.Location()),
			shouldExist: true,
		},
		// Ignored extension -> should be untouched (exist) even if old
		{
			name:        "tsm_backup_ignored.txt",
			modTime:     time.Date(baseOld.Year(), baseOld.Month(), baseOld.Day(), 12, 0, 0, 0, now.Location()),
			shouldExist: true,
		},
		// A directory with backup prefix -> should not be touched
		{
			name:        "tsm_backup_dir",
			modTime:     time.Date(baseOld.Year(), baseOld.Month(), baseOld.Day(), 12, 0, 0, 0, now.Location()),
			isDir:       true,
			shouldExist: true,
		},
	}

	// Create the mock files and directories
	for _, m := range mocks {
		path := filepath.Join(tmpDir, m.name)
		if m.isDir {
			err := os.Mkdir(path, 0755)
			if err != nil {
				t.Fatalf("failed to create directory %s: %v", m.name, err)
			}
		} else {
			err := os.WriteFile(path, []byte("dummy content"), 0644)
			if err != nil {
				t.Fatalf("failed to write file %s: %v", m.name, err)
			}
		}

		// Apply modification time using Chtimes
		err = os.Chtimes(path, m.modTime, m.modTime)
		if err != nil {
			t.Fatalf("failed to set modification time for %s: %v", m.name, err)
		}
	}

	// Run retention policy
	srv.applyRetentionPolicy(tmpDir, false)

	// Verify the result
	for _, m := range mocks {
		path := filepath.Join(tmpDir, m.name)
		_, err := os.Stat(path)
		exists := err == nil

		if m.shouldExist && !exists {
			t.Errorf("expected backup %s to exist, but it was deleted", m.name)
		} else if !m.shouldExist && exists {
			t.Errorf("expected backup %s to be deleted, but it still exists", m.name)
		}
	}

	// Now put an invalid target to verify it doesn't panic
	srv.applyRetentionPolicy("/invalid/dir/that/does/not/exist", false)
}

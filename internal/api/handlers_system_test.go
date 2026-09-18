package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemHandlers(t *testing.T) {
	_, db, mux, adminToken := setupTestServer(t)
	sharedTmpDir := t.TempDir()

	t.Run("GetSettings_Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/system/settings", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("PutSettings_Success", func(t *testing.T) {
		reqBody := map[string]string{
			"backup_target":               sharedTmpDir,
			"backup_interval_mins":        "10",
			"backup_retention_all_days":   "5",
			"backup_retention_daily_days": "15",
			"auto_populate_env_name":      "false",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/v1/system/settings", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		// Verify GET
		reqGet := httptest.NewRequest("GET", "/v1/system/settings", nil)
		reqGet.Header.Set("Authorization", "Bearer "+adminToken)
		wGet := httptest.NewRecorder()
		mux.ServeHTTP(wGet, reqGet)

		var res map[string]string
		_ = json.Unmarshal(wGet.Body.Bytes(), &res)
		if res["backup_target"] != sharedTmpDir {
			t.Errorf("expected backup_target to be set to %s, got %s", sharedTmpDir, res["backup_target"])
		}
		if res["backup_interval_mins"] != "10" {
			t.Errorf("expected backup_interval_mins to be 10")
		}
		if res["backup_retention_all_days"] != "5" {
			t.Errorf("expected backup_retention_all_days to be 5")
		}
		if res["backup_retention_daily_days"] != "15" {
			t.Errorf("expected backup_retention_daily_days to be 15")
		}
		if res["auto_populate_env_name"] != "false" {
			t.Errorf("expected auto_populate_env_name to be false")
		}
	})

	t.Run("PutSettings_ValidationErrors", func(t *testing.T) {
		testCases := []struct {
			name    string
			payload map[string]string
		}{
			{
				name:    "invalid key",
				payload: map[string]string{"non_existent_key": "some_value"},
			},
			{
				name:    "backup target starting with dash",
				payload: map[string]string{"backup_target": "-invalid-dir"},
			},
			{
				name:    "backup_interval_mins too low",
				payload: map[string]string{"backup_interval_mins": "0"},
			},
			{
				name:    "backup_interval_mins negative",
				payload: map[string]string{"backup_interval_mins": "-1"},
			},
			{
				name:    "backup_interval_mins non-numeric",
				payload: map[string]string{"backup_interval_mins": "abc"},
			},
			{
				name:    "backup_retention_all_days negative",
				payload: map[string]string{"backup_retention_all_days": "-1"},
			},
			{
				name:    "backup_retention_all_days non-numeric",
				payload: map[string]string{"backup_retention_all_days": "abc"},
			},
			{
				name:    "backup_retention_daily_days negative",
				payload: map[string]string{"backup_retention_daily_days": "-5"},
			},
			{
				name:    "backup_retention_daily_days non-numeric",
				payload: map[string]string{"backup_retention_daily_days": "xyz"},
			},
			{
				name:    "auto_populate_env_name invalid value",
				payload: map[string]string{"auto_populate_env_name": "yes"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				body, _ := json.Marshal(tc.payload)
				req := httptest.NewRequest("PUT", "/v1/system/settings", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+adminToken)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)

				if w.Code != http.StatusBadRequest {
					t.Errorf("expected 400 Bad Request for case %q, got %d", tc.name, w.Code)
				}
			})
		}
	})

	t.Run("PutSettings_ValidationSuccess", func(t *testing.T) {
		payloads := []map[string]string{
			{"auto_populate_env_name": "true"},
			{"auto_populate_env_name": "false"},
			{"backup_retention_all_days": "0"},
			{"backup_retention_daily_days": "0"},
			{"backup_interval_mins": "1"},
			{"backup_target": "  /valid/path  "},
		}

		for idx, p := range payloads {
			body, _ := json.Marshal(p)
			req := httptest.NewRequest("PUT", "/v1/system/settings", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+adminToken)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("payload %d: expected 200 OK, got %d", idx, w.Code)
			}
		}
	})

	t.Run("TriggerBackup_Success", func(t *testing.T) {
		backupDir := t.TempDir()
		ctx := context.Background()
		if err := db.PutSetting(ctx, "backup_target", backupDir); err != nil {
			t.Fatalf("failed to set backup_target: %v", err)
		}

		req := httptest.NewRequest("POST", "/v1/system/backup", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		// Verify that a backup file was created
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			t.Fatalf("failed to read backup dir: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 backup file, got %d", len(entries))
		}
	})

	t.Run("TriggerBackup_ForbiddenForNonAdmin", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/system/backup", nil)
		// No authorization header (or non-admin)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
			t.Errorf("expected 403 or 401, got %d", w.Code)
		}
	})

	t.Run("TriggerBackup_Failure_InvalidPath", func(t *testing.T) {
		// Configure an unwritable backup target by using a child path of a file
		tempFile := filepath.Join(t.TempDir(), "not_a_dir")
		if err := os.WriteFile(tempFile, []byte("file"), 0600); err != nil {
			t.Fatalf("failed to create dummy file: %v", err)
		}
		invalidPath := filepath.Join(tempFile, "cannot_create_dir")

		ctx := context.Background()
		if err := db.PutSetting(ctx, "backup_target", invalidPath); err != nil {
			t.Fatalf("failed to set backup_target: %v", err)
		}

		req := httptest.NewRequest("POST", "/v1/system/backup", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	})

	t.Run("ForbiddenForNonAdmin", func(t *testing.T) {
		// For forbidden, we just test without a token
		req := httptest.NewRequest("GET", "/v1/system/settings", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
			t.Errorf("expected 403 or 401, got %d", w.Code)
		}
	})
}

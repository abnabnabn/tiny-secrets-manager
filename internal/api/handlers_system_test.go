package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
			"backup_target":        sharedTmpDir,
			"backup_interval_mins": "10",
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
		if res["backup_target"] == "" {
			t.Errorf("expected backup_target to be set")
		}
	})

	t.Run("PutSettings_Validation", func(t *testing.T) {
		tests := []struct {
			name           string
			reqBody        map[string]string
			expectedStatus int
		}{
			{
				name: "invalid key",
				reqBody: map[string]string{
					"invalid_key_name": "some_value",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_target starts with dash",
				reqBody: map[string]string{
					"backup_target": " -some-flag",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_interval_mins invalid string",
				reqBody: map[string]string{
					"backup_interval_mins": "not-an-int",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_interval_mins less than 1",
				reqBody: map[string]string{
					"backup_interval_mins": "0",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_retention_all_days invalid string",
				reqBody: map[string]string{
					"backup_retention_all_days": "not-an-int",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_retention_all_days less than 0",
				reqBody: map[string]string{
					"backup_retention_all_days": "-1",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_retention_daily_days invalid string",
				reqBody: map[string]string{
					"backup_retention_daily_days": "not-an-int",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "backup_retention_daily_days less than 0",
				reqBody: map[string]string{
					"backup_retention_daily_days": "-5",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "auto_populate_env_name invalid value",
				reqBody: map[string]string{
					"auto_populate_env_name": "yes",
				},
				expectedStatus: http.StatusBadRequest,
			},
			{
				name: "auto_populate_env_name valid true",
				reqBody: map[string]string{
					"auto_populate_env_name": "true",
				},
				expectedStatus: http.StatusOK,
			},
			{
				name: "auto_populate_env_name valid false",
				reqBody: map[string]string{
					"auto_populate_env_name": "false",
				},
				expectedStatus: http.StatusOK,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				body, _ := json.Marshal(tc.reqBody)
				req := httptest.NewRequest("PUT", "/v1/system/settings", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+adminToken)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)

				if w.Code != tc.expectedStatus {
					t.Errorf("expected %d, got %d", tc.expectedStatus, w.Code)
				}
			})
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
		// Configure an invalid/unwritable backup target
		ctx := context.Background()
		if err := db.PutSetting(ctx, "backup_target", "/invalid/nonexistent/directory/that/cannot/be/created/or/written"); err != nil {
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

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

	t.Run("PutSettings_ValidationFailure", func(t *testing.T) {
		testCases := []struct {
			name        string
			payload     map[string]string
			expectedMsg string
		}{
			{
				name: "invalid_key",
				payload: map[string]string{
					"unknown_key": "some_value",
				},
				expectedMsg: "unsupported setting key: unknown_key",
			},
			{
				name: "backup_target_with_dash",
				payload: map[string]string{
					"backup_target": "-invalid-path",
				},
				expectedMsg: "invalid backup_target: cannot start with a dash",
			},
			{
				name: "backup_interval_mins_invalid_string",
				payload: map[string]string{
					"backup_interval_mins": "abc",
				},
				expectedMsg: "invalid backup_interval_mins: must be an integer >= 1",
			},
			{
				name: "backup_interval_mins_zero",
				payload: map[string]string{
					"backup_interval_mins": "0",
				},
				expectedMsg: "invalid backup_interval_mins: must be an integer >= 1",
			},
			{
				name: "backup_retention_all_days_negative",
				payload: map[string]string{
					"backup_retention_all_days": "-5",
				},
				expectedMsg: "invalid backup_retention_all_days: must be an integer >= 0",
			},
			{
				name: "backup_retention_daily_days_invalid_string",
				payload: map[string]string{
					"backup_retention_daily_days": "xyz",
				},
				expectedMsg: "invalid backup_retention_daily_days: must be an integer >= 0",
			},
			{
				name: "auto_populate_env_name_invalid_value",
				payload: map[string]string{
					"auto_populate_env_name": "yes",
				},
				expectedMsg: "invalid auto_populate_env_name: must be 'true' or 'false'",
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
					t.Errorf("expected 400, got %d for case %s", w.Code, tc.name)
				}

				var errResp ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}

				if errResp.Error != tc.expectedMsg {
					t.Errorf("expected error message %q, got %q", tc.expectedMsg, errResp.Error)
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

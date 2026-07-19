package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tiny-secrets-manager/internal/config"
	"tiny-secrets-manager/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRoleLifecycle(t *testing.T) {
	_, db, mux, adminToken := setupTestServer(t)
	defer db.Close()

	// 1. Create Role
	t.Run("create_role", func(t *testing.T) {
		body := map[string]interface{}{
			"name": "test-machine",
			"policies": []config.Policy{
				{Prefix: "app.*", Methods: []string{"GET"}},
			},
		}
		b, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/roles", bytes.NewBuffer(b))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp["token"])
	})

	// 2. List Roles
	t.Run("list_roles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/roles", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var tokens []store.RoleRecord
		err := json.NewDecoder(rec.Body).Decode(&tokens)
		require.NoError(t, err)

		// admin and test-machine (admin is filtered out though)
		require.Len(t, tokens, 1)
		assert.Equal(t, "test-machine", tokens[0].Name)
	})

	// 3. Update Role
	t.Run("update_role", func(t *testing.T) {
		body := map[string]interface{}{
			"policies": []config.Policy{
				{Prefix: "*", Methods: []string{"GET", "PUT"}},
			},
		}
		b, _ := json.Marshal(body)

		req := httptest.NewRequest("PUT", "/v1/roles/test-machine", bytes.NewBuffer(b))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	// 4. Regenerate Role
	t.Run("regenerate_role", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/roles/test-machine/regenerate", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp["token"])
	})

	// 5. Delete Role
	t.Run("delete_role", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/roles/test-machine", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestHandleRegenerateRecoveryKeys(t *testing.T) {
	_, db, mux, adminToken := setupTestServer(t)
	defer db.Close()

	t.Run("success_admin", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/recovery-keys/regenerate", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string][]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)

		keys := resp["recovery_keys"]
		require.Len(t, keys, 3)
		for _, k := range keys {
			assert.NotEmpty(t, k)
		}
	})

	t.Run("forbidden_non_admin", func(t *testing.T) {
		nonAdminToken := "non-admin-role-token"
		tokenHash := sha256.Sum256([]byte(nonAdminToken))
		pJSON, _ := json.Marshal([]config.Policy{{Prefix: "*", Methods: []string{"GET"}}})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := db.PutRole(ctx, "non-admin-role", tokenHash[:], pJSON, false, false, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v1/recovery-keys/regenerate", nil)
		req.Header.Set("Authorization", "Bearer "+nonAdminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp ErrorResponse
		err = json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "forbidden", resp.Error)
		assert.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("unauthorized_missing_token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/recovery-keys/regenerate", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp ErrorResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "unauthorized", resp.Error)
		assert.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("unauthorized_invalid_token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/recovery-keys/regenerate", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp ErrorResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "unauthorized", resp.Error)
		assert.Equal(t, http.StatusUnauthorized, resp.Status)
	})
}

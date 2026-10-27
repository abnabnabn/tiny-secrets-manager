package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"tiny-secrets-manager/internal/config"
	"tiny-secrets-manager/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRoleLifecycle(t *testing.T) {
	_, db, mux, adminToken := setupTestServer(t)
	defer db.Close()

	var roleToken string

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
		roleToken = resp["token"]

		// Verify role token works
		reqMe := httptest.NewRequest("GET", "/v1/auth/me", nil)
		reqMe.Header.Set("Authorization", "Bearer "+roleToken)
		recMe := httptest.NewRecorder()
		mux.ServeHTTP(recMe, reqMe)
		assert.Equal(t, http.StatusOK, recMe.Code)
		var client Client
		err = json.NewDecoder(recMe.Body).Decode(&client)
		require.NoError(t, err)
		assert.Equal(t, "test-machine", client.Name)
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
		// Non-admin token cannot regenerate roles
		reqForbidden := httptest.NewRequest("POST", "/v1/roles/test-machine/regenerate", nil)
		reqForbidden.Header.Set("Authorization", "Bearer "+roleToken)
		recForbidden := httptest.NewRecorder()
		mux.ServeHTTP(recForbidden, reqForbidden)
		assert.Equal(t, http.StatusForbidden, recForbidden.Code)

		// Admin can regenerate roles
		req := httptest.NewRequest("POST", "/v1/roles/test-machine/regenerate", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		newToken := resp["token"]
		assert.NotEmpty(t, newToken)

		// Verify old token is now unauthorized
		reqOld := httptest.NewRequest("GET", "/v1/auth/me", nil)
		reqOld.Header.Set("Authorization", "Bearer "+roleToken)
		recOld := httptest.NewRecorder()
		mux.ServeHTTP(recOld, reqOld)
		assert.Equal(t, http.StatusUnauthorized, recOld.Code)

		// Verify new token works
		reqNew := httptest.NewRequest("GET", "/v1/auth/me", nil)
		reqNew.Header.Set("Authorization", "Bearer "+newToken)
		recNew := httptest.NewRecorder()
		mux.ServeHTTP(recNew, reqNew)
		assert.Equal(t, http.StatusOK, recNew.Code)
		var client Client
		err = json.NewDecoder(recNew.Body).Decode(&client)
		require.NoError(t, err)
		assert.Equal(t, "test-machine", client.Name)

		// Update roleToken for subsequent steps if any
		roleToken = newToken
	})

	// 5. Rename Role
	t.Run("rename_role", func(t *testing.T) {
		// Renaming to protected role should fail
		bodyBad := map[string]interface{}{
			"name": "admin",
			"policies": []config.Policy{
				{Prefix: "*", Methods: []string{"GET"}},
			},
		}
		bBad, _ := json.Marshal(bodyBad)
		reqBad := httptest.NewRequest("PUT", "/v1/roles/test-machine", bytes.NewBuffer(bBad))
		reqBad.Header.Set("Authorization", "Bearer "+adminToken)
		recBad := httptest.NewRecorder()
		mux.ServeHTTP(recBad, reqBad)
		assert.Equal(t, http.StatusBadRequest, recBad.Code)

		// Create another role to test conflict
		createBody := map[string]interface{}{
			"name": "other-machine",
			"policies": []config.Policy{
				{Prefix: "*", Methods: []string{"GET"}},
			},
		}
		bCreate, _ := json.Marshal(createBody)
		reqCreate := httptest.NewRequest("POST", "/v1/roles", bytes.NewBuffer(bCreate))
		reqCreate.Header.Set("Authorization", "Bearer "+adminToken)
		recCreate := httptest.NewRecorder()
		mux.ServeHTTP(recCreate, reqCreate)
		assert.Equal(t, http.StatusOK, recCreate.Code)

		// Renaming to already existing role name should return Conflict (409)
		bodyConflict := map[string]interface{}{
			"name": "other-machine",
			"policies": []config.Policy{
				{Prefix: "*", Methods: []string{"GET"}},
			},
		}
		bConflict, _ := json.Marshal(bodyConflict)
		reqConflict := httptest.NewRequest("PUT", "/v1/roles/test-machine", bytes.NewBuffer(bConflict))
		reqConflict.Header.Set("Authorization", "Bearer "+adminToken)
		recConflict := httptest.NewRecorder()
		mux.ServeHTTP(recConflict, reqConflict)
		assert.Equal(t, http.StatusConflict, recConflict.Code)

		// Delete other-machine
		reqDelOther := httptest.NewRequest("DELETE", "/v1/roles/other-machine", nil)
		reqDelOther.Header.Set("Authorization", "Bearer "+adminToken)
		recDelOther := httptest.NewRecorder()
		mux.ServeHTTP(recDelOther, reqDelOther)
		assert.Equal(t, http.StatusNoContent, recDelOther.Code)

		// Valid rename
		body := map[string]interface{}{
			"name": "test-machine-renamed",
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

		// Verify auth/me returns the new name
		reqMe := httptest.NewRequest("GET", "/v1/auth/me", nil)
		reqMe.Header.Set("Authorization", "Bearer "+roleToken)
		recMe := httptest.NewRecorder()
		mux.ServeHTTP(recMe, reqMe)
		assert.Equal(t, http.StatusOK, recMe.Code)
		var client Client
		err := json.NewDecoder(recMe.Body).Decode(&client)
		require.NoError(t, err)
		assert.Equal(t, "test-machine-renamed", client.Name)
	})

	// 6. Delete Role
	t.Run("delete_role", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/roles/test-machine-renamed", nil)
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

func TestHandleProtectedRoles(t *testing.T) {
	_, db, mux, adminToken := setupTestServer(t)
	defer db.Close()

	protectedNames := []string{"admin", "session_user_12345", " admin ", " session_user_12345 ", "admin\t"}

	for _, name := range protectedNames {
		t.Run("create_protected_"+name, func(t *testing.T) {
			body := map[string]interface{}{
				"name": name,
				"policies": []config.Policy{
					{Prefix: "*", Methods: []string{"GET"}},
				},
			}
			b, _ := json.Marshal(body)

			req := httptest.NewRequest("POST", "/v1/roles", bytes.NewBuffer(b))
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("update_protected_"+name, func(t *testing.T) {
			body := map[string]interface{}{
				"policies": []config.Policy{
					{Prefix: "*", Methods: []string{"GET"}},
				},
			}
			b, _ := json.Marshal(body)

			req := httptest.NewRequest("PUT", "/v1/roles/"+url.PathEscape(name), bytes.NewBuffer(b))
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("delete_protected_"+name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/v1/roles/"+url.PathEscape(name), nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("regenerate_protected_"+name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/roles/"+url.PathEscape(name)+"/regenerate", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

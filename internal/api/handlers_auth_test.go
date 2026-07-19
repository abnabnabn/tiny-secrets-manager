package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tiny-secrets-manager/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleLogin(t *testing.T) {
	_, db, mux, _ := setupTestServer(t)
	defer db.Close()

	t.Run("success", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
			"password": "testpass",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(b))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Ensure cookie is set
		cookies := rec.Result().Cookies()
		require.Len(t, cookies, 1)
		assert.Equal(t, "tsm_admin", cookies[0].Name)
		assert.NotEmpty(t, cookies[0].Value)
	})

	t.Run("invalid_credentials", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
			"password": "wrongpassword",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(b))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestHandleLogout(t *testing.T) {
	_, db, mux, _ := setupTestServer(t)
	defer db.Close()

	req := httptest.NewRequest("POST", "/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "tsm_admin", cookies[0].Name)
	assert.Equal(t, "", cookies[0].Value)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestHandleAuthMe(t *testing.T) {
	_, db, mux, adminToken := setupTestServer(t)
	defer db.Close()

	t.Run("success_admin_bearer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var client Client
		err := json.NewDecoder(rec.Body).Decode(&client)
		require.NoError(t, err)

		assert.Equal(t, "admin", client.Name)
		assert.True(t, client.IsAdmin)
		assert.False(t, client.CanCreate)
		require.Len(t, client.Policies, 1)
		assert.Equal(t, "*", client.Policies[0].Prefix)
	})

	t.Run("success_cookie_auth", func(t *testing.T) {
		cookieToken := "test-cookie-token"
		tokenHash := sha256.Sum256([]byte(cookieToken))
		pJSON, _ := json.Marshal([]config.Policy{{Prefix: "app.*", Methods: []string{"GET"}}})
		err := db.PutRole(context.Background(), "cookie-role", tokenHash[:], pJSON, false, true, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{
			Name:  "tsm_admin",
			Value: cookieToken,
		})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var client Client
		err = json.NewDecoder(rec.Body).Decode(&client)
		require.NoError(t, err)

		assert.Equal(t, "cookie-role", client.Name)
		assert.False(t, client.IsAdmin)
		assert.True(t, client.CanCreate)
		require.Len(t, client.Policies, 1)
		assert.Equal(t, "app.*", client.Policies[0].Prefix)
	})

	t.Run("success_non_admin_role", func(t *testing.T) {
		roleToken := "test-role-token"
		tokenHash := sha256.Sum256([]byte(roleToken))
		pJSON, _ := json.Marshal([]config.Policy{{Prefix: "app.db.pass", Methods: []string{"GET", "PUT"}}})
		err := db.PutRole(context.Background(), "my-custom-role", tokenHash[:], pJSON, false, false, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+roleToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var client Client
		err = json.NewDecoder(rec.Body).Decode(&client)
		require.NoError(t, err)

		assert.Equal(t, "my-custom-role", client.Name)
		assert.False(t, client.IsAdmin)
		assert.False(t, client.CanCreate)
		require.Len(t, client.Policies, 1)
		assert.Equal(t, "app.db.pass", client.Policies[0].Prefix)
		assert.Equal(t, []string{"GET", "PUT"}, client.Policies[0].Methods)
	})

	t.Run("unauthorized_missing_token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("unauthorized_invalid_token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-here")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("unauthorized_expired_token", func(t *testing.T) {
		expiredToken := "test-expired-token"
		tokenHash := sha256.Sum256([]byte(expiredToken))
		pJSON, _ := json.Marshal([]config.Policy{{Prefix: "*", Methods: []string{"*"}}})
		expiredAt := time.Now().Add(-1 * time.Hour)
		err := db.PutRole(context.Background(), "expired-role", tokenHash[:], pJSON, false, false, &expiredAt)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("success_impersonation", func(t *testing.T) {
		// Create target role to impersonate
		pJSON, _ := json.Marshal([]config.Policy{{Prefix: "app.*", Methods: []string{"GET"}}})
		// We insert it with a dummy token hash since we will impersonate by name
		dummyHash := sha256.Sum256([]byte("impersonated-dummy-token"))
		err := db.PutRole(context.Background(), "impersonated-role", dummyHash[:], pJSON, false, true, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("X-Impersonate-Token", "impersonated-role")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var client Client
		err = json.NewDecoder(rec.Body).Decode(&client)
		require.NoError(t, err)

		assert.Equal(t, "impersonated-role", client.Name)
		assert.False(t, client.IsAdmin)
		assert.True(t, client.CanCreate)
		require.Len(t, client.Policies, 1)
		assert.Equal(t, "app.*", client.Policies[0].Prefix)
	})
}

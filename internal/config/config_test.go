package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars to ensure defaults
	os.Unsetenv("TSM_MASTER_KEY")
	os.Unsetenv("TSM_LISTEN")
	os.Unsetenv("TSM_DB_PATH")
	os.Unsetenv("TSM_INSECURE")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0:8090", cfg.Listen)
	assert.Equal(t, "tsm.db", cfg.DBPath)
	assert.Equal(t, "", cfg.MasterKey)
	assert.False(t, cfg.Insecure)
}

func TestLoad_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonData := `{
		"master_key": "json_master_key",
		"listen": "127.0.0.1:9090",
		"db_path": "json.db"
	}`
	err := os.WriteFile(configPath, []byte(jsonData), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, "json_master_key", cfg.MasterKey)
	assert.Equal(t, "127.0.0.1:9090", cfg.Listen)
	assert.Equal(t, "json.db", cfg.DBPath)
}

func TestLoad_EnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonData := `{
		"master_key": "json_master_key",
		"listen": "127.0.0.1:9090",
		"db_path": "json.db"
	}`
	err := os.WriteFile(configPath, []byte(jsonData), 0644)
	require.NoError(t, err)

	// Set overrides
	os.Setenv("TSM_MASTER_KEY", "env_master_key")
	os.Setenv("TSM_LISTEN", "localhost:8080")
	os.Setenv("TSM_INSECURE", "1")
	defer func() {
		os.Unsetenv("TSM_MASTER_KEY")
		os.Unsetenv("TSM_LISTEN")
		os.Unsetenv("TSM_INSECURE")
	}()

	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Overridden
	assert.Equal(t, "env_master_key", cfg.MasterKey)
	assert.Equal(t, "localhost:8080", cfg.Listen)
	assert.True(t, cfg.Insecure)

	// Not overridden
	assert.Equal(t, "json.db", cfg.DBPath)
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(configPath, []byte(`{invalid`), 0644)
	require.NoError(t, err)

	_, err = Load(configPath)
	require.Error(t, err)
}

func TestLoad_MissingFile(t *testing.T) {
	// Should ignore missing file error and just load defaults
	cfg, err := Load("/does/not/exist/config.json")
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8090", cfg.Listen)
}

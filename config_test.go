package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDefaultConfig(t *testing.T) {
	config := GetDefaultConfig()
	assert.Equal(t, 8080, config.ListenPortHTTP)
	assert.Equal(t, 443, config.ListenPortHTTPS)
}

func TestReadConfig_FromFile(t *testing.T) {
	// Unset DATABASE_URL to avoid interference
	t.Setenv("DATABASE_URL", "")

	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configContent := `{
		"hostWhitelist": ["example.com"],
		"listenPortHTTP": 8080,
		"listenPortHTTPS": 8443,
		"databaseType": "sqlite",
		"databasePath": "/tmp/test.db"
	}`

	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Read config
	config, err := ReadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, []string{"example.com"}, config.HostWhitelist)
	assert.Equal(t, 8080, config.ListenPortHTTP)
	assert.Equal(t, 8443, config.ListenPortHTTPS)
	assert.Equal(t, "sqlite", config.DatabaseType)
	assert.Equal(t, "/tmp/test.db", config.DatabasePath)
}

func TestReadConfig_FromEnvironmentVariable(t *testing.T) {
	// Set environment variable
	configJSON := `{"listenPortHTTP":9000,"listenPortHTTPS":9443,"databaseType":"postgres"}`
	t.Setenv("IPTHING_CONFIG", configJSON)

	// Read config
	config, err := ReadConfig("")
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, 9000, config.ListenPortHTTP)
	assert.Equal(t, 9443, config.ListenPortHTTPS)
	assert.Equal(t, "postgres", config.DatabaseType)
}

func TestReadConfig_EnvironmentOverrides(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configContent := `{
		"listenPortHTTP": 8080,
		"listenPortHTTPS": 8443,
		"databaseType": "sqlite"
	}`

	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Set environment overrides
	t.Setenv("IPTHING_HTTP_PORT", "9000")
	t.Setenv("IPTHING_HTTPS_PORT", "9443")
	t.Setenv("IPTHING_DB_TYPE", "postgres")

	// Read config
	config, err := ReadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Environment variables should override file values
	assert.Equal(t, 9000, config.ListenPortHTTP)
	assert.Equal(t, 9443, config.ListenPortHTTPS)
	assert.Equal(t, "postgres", config.DatabaseType)
}

func TestReadConfig_LegacyPortHandling(t *testing.T) {
	// Create temporary config file with legacy listenPort field
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configContent := `{
		"listenPort": 1323
	}`

	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Read config
	config, err := ReadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Legacy port should be applied to both HTTP and HTTPS
	assert.Equal(t, 1323, config.ListenPortHTTP)
	assert.Equal(t, 1323, config.ListenPortHTTPS)
}

func TestReadConfig_LegacyDatabaseURL(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configContent := `{}`

	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Set legacy DATABASE_URL environment variable
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")

	// Read config
	config, err := ReadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, config)

	// DATABASE_URL should be recognized and type set to postgres
	assert.Equal(t, "postgres://user:pass@localhost/db", config.DatabaseConnectionString)
	assert.Equal(t, "postgres", config.DatabaseType)
}

func TestReadConfig_MissingFileUsesEnv(t *testing.T) {
	t.Setenv("IPTHING_CONFIG", "")
	t.Setenv("IPTHING_HTTP_PORT", "80")
	t.Setenv("IPTHING_HTTPS_PORT", "443")
	t.Setenv("IPTHING_HOST_WHITELIST", "ipthing.net, test.ipthing.net")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/ipthing")

	config, err := ReadConfig("/nonexistent/config.json")
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, 80, config.ListenPortHTTP)
	assert.Equal(t, 443, config.ListenPortHTTPS)
	assert.Equal(t, []string{"ipthing.net", "test.ipthing.net"}, config.HostWhitelist)
	assert.Equal(t, "postgres://user:pass@localhost/ipthing", config.DatabaseConnectionString)
	assert.Equal(t, "postgres", config.DatabaseType)
}

func TestGetContentType(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"favicon.ico", "image/x-icon"},
		{"favicon-16x16.png", "image/png"},
		{"favicon-32x32.png", "image/png"},
		{"apple-touch-icon.png", "image/png"},
		{"site.webmanifest", "application/manifest+json"},
		{"unknown.txt", "application/octet-stream"},
	}

	for _, tt := range tests {
		result := getContentType(tt.filename)
		assert.Equal(t, tt.expected, result, "Filename %s should return %s", tt.filename, tt.expected)
	}
}

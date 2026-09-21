package main

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyslogWriter_Write(t *testing.T) {
	// Test with nil syslog writer (fallback mode)
	sw := &SyslogWriter{writer: nil}

	testMsg := "Test log message\n"
	n, err := sw.Write([]byte(testMsg))

	require.NoError(t, err)
	assert.Equal(t, len(testMsg), n)
}

func TestSetupLogger_NoSyslog(t *testing.T) {
	// Test logger setup when syslog is not available
	var buf bytes.Buffer

	writer := setupLogger(nil)
	require.NotNil(t, writer)

	// Temporarily redirect log output to capture it
	log.SetOutput(&buf)
	log.Print("test message")

	output := buf.String()
	assert.Contains(t, output, "test message")
}

func TestConfigureEchoLogger(t *testing.T) {
	e := echo.New()

	// Test with nil syslog (fallback to stdout)
	configureEchoLogger(e, nil)

	require.NotNil(t, e.Logger)
	assert.NotNil(t, e.Logger.Output())
}

func TestGetContentType_AllTypes(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"favicon.ico", "image/x-icon"},
		{"icon.png", "image/png"},
		{"manifest.webmanifest", "application/manifest+json"},
		{"robots.txt", "text/plain; charset=utf-8"},
		{"sitemap.xml", "application/xml; charset=utf-8"},
		{"unknown.file", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := getContentType(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplicationLogging(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(log.Writer())

	// Create minimal config for testing
	config := &Config{
		ListenPortHTTP:  8080,
		ListenPortHTTPS: 0, // Disable HTTPS for testing
	}

	// Test configuration logging
	logConfigSettings(config)

	output := buf.String()
	assert.Contains(t, output, "HTTP port: 8080")
	assert.Contains(t, output, "HTTPS port: 0")
}

func TestVersionVariable(t *testing.T) {
	// Verify version variable exists and has a default value
	assert.NotEmpty(t, version)

	// Default version should be "dev" unless overridden by build
	if !strings.Contains(version, "-") {
		assert.Equal(t, "dev", version)
	}
}

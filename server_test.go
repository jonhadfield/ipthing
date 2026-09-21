package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestProcessor(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:54321"
	// Spoofable headers must not override the TCP peer IP.
	req.Header.Set("Cf-Connecting-Ip", "192.168.1.1")
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "198.51.100.2")
	req.Header.Set("Cf-IPcountry", "US")
	req.Header.Set("Cf-Visitor", "Visitor")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("DNT", "1")
	req.Header.Set("Language", "en")
	req.Header.Set("Referer", "http://localhost")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Charset", "UTF-8")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, e.Logger)

	httpReq, _, err := rp.ProcessRequest(c)
	require.NoError(t, err)
	require.NotNil(t, httpReq)
	assert.Equal(t, "203.0.113.50", httpReq.IP)
	assert.Equal(t, "GET", httpReq.Method)
	assert.Equal(t, "/", httpReq.Path)
	assert.Equal(t, "http://localhost", httpReq.Referer)
}

func TestExtractClientIPIgnoresForwardingHeaders(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.99:12345"
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4")
	req.Header.Set("X-Forwarded-For", "5.6.7.8, 9.9.9.9")
	req.Header.Set("X-Real-IP", "10.0.0.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	rp := NewRequestProcessor(&NoOpDB{}, false, e.Logger)
	assert.Equal(t, "203.0.113.99", rp.extractClientIP(c))
}

func TestIsWebBrowser(t *testing.T) {
	tests := []struct {
		userAgent string
		expected  bool
	}{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", true},
		{"curl/7.68.0", false},
		{"wget/1.20.3", false},
		{"Lynx/2.8.9rel.1", true},
		{"", false},
	}

	for _, tt := range tests {
		result := IsWebBrowser(tt.userAgent)
		assert.Equal(t, tt.expected, result, "Failed for user agent: %s", tt.userAgent)
	}
}

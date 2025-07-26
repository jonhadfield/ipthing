package main

import (
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestProcessor(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cf-Connecting-Ip", "192.168.1.1")
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
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test RequestProcessor with NoOp database
	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, e.Logger)

	httpReq, _, err := rp.ProcessRequest(c)
	require.NoError(t, err)
	require.NotNil(t, httpReq)
	assert.Equal(t, "192.168.1.1", httpReq.IP)
	assert.Equal(t, "GET", httpReq.Method)
	assert.Equal(t, "/", httpReq.Path)
	assert.Equal(t, "http://localhost", httpReq.Referer)
}

func TestParseXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Add("X-Forwarded-For", "203.0.113.199, 192.168.1.100")
	addr, err := parseXForwardedFor(req)
	require.NoError(t, err)
	require.NotEmpty(t, addr)
	require.Equal(t, "203.0.113.199", addr)
}

func TestStripTrustedFromXFF(t *testing.T) {
	in := []struct {
		xff        string
		trustedPos int
		expected   string
	}{
		{
			xff:        "1.1.1.1,2.2.2.2,3.3.3.3",
			trustedPos: 1,
			expected:   "1.1.1.1,2.2.2.2",
		},
		{
			xff:        "1.1.1.1,6.6.6.6,192.168.4.50",
			trustedPos: 1,
			expected:   "1.1.1.1,6.6.6.6",
		},
		{
			xff:        "1.1.1.1,6.6.6.6:443,192.168.4.50",
			trustedPos: 2,
			expected:   "1.1.1.1",
		},
	}

	for x := range in {
		actual := stripXFFTrustedProxies(in[x].xff, in[x].trustedPos)
		require.Equal(t, in[x].expected, actual)
	}
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

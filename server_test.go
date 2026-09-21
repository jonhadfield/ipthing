package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestHeadersForStorageRedactsSecrets(t *testing.T) {
	h := http.Header{}
	h.Set("Accept", "application/json")
	h.Set("Cookie", "session=secret")
	h.Set("Authorization", "Bearer tok")
	h.Set("X-Api-Key", "key123")
	h.Set("X-Forwarded-For", "203.0.113.1")

	stored := headersForStorage(h)
	assert.Equal(t, []string{"application/json"}, stored["Accept"])
	assert.Equal(t, []string{"203.0.113.1"}, stored["X-Forwarded-For"])
	assert.Equal(t, []string{"[redacted]"}, stored["Cookie"])
	assert.Equal(t, []string{"[redacted]"}, stored["Authorization"])
	assert.Equal(t, []string{"[redacted]"}, stored["X-Api-Key"])
}

func TestBuildHTTPRequestRedactsHeadersAndSkipsBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/?q=1", strings.NewReader(`{"password":"secret"}`))
	req.RemoteAddr = "203.0.113.50:1"
	req.Header.Set("Cookie", "a=b")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 10.0.0.1")
	req.Header.Set("Cf-Connecting-Ip", "203.0.113.9")
	req.ContentLength = int64(len(`{"password":"secret"}`))

	rp := NewRequestProcessor(&NoOpDB{}, false, nil)
	httpReq := rp.buildHTTPRequest(req, "203.0.113.50")

	assert.Empty(t, httpReq.Body)
	assert.True(t, httpReq.HasCookies)
	assert.Equal(t, "198.51.100.1, 10.0.0.1", httpReq.ClaimedXFF)
	assert.Equal(t, "203.0.113.9", httpReq.CfConnectingIP)
	assert.Contains(t, httpReq.Headers, `"Cookie":["[redacted]"]`)
	assert.NotContains(t, httpReq.Headers, "session=secret")
	assert.NotContains(t, httpReq.Headers, "password")
}

type recordingDB struct {
	NoOpDB
	mu    sync.Mutex
	saved *HTTPRequest
}

func (r *recordingDB) SaveHTTPRequest(req *HTTPRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *req
	r.saved = &cp
	return nil
}

func (r *recordingDB) getSaved() *HTTPRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saved
}

func TestHandleRoot_SetsFormatAndDuration(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:54321"
	req.Header.Set("User-Agent", "curl/7.68.0")
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	db := &recordingDB{}
	rp := NewRequestProcessor(db, true, e.Logger)
	handler := NewHandler(rp)

	err := handler.HandleRoot(c)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return db.getSaved() != nil
	}, time.Second, 10*time.Millisecond)

	saved := db.getSaved()
	assert.Equal(t, "json", saved.ResponseFormat)
	assert.Equal(t, http.StatusOK, saved.StatusCode)
	assert.GreaterOrEqual(t, saved.DurationMs, int64(0))
	assert.Equal(t, "198.51.100.1", saved.ClaimedXFF)
	assert.False(t, saved.HasCookies)
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

func TestClacksOverheadMiddleware(t *testing.T) {
	e := echo.New()
	e.Use(clacksOverheadMiddleware())
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "GNU Terry Pratchett", rec.Header().Get("X-Clacks-Overhead"))
}

func TestRateLimitMiddleware_PerIP(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(rateLimitMiddleware())
	e.Any("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/favicon.ico", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	// Burst allows a short spike (10) before 429.
	for i := range 10 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.200:1"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d within burst should succeed", i+1)
	}

	reqDenied := httptest.NewRequest(http.MethodGet, "/", nil)
	reqDenied.RemoteAddr = "203.0.113.200:1"
	recDenied := httptest.NewRecorder()
	e.ServeHTTP(recDenied, reqDenied)
	assert.Equal(t, http.StatusTooManyRequests, recDenied.Code)

	// Different source is unaffected.
	reqOther := httptest.NewRequest(http.MethodGet, "/", nil)
	reqOther.RemoteAddr = "203.0.113.201:1"
	recOther := httptest.NewRecorder()
	e.ServeHTTP(recOther, reqOther)
	assert.Equal(t, http.StatusOK, recOther.Code)

	// Favicon is skipped even for the limited IP.
	reqFav := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	reqFav.RemoteAddr = "203.0.113.200:1"
	recFav := httptest.NewRecorder()
	e.ServeHTTP(recFav, reqFav)
	assert.Equal(t, http.StatusNoContent, recFav.Code)
}

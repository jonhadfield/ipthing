package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyLimit_RejectsOversizedContentLength(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	db := &recordingDB{}
	rp := NewRequestProcessor(db, true, e.Logger)
	app := &Application{handler: NewHandler(rp)}
	e.Use(app.requestBodyLimitMiddleware())
	e.Any("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	body := strings.NewReader("x")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.RemoteAddr = "203.0.113.50:1"
	req.Header.Set("User-Agent", "curl/8.0")
	req.ContentLength = maxRequestBodyBytes + 1
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "request body exceeds")

	require.Eventually(t, func() bool {
		return db.getSaved() != nil
	}, time.Second, 10*time.Millisecond)

	saved := db.getSaved()
	assert.Equal(t, http.StatusRequestEntityTooLarge, saved.StatusCode)
	assert.Equal(t, maxRequestBodyBytes+1, saved.ContentLength)
	assert.Equal(t, "error", saved.ResponseFormat)
	assert.Equal(t, "203.0.113.50", saved.IP)
}

func TestBodyLimit_RejectsOversizedChunkedBody(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	db := &recordingDB{}
	rp := NewRequestProcessor(db, true, e.Logger)
	app := &Application{handler: NewHandler(rp)}
	e.Use(app.requestBodyLimitMiddleware())
	e.Any("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	oversized := strings.Repeat("a", int(maxRequestBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(oversized))
	req.RemoteAddr = "203.0.113.51:1"
	req.Header.Set("User-Agent", "curl/8.0")
	req.ContentLength = -1 // unknown / chunked
	req.Body = io.NopCloser(strings.NewReader(oversized))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

	require.Eventually(t, func() bool {
		return db.getSaved() != nil
	}, time.Second, 10*time.Millisecond)

	saved := db.getSaved()
	assert.Equal(t, http.StatusRequestEntityTooLarge, saved.StatusCode)
	assert.Equal(t, maxRequestBodyBytes+1, saved.ContentLength)
}

func TestBodyLimit_AllowsBodyWithinLimit(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	app := &Application{handler: NewHandler(NewRequestProcessor(&NoOpDB{}, false, e.Logger))}
	e.Use(app.requestBodyLimitMiddleware())
	e.Any("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	payload := strings.Repeat("b", 1024)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.RemoteAddr = "203.0.113.52:1"
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBuildHTTPRequest_RecordsContentLength(t *testing.T) {
	payload := `{"probe":true}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/json")

	rp := NewRequestProcessor(&NoOpDB{}, false, nil)
	httpReq := rp.buildHTTPRequest(req, "203.0.113.50")

	assert.Equal(t, int64(len(payload)), httpReq.ContentLength)
	assert.Equal(t, "application/json", httpReq.ContentType)
	assert.Empty(t, httpReq.Body)
}

func TestHandleRoot_RecordsContentLengthAndStatus(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	payload := "hello"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.RemoteAddr = "203.0.113.50:54321"
	req.Header.Set("User-Agent", "curl/7.68.0")
	req.ContentLength = int64(len(payload))
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
	assert.Equal(t, int64(len(payload)), saved.ContentLength)
	assert.Equal(t, http.StatusOK, saved.StatusCode)
	assert.Equal(t, "json", saved.ResponseFormat)
}

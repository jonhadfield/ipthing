package main

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRoot_BrowserResponse(t *testing.T) {
	// Setup
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Create handler with embedded renderer
	renderer, err := NewEmbeddedRenderer()
	require.NoError(t, err)
	e.Renderer = renderer

	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, e.Logger)
	handler := NewHandler(rp)

	// Execute
	err = handler.HandleRoot(c)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<html lang=\"en\">")
}

func TestHandleRoot_JSONResponse(t *testing.T) {
	// Setup
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:54321"
	req.Header.Set("User-Agent", "curl/7.68.0")
	// Spoofable headers must not become IPAddr.
	req.Header.Set("Cf-Connecting-Ip", "198.51.100.1")
	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, e.Logger)
	handler := NewHandler(rp)

	// Execute
	err := handler.HandleRoot(c)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	// Verify JSON structure
	var responseData map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &responseData)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.50", responseData["IPAddr"])
	assert.Equal(t, "curl/7.68.0", responseData["UserAgent"])
	assert.Equal(t, "198.51.100.2", responseData["XFF"])
}

func TestRootAcceptsNonGETMethods(t *testing.T) {
	app := &Application{
		handler: NewHandler(NewRequestProcessor(&NoOpDB{}, false, nil)),
	}
	e := app.setupServer()

	for _, method := range []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodHead,
	} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/", strings.NewReader(`{"probe":true}`))
			req.RemoteAddr = "203.0.113.77:9"
			req.Header.Set("User-Agent", "scanner/1.0")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code, "method %s should not 405", method)
			if method != http.MethodHead {
				assert.Contains(t, rec.Body.String(), "203.0.113.77")
				assert.Contains(t, rec.Body.String(), method)
			}
		})
	}
}

func TestBuildResponseData_WithTLS(t *testing.T) {
	// Setup
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{
		Version:            tls.VersionTLS13,
		CipherSuite:        tls.TLS_AES_128_GCM_SHA256,
		ServerName:         "example.com",
		NegotiatedProtocol: "h2",
	}

	httpReq := &HTTPRequest{
		IP:                    "192.168.1.1",
		Method:                "GET",
		Path:                  "/",
		TLSVersion:            tls.VersionTLS13,
		TLSCipherSuite:        tls.TLS_AES_128_GCM_SHA256,
		TLSServerName:         "example.com",
		TLSNegotiatedProtocol: "h2",
	}

	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, nil)
	handler := NewHandler(rp)

	// Execute
	data := handler.buildResponseData(req, httpReq, nil)

	// Assert
	assert.Equal(t, "TLS 1.3", data["TLSVersion"])
	assert.Equal(t, "TLS_AES_128_GCM_SHA256", data["TLSCipherSuite"])
	assert.Equal(t, "example.com", data["TLSServerName"])
	assert.Equal(t, "h2", data["TLSNegotiatedProtocol"])
}

func TestBuildResponseData_WithCloudflareHeaders(t *testing.T) {
	// Setup
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cf-IPcountry", "US")
	req.Header.Set("Cf-Visitor", `{"scheme":"https"}`)

	httpReq := &HTTPRequest{
		IP:     "192.168.1.1",
		Method: "GET",
		Path:   "/",
	}

	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, nil)
	handler := NewHandler(rp)

	// Execute
	data := handler.buildResponseData(req, httpReq, nil)

	// Assert
	assert.Equal(t, "US", data["Country"])
	assert.Equal(t, `{"scheme":"https"}`, data["Visitor"])
}

func TestBuildResponseData_WithIPInfo(t *testing.T) {
	// Setup
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	httpReq := &HTTPRequest{
		IP:     "203.0.113.50",
		Method: "GET",
		Path:   "/",
	}

	ipInfo := &IPInfo{
		IP:      "203.0.113.50",
		Country: "GB",
		City:    "London",
		Org:     "Example ISP",
	}

	db := &NoOpDB{}
	rp := NewRequestProcessor(db, false, nil)
	handler := NewHandler(rp)

	// Execute
	data := handler.buildResponseData(req, httpReq, ipInfo)

	// Assert
	assert.Equal(t, "GB", data["Country"])
	assert.Equal(t, "London", data["City"])
	assert.Equal(t, "Example ISP", data["Org"])
}

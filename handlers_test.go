package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	body := rec.Body.String()
	assert.Contains(t, body, "<html lang=\"en\">")
	assert.Contains(t, body, "What is my IP address?")
	assert.Contains(t, body, `name="description"`)
	assert.Contains(t, body, `rel="canonical"`)
	assert.Contains(t, body, "curl https://ipthing.net")
}

func TestHandleRoot_BrowserEscapesXSSInUserAgent(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	renderer, err := NewEmbeddedRenderer()
	require.NoError(t, err)
	e.Renderer = renderer

	payload := `<script>alert(1)</script>`
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("User-Agent", "Mozilla/5.0 "+payload)
	req.Header.Set("Referer", "https://evil.example/"+payload)
	req.Header.Set("X-Forwarded-For", payload)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := NewHandler(NewRequestProcessor(&NoOpDB{}, false, e.Logger))
	require.NoError(t, handler.HandleRoot(c))

	body := rec.Body.String()
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, body, "<script>alert(1)</script>")
	assert.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	e := echo.New()
	e.Use(securityHeadersMiddleware())
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "default-src 'none'")
	assert.Contains(t, csp, "style-src 'unsafe-inline'")
	assert.Contains(t, csp, "img-src 'self'")
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
}

func TestPrivacyPage(t *testing.T) {
	renderer, err := NewEmbeddedRenderer()
	require.NoError(t, err)

	app := &Application{
		handler:  NewHandler(NewRequestProcessor(&NoOpDB{}, false, nil)),
		template: renderer,
	}
	e := app.setupServer()

	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Request bodies are not stored")
	assert.Contains(t, body, "[redacted]")
	assert.Contains(t, body, "retained indefinitely")
	assert.Contains(t, body, "stats.ipthing.net")
	assert.Contains(t, body, "jonhadfield/ipthing-analysis")
	assert.Contains(t, body, `rel="canonical"`)
	assert.Contains(t, body, "https://ipthing.net/privacy")
}

func TestNotFoundPathIsRecorded(t *testing.T) {
	renderer, err := NewEmbeddedRenderer()
	require.NoError(t, err)

	db := &recordingDB{}
	app := &Application{
		handler:  NewHandler(NewRequestProcessor(db, true, nil)),
		template: renderer,
	}
	e := app.setupServer()

	req := httptest.NewRequest(http.MethodGet, "/.env", nil)
	req.RemoteAddr = "203.0.113.77:9"
	req.Header.Set("User-Agent", "scanner/1.0")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	require.Eventually(t, func() bool {
		return db.getSaved() != nil
	}, time.Second, 10*time.Millisecond)

	saved := db.getSaved()
	assert.Equal(t, "/.env", saved.Path)
	assert.Equal(t, http.StatusNotFound, saved.StatusCode)
	assert.Equal(t, "error", saved.ResponseFormat)
	assert.Equal(t, "203.0.113.77", saved.IP)
}

func TestTruncateForLog(t *testing.T) {
	assert.Equal(t, "short", truncateForLog("short", 10))
	assert.Equal(t, "abcdefghij", truncateForLog("abcdefghijklmnopqrstuvwxyz", 10))
	assert.Equal(t, "abc", truncateForLog("abc", 0))
}

func TestSEOStaticRoutes(t *testing.T) {
	renderer, err := NewEmbeddedRenderer()
	require.NoError(t, err)

	app := &Application{
		handler:  NewHandler(NewRequestProcessor(&NoOpDB{}, false, nil)),
		template: renderer,
	}
	e := app.setupServer()

	t.Run("robots.txt", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
		body := rec.Body.String()
		assert.Contains(t, body, "User-agent: *")
		assert.Contains(t, body, "Sitemap: https://ipthing.net/sitemap.xml")
	})

	t.Run("sitemap.xml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/xml")
		body := rec.Body.String()
		assert.Contains(t, body, "https://ipthing.net/")
		assert.Contains(t, body, "https://ipthing.net/privacy")
		assert.NotContains(t, body, "stats.ipthing.net")
	})

	t.Run("site.webmanifest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/site.webmanifest", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/manifest+json")
		assert.Contains(t, rec.Body.String(), `"name": "IPThing"`)
	})
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

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodHead,
		http.MethodTrace,
		echo.PROPFIND,
		echo.REPORT,
		"QUERY", // RFC 10008; registered in extraRootMethods
		"COPY",
		"MOVE",
		"MKCOL",
		"LOCK",
		"UNLOCK",
		"PROPPATCH",
		"SEARCH",
		"PURGE",
		"ACL",
		"TOTALLY-MADE-UP", // not registered; must hit 405 fallback
	}

	for i, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{"probe":true}`))
			req.Method = method
			// Unique peer per method so the per-IP rate limiter does not interfere.
			req.RemoteAddr = fmt.Sprintf("203.0.113.%d:9", 10+i)
			req.Header.Set("User-Agent", "scanner/1.0")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code, "method %s should not 405", method)
			if method != http.MethodHead {
				assert.Contains(t, rec.Body.String(), fmt.Sprintf("203.0.113.%d", 10+i))
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

func TestWWWRedirectsToApex(t *testing.T) {
	renderer, err := NewEmbeddedRenderer()
	require.NoError(t, err)

	app := &Application{
		handler:  NewHandler(NewRequestProcessor(&NoOpDB{}, false, nil)),
		template: renderer,
	}
	e := app.setupServer()

	req := httptest.NewRequest(http.MethodGet, "/privacy?x=1", nil)
	req.Host = "www.ipthing.net"
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "https://ipthing.net/privacy?x=1", rec.Header().Get("Location"))

	apex := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	apex.Host = "ipthing.net"
	apex.TLS = &tls.ConnectionState{}
	apexRec := httptest.NewRecorder()
	e.ServeHTTP(apexRec, apex)
	assert.Equal(t, http.StatusOK, apexRec.Code)

	// Plain HTTP stays available: www keeps the scheme, and browsers are not upgraded.
	const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"
	httpWWW := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	httpWWW.Host = "www.ipthing.net"
	httpWWW.Header.Set("User-Agent", browserUA)
	httpWWWRec := httptest.NewRecorder()
	e.ServeHTTP(httpWWWRec, httpWWW)
	assert.Equal(t, http.StatusMovedPermanently, httpWWWRec.Code)
	assert.Equal(t, "http://ipthing.net/privacy", httpWWWRec.Header().Get("Location"))

	httpApex := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	httpApex.Host = "ipthing.net"
	httpApex.Header.Set("User-Agent", browserUA)
	httpApexRec := httptest.NewRecorder()
	e.ServeHTTP(httpApexRec, httpApex)
	assert.Equal(t, http.StatusOK, httpApexRec.Code)
	assert.Empty(t, httpApexRec.Header().Get("Strict-Transport-Security"))
}

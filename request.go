package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/labstack/echo/v4"
)

// RequestProcessor handles request processing and IP extraction
type RequestProcessor struct {
	db        Database
	dbDefined bool
	logger    echo.Logger
}

// NewRequestProcessor creates a new request processor
func NewRequestProcessor(db Database, dbDefined bool, logger echo.Logger) *RequestProcessor {
	return &RequestProcessor{
		db:        db,
		dbDefined: dbDefined,
		logger:    logger,
	}
}

// ProcessRequest handles the complete request processing pipeline
func (rp *RequestProcessor) ProcessRequest(c echo.Context) (*HTTPRequest, *IPInfo, error) {
	req := c.Request()

	// Extract client IP from the TCP peer (no proxy headers)
	clientIP := rp.extractClientIP(c)

	// Fetch IP geolocation info
	var ipInfo *IPInfo
	if clientIP != "" {
		var err error
		ipInfo, err = getOrFetchIPInfo(req.Context(), rp.db, clientIP)
		if err != nil {
			rp.logger.Errorf("Failed to get IP info: %v", err)
		} else {
			rp.logger.Infof("IP info for %s: %+v", clientIP, ipInfo)
		}
	}

	// Create HTTP request record (storage deferred until response format/latency known)
	httpReq := rp.buildHTTPRequest(req, clientIP)

	return httpReq, ipInfo, nil
}

// extractClientIP returns the TCP peer IP via Echo's IPExtractor.
// Spoofable forwarding headers are displayed in the response but never trusted here.
func (rp *RequestProcessor) extractClientIP(c echo.Context) string {
	return c.RealIP()
}

// buildHTTPRequest creates an HTTPRequest from the incoming request and client IP
func (rp *RequestProcessor) buildHTTPRequest(req *http.Request, clientIP string) *HTTPRequest {
	headersJSON, _ := json.Marshal(headersForStorage(req.Header))

	queryParams := make(map[string][]string)
	for k, v := range req.URL.Query() {
		queryParams[k] = v
	}
	queryParamsJSON, _ := json.Marshal(queryParams)

	httpReq := &HTTPRequest{
		IP:             clientIP,
		Method:         req.Method,
		Path:           req.URL.Path,
		UserAgent:      req.UserAgent(),
		Referer:        req.Header.Get("Referer"),
		Headers:        string(headersJSON),
		QueryParams:    string(queryParamsJSON),
		Timestamp:      time.Now(),
		Proto:          req.Proto,
		ContentLength:  req.ContentLength,
		RemoteAddr:     req.RemoteAddr,
		RequestURI:     req.RequestURI,
		Host:           req.Host,
		ContentType:    req.Header.Get("Content-Type"),
		HasCookies:     req.Header.Get("Cookie") != "",
		ClaimedXFF:     req.Header.Get("X-Forwarded-For"),
		CfConnectingIP: req.Header.Get("Cf-Connecting-Ip"),
	}

	// Capture TLS information if available
	if req.TLS != nil {
		httpReq.TLSVersion = req.TLS.Version
		httpReq.TLSCipherSuite = req.TLS.CipherSuite
		httpReq.TLSServerName = req.TLS.ServerName
		httpReq.TLSNegotiatedProtocol = req.TLS.NegotiatedProtocol
		httpReq.Scheme = "https"
	} else {
		httpReq.Scheme = "http"
	}

	return httpReq
}

// sensitiveHeaderNames are never stored with their real values.
var sensitiveHeaderNames = map[string]struct{}{
	"Authorization":       {},
	"Proxy-Authorization": {},
	"Cookie":              {},
	"Set-Cookie":          {},
	"X-Api-Key":           {},
	"X-Auth-Token":        {},
}

// headersForStorage copies headers for DB persistence, redacting secrets.
func headersForStorage(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		if _, sensitive := sensitiveHeaderNames[http.CanonicalHeaderKey(k)]; sensitive {
			out[k] = []string{"[redacted]"}
			continue
		}
		out[k] = v
	}
	return out
}

// QueueStore persists the request asynchronously when a database is configured.
func (rp *RequestProcessor) QueueStore(httpReq *HTTPRequest) {
	if !rp.dbDefined || httpReq == nil || httpReq.IP == "" {
		return
	}
	go rp.storeRequestAsync(httpReq)
}

// storeRequestAsync stores the request in the database asynchronously after duplicate checking
func (rp *RequestProcessor) storeRequestAsync(httpReq *HTTPRequest) {
	// Create fingerprint for duplicate checking
	fingerprint := &RequestFingerprint{
		IP:          httpReq.IP,
		Path:        httpReq.Path,
		QueryParams: httpReq.QueryParams,
		UserAgent:   httpReq.UserAgent,
	}

	// Check for duplicates within the last 5 minutes
	isDuplicate, err := rp.db.IsDuplicateRequest(fingerprint, 5*time.Minute)
	if err != nil {
		rp.logger.Errorf("Failed to check for duplicate request: %v", err)
		return
	}

	if isDuplicate {
		rp.logger.Debugf("Skipping duplicate request from %s to %s", httpReq.IP, httpReq.Path)
		return
	}

	// Ensure IP exists in ip_info table before saving request
	// This prevents foreign key constraint violations
	if _, err := rp.db.GetIPInfo(httpReq.IP); err != nil {
		// IP doesn't exist, create a minimal entry
		minimalIPInfo := &IPInfo{
			IP:         httpReq.IP,
			LastUpdate: time.Now(),
		}
		if err := rp.db.SaveIPInfo(minimalIPInfo); err != nil {
			rp.logger.Errorf("Failed to save minimal IP info: %v", err)
			return
		}
	}

	// Save HTTP request
	if err := rp.db.SaveHTTPRequest(httpReq); err != nil {
		rp.logger.Errorf("Failed to save HTTP request: %v", err)
	}
}

// IsWebBrowser determines if the request comes from a web browser based on User-Agent
func IsWebBrowser(userAgent string) bool {
	r := regexp.MustCompile(`.*(Mozilla|AppleWebKit|Trident|Presto|Gecko|KHTML|Blink|Lynx|Links|w3m|elinks).*`)
	return r.MatchString(userAgent)
}

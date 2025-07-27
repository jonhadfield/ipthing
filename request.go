package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
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
func (rp *RequestProcessor) ProcessRequest(c echo.Context, behindProxy bool) (*HTTPRequest, *IPInfo, error) {
	req := c.Request()

	var clientIP string

	var err error

	remoteAddr := c.Request().RemoteAddr
	clientIP, _, err = net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse remote address %s: %w", remoteAddr, err)
	}

	if behindProxy {
		// Extract client IP using multiple methods
		clientIP, err = rp.extractClientIP(c)
		if err != nil {
			return nil, nil, err
		}
	}

	// Fetch IP geolocation info
	var ipInfo *IPInfo
	if clientIP != "" {
		ipInfo, err = getOrFetchIPInfo(rp.db, clientIP)
		if err != nil {
			rp.logger.Errorf("Failed to get IP info: %v", err)
		} else {
			rp.logger.Infof("IP info for %s: %+v", clientIP, ipInfo)
		}
	}

	// Create HTTP request record
	httpReq := rp.buildHTTPRequest(req, clientIP)

	// Store request in database asynchronously if database is configured
	if rp.dbDefined && clientIP != "" {
		go rp.storeRequestAsync(httpReq)
	}

	return httpReq, ipInfo, nil
}

// extractClientIP extracts the client IP using multiple methods
func (rp *RequestProcessor) extractClientIP(c echo.Context) (string, error) {
	req := c.Request()

	// Try Cloudflare connecting IP first
	clientIP := req.Header.Get(HeaderCloudflareConnectingIP)
	if clientIP != "" {
		return clientIP, nil
	}

	// Try X-Forwarded-For parsing
	firstUntrusted, err := parseXForwardedFor(req)
	if err != nil {
		return "", err
	}
	if firstUntrusted != "" {
		log.Print("first untrusted IP from X-Forwarded-For: ", firstUntrusted)
		rp.logger.Info(firstUntrusted)
		return firstUntrusted, nil
	}

	return c.RealIP(), nil
}

// buildHTTPRequest creates an HTTPRequest from the incoming request and client IP
func (rp *RequestProcessor) buildHTTPRequest(req *http.Request, clientIP string) *HTTPRequest {
	// Prepare headers as JSON
	headers := make(map[string][]string)
	for k, v := range req.Header {
		headers[k] = v
	}
	headersJSON, _ := json.Marshal(headers)

	// Prepare query params as JSON
	queryParams := make(map[string][]string)
	for k, v := range req.URL.Query() {
		queryParams[k] = v
	}
	queryParamsJSON, _ := json.Marshal(queryParams)

	// Capture request body (first 1KB for debugging)
	var bodyContent string
	if req.Body != nil && req.ContentLength > 0 {
		bodyBytes := make([]byte, RequestBodyReadLimit) // Read up to 1KB
		n, _ := req.Body.Read(bodyBytes)
		if n > 0 {
			bodyContent = string(bodyBytes[:n])
		}
		// Note: Body would need to be restored for the actual handler
		// This is just for logging/debugging purposes
	}

	httpReq := &HTTPRequest{
		IP:            clientIP,
		Method:        req.Method,
		Path:          req.URL.Path,
		UserAgent:     req.UserAgent(),
		Referer:       req.Header.Get("Referer"),
		Headers:       string(headersJSON),
		QueryParams:   string(queryParamsJSON),
		Timestamp:     time.Now(),
		Proto:         req.Proto,
		ContentLength: req.ContentLength,
		RemoteAddr:    req.RemoteAddr,
		RequestURI:    req.RequestURI,
		Host:          req.Host,
		ContentType:   req.Header.Get("Content-Type"),
		Body:          bodyContent,
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
	isDuplicate, err := rp.db.IsDuplicateRequest(fingerprint, DuplicateRequestWindow)
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

// parseXForwardedFor extracts the first untrusted IP from X-Forwarded-For header
func parseXForwardedFor(req *http.Request) (string, error) {
	xff := req.Header.Get(HeaderXForwardedFor)
	if xff == "" {
		return "", nil
	}

	xff = strings.ReplaceAll(xff, " ", "")
	fmt.Println("GOT X-Forwarded-For:", xff)
	// Strip private IPs and extract the first untrusted IP
	strippedXFF := stripXFFTrustedProxies(xff, 1)
	return stripPrivateIPs(strippedXFF), nil
}

// stripPrivateIPs removes IPv6 addresses and private IPs from XFF string
func stripPrivateIPs(xff string) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.Contains(parts[i], ":") {
			return strings.Join(parts[:i], ",")
		}
	}

	return xff
}

// stripXFFTrustedProxies removes trusted proxies from the end of XFF chain
func stripXFFTrustedProxies(xff string, trustedPos int) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	if len(parts) <= trustedPos {
		return ""
	}

	realTrustedPos := len(parts) - trustedPos
	return strings.Join(parts[:realTrustedPos], ",")
}

// IsWebBrowser determines if the request comes from a web browser based on User-Agent
func IsWebBrowser(userAgent string) bool {
	r := regexp.MustCompile(`.*(Mozilla|AppleWebKit|Trident|Presto|Gecko|KHTML|Blink|Lynx|Links|w3m|elinks).*`)
	return r.MatchString(userAgent)
}

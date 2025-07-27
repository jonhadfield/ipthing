package main

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler contains the application handlers
type Handler struct {
	requestProcessor *RequestProcessor
}

// NewHandler creates a new handler instance
func NewHandler(requestProcessor *RequestProcessor) *Handler {
	return &Handler{
		requestProcessor: requestProcessor,
	}
}

// HandleRoot handles the main route that displays client information
func (h *Handler) HandleRoot(c echo.Context, behindProxy bool) error {
	// Process the request and extract information
	httpReq, ipInfo, err := h.requestProcessor.ProcessRequest(c, false)
	if err != nil {
		return err
	}

	// Determine response format based on User-Agent
	if IsWebBrowser(c.Request().UserAgent()) {
		// Build flat response data for web view
		responseData := h.buildResponseData(c.Request(), httpReq, ipInfo)
		return c.Render(http.StatusOK, "web", responseData)
	} else {
		// Build structured response for CLI
		structuredResponse := h.buildStructuredResponse(c.Request(), httpReq, ipInfo)
		return c.JSONPretty(http.StatusOK, structuredResponse, "  ")
	}
}

// buildResponseData creates the response data structure from request information
func (h *Handler) buildResponseData(req *http.Request, httpReq *HTTPRequest, ipInfo *IPInfo) map[string]interface{} {
	// Start with basic request data - using capitalized keys to match template expectations
	data := map[string]interface{}{
		"IPAddr":         httpReq.IP,
		"UserAgent":      req.UserAgent(),
		"Host":           req.Host,
		"Accept":         req.Header.Get("Accept"),
		"AcceptEncoding": req.Header.Get("Accept-Encoding"),
		"AcceptLanguage": req.Header.Get("Accept-Language"),
		"DNT":            req.Header.Get("DNT"),
		"Language":       req.Header.Get("Language"),
		"Referer":        req.Header.Get("Referer"),
		"Method":         req.Method,
		"MIMEType":       req.Header.Get("Content-Type"),
		"Charset":        req.Header.Get("Charset"),
		"XFF":            req.Header.Get(HeaderXForwardedFor),
		"XRI":            req.Header.Get(HeaderXRealIP),
		// New fields
		"Proto":         httpReq.Proto,
		"ContentLength": httpReq.ContentLength,
		"RemoteAddr":    httpReq.RemoteAddr,
		"RequestURI":    httpReq.RequestURI,
		"Scheme":        httpReq.Scheme,
		// Initialize empty values for fields that templates expect
		"Country": "",
		"City":    "",
		"Org":     "",
		"Visitor": "",
		// CSS constants
		"WebTableWidth": WebTableWidth,
	}

	// Add TLS information if available
	if httpReq.TLSVersion > 0 {
		data["TLSVersion"] = getTLSVersionString(httpReq.TLSVersion)
		data["TLSCipherSuite"] = getTLSCipherSuiteString(httpReq.TLSCipherSuite)
		data["TLSServerName"] = httpReq.TLSServerName
		data["TLSNegotiatedProtocol"] = httpReq.TLSNegotiatedProtocol
	}

	// Add Cloudflare specific headers
	if cfCountry := req.Header.Get(HeaderCloudflareiPCountry); cfCountry != "" {
		data["Country"] = cfCountry
	}
	if cfVisitor := req.Header.Get(HeaderCloudflareVisitor); cfVisitor != "" {
		data["Visitor"] = cfVisitor
	}

	// Override with IP info from geolocation service if available
	if ipInfo != nil {
		if ipInfo.Country != "" {
			data["Country"] = ipInfo.Country
		}
		if ipInfo.City != "" {
			data["City"] = ipInfo.City
		}
		if ipInfo.Org != "" {
			data["Org"] = ipInfo.Org
		}
	}

	return data
}

// buildStructuredResponse creates a structured response for CLI clients
func (h *Handler) buildStructuredResponse(req *http.Request, httpReq *HTTPRequest, ipInfo *IPInfo) *StructuredResponse {
	response := &StructuredResponse{
		IP: &IPSection{
			Address: httpReq.IP,
		},
		Request: &RequestSection{
			Method:        httpReq.Method,
			Path:          httpReq.Path,
			Proto:         httpReq.Proto,
			Host:          httpReq.Host,
			Scheme:        httpReq.Scheme,
			ContentLength: httpReq.ContentLength,
			UserAgent:     httpReq.UserAgent,
		},
		Headers: &HeadersSection{
			Accept:         req.Header.Get("Accept"),
			AcceptEncoding: req.Header.Get("Accept-Encoding"),
			AcceptLanguage: req.Header.Get("Accept-Language"),
			Referer:        req.Header.Get("Referer"),
			XForwardedFor:  req.Header.Get(HeaderXForwardedFor),
			XRealIP:        req.Header.Get(HeaderXRealIP),
			DNT:            req.Header.Get("DNT"),
			ContentType:    req.Header.Get("Content-Type"),
		},
	}

	// Add IP location info if available
	if ipInfo != nil {
		response.IP.City = ipInfo.City
		response.IP.Country = ipInfo.Country
		response.IP.Org = ipInfo.Org
		response.IP.Hostname = ipInfo.Hostname
	}

	// Add Cloudflare country if available
	if cfCountry := req.Header.Get(HeaderCloudflareiPCountry); cfCountry != "" && response.IP.Country == "" {
		response.IP.Country = cfCountry
	}

	// Add TLS information if available
	if httpReq.TLSVersion > 0 {
		response.TLS = &TLSInfo{
			Version:            getTLSVersionString(httpReq.TLSVersion),
			CipherSuite:        getTLSCipherSuiteString(httpReq.TLSCipherSuite),
			ServerName:         httpReq.TLSServerName,
			NegotiatedProtocol: httpReq.TLSNegotiatedProtocol,
		}
	}

	// Collect other headers (excluding those already captured)
	knownHeaders := map[string]bool{
		"accept": true, "accept-encoding": true, "accept-language": true,
		"referer": true, "x-forwarded-for": true, "x-real-ip": true,
		"dnt": true, "content-type": true, "user-agent": true,
		"host": true, "content-length": true,
		strings.ToLower(HeaderCloudflareConnectingIP): true,
		strings.ToLower(HeaderCloudflareiPCountry):    true,
		strings.ToLower(HeaderCloudflareVisitor):      true,
	}

	otherHeaders := make(map[string]interface{})
	for key, values := range req.Header {
		lowerKey := strings.ToLower(key)
		if !knownHeaders[lowerKey] && len(values) > 0 {
			if len(values) == 1 {
				otherHeaders[key] = values[0]
			} else {
				otherHeaders[key] = values
			}
		}
	}

	if len(otherHeaders) > 0 {
		response.Headers.Other = otherHeaders
	}

	return response
}

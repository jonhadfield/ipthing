package main

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"net/http"
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
func (h *Handler) HandleRoot(c echo.Context) error {
	// Process the request and extract information
	httpReq, ipInfo, err := h.requestProcessor.ProcessRequest(c)
	if err != nil {
		return err
	}

	// Build response data
	responseData := h.buildResponseData(c.Request(), httpReq, ipInfo)

	// Determine response format based on User-Agent
	if IsWebBrowser(c.Request().UserAgent()) {
		return c.Render(http.StatusOK, "web", responseData)
	} else {
		consoleData, _ := json.MarshalIndent(responseData, "", "  ")
		return c.Render(http.StatusOK, "console", string(consoleData))
	}
}

// buildResponseData creates the response data structure from request information
func (h *Handler) buildResponseData(req *http.Request, httpReq *HTTPRequest, ipInfo *IPInfo) map[string]interface{} {
	// Start with basic request data
	data := map[string]interface{}{
		"ip_addr":         httpReq.IP,
		"user_agent":      req.UserAgent(),
		"host":            req.Host,
		"accept":          req.Header.Get("Accept"),
		"accept_encoding": req.Header.Get("Accept-Encoding"),
		"accept_language": req.Header.Get("Accept-Language"),
		"dnt":             req.Header.Get("DNT"),
		"language":        req.Header.Get("Language"),
		"referer":         req.Header.Get("Referer"),
		"method":          req.Method,
		"mime_type":       req.Header.Get("Content-Type"),
		"charset":         req.Header.Get("Charset"),
		"xff":             req.Header.Get("X-Forwarded-For"),
		"xri":             req.Header.Get("X-Real-IP"),
	}

	// Add Cloudflare specific headers
	if cfCountry := req.Header.Get("Cf-IPcountry"); cfCountry != "" {
		data["country"] = cfCountry
	}
	if cfVisitor := req.Header.Get("Cf-Visitor"); cfVisitor != "" {
		data["visitor"] = cfVisitor
	}

	// Override with IP info from geolocation service if available
	if ipInfo != nil {
		if ipInfo.Country != "" {
			data["country"] = ipInfo.Country
		}
		if ipInfo.City != "" {
			data["city"] = ipInfo.City
		}
		if ipInfo.Org != "" {
			data["org"] = ipInfo.Org
		}
	}

	return data
}

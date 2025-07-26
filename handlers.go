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
		"XFF":            req.Header.Get("X-Forwarded-For"),
		"XRI":            req.Header.Get("X-Real-IP"),
		// Initialize empty values for fields that templates expect
		"Country":        "",
		"City":           "",
		"Org":            "",
		"Visitor":        "",
	}

	// Add Cloudflare specific headers
	if cfCountry := req.Header.Get("Cf-IPcountry"); cfCountry != "" {
		data["Country"] = cfCountry
	}
	if cfVisitor := req.Header.Get("Cf-Visitor"); cfVisitor != "" {
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

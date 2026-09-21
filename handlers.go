package main

import (
	"encoding/json"
	"net/http"
	"time"

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
func (h *Handler) HandleRoot(c echo.Context) error {
	start := time.Now()

	httpReq, ipInfo, err := h.requestProcessor.ProcessRequest(c)
	if err != nil {
		return err
	}

	responseData := h.buildResponseData(c.Request(), httpReq, ipInfo)

	var writeErr error
	if IsWebBrowser(c.Request().UserAgent()) {
		httpReq.ResponseFormat = "html"
		writeErr = c.Render(http.StatusOK, "web", responseData)
	} else {
		httpReq.ResponseFormat = "json"
		consoleData, _ := json.MarshalIndent(responseData, "", "  ")
		writeErr = c.JSONBlob(http.StatusOK, consoleData)
	}

	httpReq.DurationMs = time.Since(start).Milliseconds()
	h.requestProcessor.QueueStore(httpReq)

	return writeErr
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
	}

	// Add TLS information if available
	if httpReq.TLSVersion > 0 {
		data["TLSVersion"] = getTLSVersionString(httpReq.TLSVersion)
		data["TLSCipherSuite"] = getTLSCipherSuiteString(httpReq.TLSCipherSuite)
		data["TLSServerName"] = httpReq.TLSServerName
		data["TLSNegotiatedProtocol"] = httpReq.TLSNegotiatedProtocol
	}

	// Add Cloudflare specific headers
	if cfCountry := req.Header.Get("Cf-IPcountry"); cfCountry != "" {
		data["Country"] = cfCountry
	}
	if cfVisitor := req.Header.Get("Cf-Visitor"); cfVisitor != "" {
		data["Visitor"] = cfVisitor
	}
	if cfRay := req.Header.Get("CF-Ray"); cfRay != "" {
		data["CFRay"] = cfRay
	}
	if cfRequestID := req.Header.Get("CF-Request-ID"); cfRequestID != "" {
		data["CFRequestID"] = cfRequestID
	}

	// Add security headers (Sec-Fetch-*)
	if secFetchSite := req.Header.Get("Sec-Fetch-Site"); secFetchSite != "" {
		data["SecFetchSite"] = secFetchSite
	}
	if secFetchMode := req.Header.Get("Sec-Fetch-Mode"); secFetchMode != "" {
		data["SecFetchMode"] = secFetchMode
	}
	if secFetchUser := req.Header.Get("Sec-Fetch-User"); secFetchUser != "" {
		data["SecFetchUser"] = secFetchUser
	}
	if secFetchDest := req.Header.Get("Sec-Fetch-Dest"); secFetchDest != "" {
		data["SecFetchDest"] = secFetchDest
	}

	// Add client hints
	if secCHUA := req.Header.Get("Sec-CH-UA"); secCHUA != "" {
		data["SecCHUA"] = secCHUA
	}
	if secCHUAMobile := req.Header.Get("Sec-CH-UA-Mobile"); secCHUAMobile != "" {
		data["SecCHUAMobile"] = secCHUAMobile
	}
	if secCHUAPlatform := req.Header.Get("Sec-CH-UA-Platform"); secCHUAPlatform != "" {
		data["SecCHUAPlatform"] = secCHUAPlatform
	}
	if secCHUAArch := req.Header.Get("Sec-CH-UA-Arch"); secCHUAArch != "" {
		data["SecCHUAArch"] = secCHUAArch
	}
	if deviceMemory := req.Header.Get("Device-Memory"); deviceMemory != "" {
		data["DeviceMemory"] = deviceMemory
	}
	if viewportWidth := req.Header.Get("Viewport-Width"); viewportWidth != "" {
		data["ViewportWidth"] = viewportWidth
	}

	// Add additional security/privacy headers
	if upgradeInsecure := req.Header.Get("Upgrade-Insecure-Requests"); upgradeInsecure != "" {
		data["UpgradeInsecureRequests"] = upgradeInsecure
	}
	if saveData := req.Header.Get("Save-Data"); saveData != "" {
		data["SaveData"] = saveData
	}

	// Add proxy/forwarding headers
	if via := req.Header.Get("Via"); via != "" {
		data["Via"] = via
	}
	if forwarded := req.Header.Get("Forwarded"); forwarded != "" {
		data["Forwarded"] = forwarded
	}
	if trueClientIP := req.Header.Get("True-Client-IP"); trueClientIP != "" {
		data["TrueClientIP"] = trueClientIP
	}

	// Add connection headers
	if connection := req.Header.Get("Connection"); connection != "" {
		data["Connection"] = connection
	}
	if cacheControl := req.Header.Get("Cache-Control"); cacheControl != "" {
		data["CacheControl"] = cacheControl
	}

	// Add request metadata
	data["Timestamp"] = httpReq.Timestamp.Format("2006-01-02 15:04:05 MST")

	// Add cookie presence (boolean for privacy)
	if cookie := req.Header.Get("Cookie"); cookie != "" {
		data["HasCookies"] = true
	} else {
		data["HasCookies"] = false
	}

	// Parse and add query parameters
	if httpReq.QueryParams != "" && httpReq.QueryParams != "{}" {
		var queryParams map[string][]string
		if err := json.Unmarshal([]byte(httpReq.QueryParams), &queryParams); err == nil {
			data["QueryParams"] = queryParams
		}
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
		// Add additional geolocation fields
		if ipInfo.Region != "" {
			data["Region"] = ipInfo.Region
		}
		if ipInfo.Postal != "" {
			data["Postal"] = ipInfo.Postal
		}
		if ipInfo.Timezone != "" {
			data["Timezone"] = ipInfo.Timezone
		}
		if ipInfo.Loc != "" {
			data["Loc"] = ipInfo.Loc
		}
		if ipInfo.Hostname != "" {
			data["Hostname"] = ipInfo.Hostname
		}
		data["Bogon"] = ipInfo.Bogon
	}

	return data
}

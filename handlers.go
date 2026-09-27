package main

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
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

	httpReq.StatusCode = http.StatusOK
	httpReq.DurationMs = time.Since(start).Milliseconds()
	h.requestProcessor.QueueStore(httpReq)

	return writeErr
}

// headerEntry is one request header for JSON/HTML (values may be redacted).
type headerEntry struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// buildResponseData creates the response data structure from request information
func (h *Handler) buildResponseData(req *http.Request, httpReq *HTTPRequest, ipInfo *IPInfo) map[string]any {
	data := map[string]any{
		"IPAddr":         httpReq.IP,
		"IPFamily":       httpReq.IPFamily,
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
		"Proto":          httpReq.Proto,
		"ContentLength":  httpReq.ContentLength,
		"RemoteAddr":     httpReq.RemoteAddr,
		"RequestURI":     httpReq.RequestURI,
		"Scheme":         httpReq.Scheme,
		"Country":        "",
		"City":           "",
		"Org":            "",
		"Visitor":        "",
	}

	if httpReq.PTRHostname != "" {
		data["PTRHostname"] = httpReq.PTRHostname
	}

	if httpReq.TLSVersion > 0 {
		data["TLSVersion"] = getTLSVersionString(httpReq.TLSVersion)
		data["TLSCipherSuite"] = getTLSCipherSuiteString(httpReq.TLSCipherSuite)
		data["TLSServerName"] = httpReq.TLSServerName
		data["TLSNegotiatedProtocol"] = httpReq.TLSNegotiatedProtocol
		data["TLSDidResume"] = httpReq.TLSDidResume
		if httpReq.TLSCurve != "" {
			data["TLSCurve"] = httpReq.TLSCurve
		}
		if httpReq.TLSClientSubject != "" {
			data["TLSClientSubject"] = httpReq.TLSClientSubject
		}
	}
	if httpReq.JA3 != "" {
		data["JA3"] = httpReq.JA3
	}
	if httpReq.JA4 != "" {
		data["JA4"] = httpReq.JA4
	}

	// Claimed forwarding / CDN headers (never trusted for client IP)
	if cfConnecting := req.Header.Get("Cf-Connecting-Ip"); cfConnecting != "" {
		data["CfConnectingIP"] = cfConnecting
	}
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

	setIfHeader := func(key, header string) {
		if v := req.Header.Get(header); v != "" {
			data[key] = v
		}
	}
	setIfHeader("SecFetchSite", "Sec-Fetch-Site")
	setIfHeader("SecFetchMode", "Sec-Fetch-Mode")
	setIfHeader("SecFetchUser", "Sec-Fetch-User")
	setIfHeader("SecFetchDest", "Sec-Fetch-Dest")
	setIfHeader("SecCHUA", "Sec-CH-UA")
	setIfHeader("SecCHUAMobile", "Sec-CH-UA-Mobile")
	setIfHeader("SecCHUAPlatform", "Sec-CH-UA-Platform")
	setIfHeader("SecCHUAArch", "Sec-CH-UA-Arch")
	setIfHeader("SecCHUAFullVersionList", "Sec-CH-UA-Full-Version-List")
	setIfHeader("SecCHUAModel", "Sec-CH-UA-Model")
	setIfHeader("SecCHUABitness", "Sec-CH-UA-Bitness")
	setIfHeader("SecCHPrefersColorScheme", "Sec-CH-Prefers-Color-Scheme")
	setIfHeader("SecCHViewportHeight", "Sec-CH-Viewport-Height")
	setIfHeader("DeviceMemory", "Device-Memory")
	setIfHeader("ViewportWidth", "Viewport-Width")
	setIfHeader("DPR", "DPR")
	setIfHeader("Width", "Width")
	setIfHeader("UpgradeInsecureRequests", "Upgrade-Insecure-Requests")
	setIfHeader("SaveData", "Save-Data")
	setIfHeader("Via", "Via")
	setIfHeader("Forwarded", "Forwarded")
	setIfHeader("TrueClientIP", "True-Client-IP")
	setIfHeader("Connection", "Connection")
	setIfHeader("CacheControl", "Cache-Control")
	setIfHeader("Priority", "Priority")
	setIfHeader("TE", "TE")

	data["Timestamp"] = httpReq.Timestamp.Format("2006-01-02 15:04:05 MST")
	data["HasCookies"] = httpReq.HasCookies
	if httpReq.CookieNames != "" {
		data["CookieNames"] = strings.Split(httpReq.CookieNames, ",")
	}

	if httpReq.QueryParams != "" && httpReq.QueryParams != "{}" {
		var queryParams map[string][]string
		if err := json.Unmarshal([]byte(httpReq.QueryParams), &queryParams); err == nil {
			data["QueryParams"] = queryParams
		}
	}

	// Full header list (secrets already redacted in stored JSON / headersForStorage)
	stored := headersForStorage(req.Header)
	keys := make([]string, 0, len(stored))
	for k := range stored {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	headers := make([]headerEntry, 0, len(keys))
	for _, k := range keys {
		headers = append(headers, headerEntry{Name: k, Values: stored[k]})
	}
	data["Headers"] = headers

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

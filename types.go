package main

import "time"

type IPInfo struct {
	IP         string `json:"ip"`
	City       string `json:"city"`
	Region     string `json:"region"`
	Country    string `json:"country"`
	Loc        string `json:"loc"`
	Org        string `json:"org"`
	Postal     string `json:"postal"`
	Timezone   string `json:"timezone"`
	Hostname   string `json:"hostname"`
	Bogon      bool   `json:"bogon"`
	LastUpdate time.Time
}

type HTTPRequest struct {
	ID          int64
	IP          string
	Method      string
	Path        string
	UserAgent   string
	Referer     string
	Headers     string
	QueryParams string
	Timestamp   time.Time
	// TLS Information
	TLSVersion            uint16
	TLSCipherSuite        uint16
	TLSServerName         string
	TLSNegotiatedProtocol string
	// Additional request details
	Proto         string // HTTP/1.1, HTTP/2.0, etc.
	ContentLength int64
	RemoteAddr    string
	RequestURI    string
	Host          string
	Scheme        string // http or https
	// Request body info
	ContentType string
	Body        string // Intentionally unused for persistence (privacy)
	// Inspection / analytics
	HasCookies     bool
	ClaimedXFF     string // raw X-Forwarded-For (spoofable; not trusted for IP)
	CfConnectingIP string // Cf-Connecting-Ip (spoofable; not trusted for IP)
	DurationMs     int64
	ResponseFormat string // "html", "json", or "error"
	StatusCode     int    // HTTP response status written for this request
}

type RequestFingerprint struct {
	IP          string
	Path        string
	QueryParams string
	UserAgent   string
}

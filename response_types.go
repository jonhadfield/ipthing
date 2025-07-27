package main

// StructuredResponse represents the sectioned JSON response for CLI clients
type StructuredResponse struct {
	IP      *IPSection      `json:"ip"`
	Request *RequestSection `json:"request"`
	Headers *HeadersSection `json:"headers"`
	TLS     *TLSInfo        `json:"tls,omitempty"`
}

// IPSection contains IP and location information
type IPSection struct {
	Address  string `json:"address"`
	City     string `json:"city,omitempty"`
	Country  string `json:"country,omitempty"`
	Org      string `json:"org,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

// RequestSection contains core request information
type RequestSection struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	Proto         string `json:"proto"`
	Host          string `json:"host"`
	Scheme        string `json:"scheme"`
	ContentLength int64  `json:"content_length"`
	UserAgent     string `json:"user_agent"`
}

// HeadersSection contains additional headers
type HeadersSection struct {
	Accept         string                 `json:"accept,omitempty"`
	AcceptEncoding string                 `json:"accept_encoding,omitempty"`
	AcceptLanguage string                 `json:"accept_language,omitempty"`
	Referer        string                 `json:"referer,omitempty"`
	XForwardedFor  string                 `json:"x_forwarded_for,omitempty"`
	XRealIP        string                 `json:"x_real_ip,omitempty"`
	DNT            string                 `json:"dnt,omitempty"`
	ContentType    string                 `json:"content_type,omitempty"`
	Other          map[string]interface{} `json:"other,omitempty"`
}

// TLSInfo contains TLS connection details
type TLSInfo struct {
	Version            string `json:"version"`
	CipherSuite        string `json:"cipher_suite"`
	ServerName         string `json:"server_name,omitempty"`
	NegotiatedProtocol string `json:"negotiated_protocol,omitempty"`
}

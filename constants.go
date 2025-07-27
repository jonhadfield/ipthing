package main

import "time"

// Default network ports
const (
	DefaultHTTPPort  = 8080
	DefaultHTTPSPort = 443
	DefaultPortHTTPS = 8443 // Alternative HTTPS port for testing
)

// Server timeouts and limits
const (
	GracefulShutdownTimeout = 10 * time.Second
	DuplicateRequestWindow  = 5 * time.Minute
	RequestBodyReadLimit    = 1024 // bytes
)

// File paths and directories
const (
	AutoTLSCacheDir = "/opt/ipthing/.cache"
)

// MIME types
const (
	MimeTypeIcon        = "image/x-icon"
	MimeTypePNG         = "image/png"
	MimeTypeManifest    = "application/manifest+json"
	MimeTypeOctetStream = "application/octet-stream"
)

// HTTP headers
const (
	HeaderCloudflareConnectingIP = "Cf-Connecting-Ip"
	HeaderXForwardedFor          = "X-Forwarded-For"
	HeaderXRealIP                = "X-Real-IP"
	HeaderCloudflareiPCountry    = "Cf-IPcountry"
	HeaderCloudflareVisitor      = "Cf-Visitor"
)

// Database table and column names
const (
	TableIPInfo       = "ip_info"
	TableHTTPRequests = "http_requests"

	// Common column names
	ColumnIP          = "ip"
	ColumnMethod      = "method"
	ColumnPath        = "path"
	ColumnUserAgent   = "user_agent"
	ColumnReferer     = "referer"
	ColumnHeaders     = "headers"
	ColumnQueryParams = "query_params"
	ColumnTimestamp   = "timestamp"
)

// File extensions
const (
	ExtensionICO         = ".ico"
	ExtensionPNG         = ".png"
	ExtensionWebmanifest = ".webmanifest"
)

// CSS and layout values
const (
	WebTableWidth = 1024 // pixels
)

// Configuration environment variables
const (
	EnvIPThingConfig      = "IPTHING_CONFIG"
	EnvIPThingDBType      = "IPTHING_DB_TYPE"
	EnvIPThingSQLitePath  = "IPTHING_SQLITE_PATH"
	EnvIPThingPostgresURL = "IPTHING_POSTGRES_URL"
	EnvIPThingHTTPPort    = "IPTHING_HTTP_PORT"
	EnvIPThingHTTPSPort   = "IPTHING_HTTPS_PORT"
	EnvDatabaseURL        = "DATABASE_URL"
)

// Database types
const (
	DatabaseTypeSQLite   = "sqlite"
	DatabaseTypePostgres = "postgres"
	DatabaseTypeMariaDB  = "mariadb"
	DatabaseTypeMySQL    = "mysql"
)

// Signal and process management
const (
	MaxSignalBufferSize = 1
)

// Default file sizes and limits
const (
	DefaultReadLimit = 2000 // lines for file reading
	MaxLineLength    = 2000 // characters per line
)

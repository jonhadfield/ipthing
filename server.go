package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	// Initialize application
	app, err := initializeApplication()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.cleanup()

	// Log successful startup
	log.Printf("ipthing application initialized successfully")
	log.Printf("Version: %s", versionString())
	if version != "" && version != versionString() {
		log.Printf("Build details: %s", version)
	}

	// Setup and start servers
	app.startServers()
}

// Application holds all application dependencies
type Application struct {
	config       *Config
	db           Database
	dbDefined    bool
	template     *EmbeddedRenderer
	handler      *Handler
	syslogWriter syslogSink
}

// initializeApplication sets up all application dependencies
func initializeApplication() (*Application, error) {
	// Setup syslog
	syslogWriter, err := setupSyslog()
	if err != nil {
		log.Printf("Warning: Syslog not available, using stderr: %v", err)
	} else {
		log.Printf("Syslog configured successfully")
	}

	// Configure logging
	setupLogger(syslogWriter)
	log.Printf("Starting ipthing initialization")

	// Load configuration (env from systemd still applies when config.json is absent)
	config, err := ReadConfig("config.json")
	if err != nil {
		log.Printf("Warning: %v", err)
		config = GetDefaultConfig()
		applyEnvironmentOverrides(config)
		applyLegacyEnvironmentVariables(config)
		handleLegacyPortConfig(config)
	}
	log.Printf("Configuration loaded: HTTP port=%d, HTTPS port=%d", config.ListenPortHTTP, config.ListenPortHTTPS)

	// Initialize embedded template renderer
	template, err := NewEmbeddedRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedded templates: %w", err)
	}
	log.Printf("Template renderer initialized")

	// Initialize database
	db, dbDefined, err := NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	if dbDefined {
		log.Printf("Database type: %s", config.DatabaseType)
		if err := db.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}
		log.Printf("Database connected successfully")

		if err := db.Migrate(); err != nil {
			return nil, fmt.Errorf("failed to run database migrations: %w", err)
		}
		log.Printf("Database migrations completed")
	} else {
		log.Printf("No database configured, using in-memory storage")
	}

	// Initialize request processor and handler
	requestProcessor := NewRequestProcessor(db, dbDefined, nil) // logger will be set later
	handler := NewHandler(requestProcessor)
	log.Printf("Request handler initialized")

	return &Application{
		config:       config,
		db:           db,
		dbDefined:    dbDefined,
		template:     template,
		handler:      handler,
		syslogWriter: syslogWriter,
	}, nil
}

// cleanup closes database connections and other resources
func (app *Application) cleanup() {
	log.Printf("Starting application shutdown")

	if app.dbDefined && app.db != nil {
		log.Printf("Closing database connection")
		if err := app.db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		} else {
			log.Printf("Database connection closed successfully")
		}
	}

	if app.syslogWriter != nil {
		log.Printf("Closing syslog connection")
		if err := app.syslogWriter.Close(); err != nil {
			log.Printf("Error closing syslog: %v", err)
		}
	}

	log.Printf("Application shutdown complete")
}

// setupServer configures the Echo server with middleware and routes
func (app *Application) setupServer() *echo.Echo {
	e := echo.New()

	// Direct-facing deployment: trust the TCP peer only (see AGENTS.md).
	e.IPExtractor = echo.ExtractIPDirect()

	// 301 www.ipthing.net to the apex before routing so only one host is indexed.
	e.Pre(middleware.NonWWWRedirect())

	// Configure Echo logger to use syslog
	configureEchoLogger(e, app.syslogWriter)

	// Set the logger for request processor now that we have the echo instance
	app.handler.requestProcessor.logger = e.Logger

	// Configure middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogError:     true,
		LogMethod:    true,
		LogRemoteIP:  true,
		LogLatency:   true,
		LogUserAgent: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			// Log detailed request information
			e.Logger.Infof("REQUEST: method=%s uri=%s status=%d remote_ip=%s latency=%v user_agent=%s",
				v.Method, v.URI, v.Status, v.RemoteIP, v.Latency, v.UserAgent)
			if v.Error != nil {
				e.Logger.Errorf("REQUEST_ERROR: method=%s uri=%s error=%v", v.Method, v.URI, v.Error)
			}
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(versionHeaderMiddleware())
	e.Use(rateLimitMiddleware())
	// Bound handler work (e.g. ipinfo.io). Slow-client DoS is handled by server read timeouts.
	e.Use(middleware.ContextTimeout(8 * time.Second))
	e.Use(clacksOverheadMiddleware())
	e.Use(securityHeadersMiddleware())
	e.Use(app.databaseMiddleware())

	applyServerTimeouts(e)

	// Configure TLS
	e.AutoTLSManager.Cache = autocert.DirCache("/var/www/.cache")

	// Set template renderer
	e.Renderer = app.template

	// Unknown methods on / still hit the inspector (Echo's Any is not a true catch-all).
	e.HTTPErrorHandler = app.rootMethodFallback(e)

	// Setup routes
	app.setupRoutes(e)

	return e
}

// extraRootMethods are verbs beyond Echo's Any() set (WebDAV, caches, RFC 10008 QUERY, etc.).
var extraRootMethods = []string{
	"QUERY", // RFC 10008
	"COPY", "MOVE", "MKCOL", "LOCK", "UNLOCK", "PROPPATCH", "SEARCH", "PURGE",
	"LINK", "UNLINK", "VIEW", "CHECKOUT", "CHECKIN", "MERGE", "ACL", "ORDERPATCH",
	"UPDATE", "VERSION-CONTROL", "BASELINE-CONTROL", "LABEL", "MKACTIVITY",
	"MKWORKSPACE", "BIND", "REBIND", "UNBIND",
}

// rootMethodFallback serves / for methods Echo would otherwise answer with 405,
// and records unmatched paths (404) for analytics without serving the inspector.
func (app *Application) rootMethodFallback(e *echo.Echo) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		var he *echo.HTTPError
		if errors.As(err, &he) && he.Code == http.StatusMethodNotAllowed {
			path := c.Request().URL.Path
			if path == "/" || path == "" {
				if herr := app.rootHandler(c); herr != nil {
					e.DefaultHTTPErrorHandler(herr, c)
				}
				return
			}
		}
		if errors.As(err, &he) && he.Code == http.StatusNotFound {
			app.recordNotFound(c)
		}
		e.DefaultHTTPErrorHandler(err, c)
	}
}

// recordNotFound persists a 404 probe path for analytics (same async path as /).
func (app *Application) recordNotFound(c echo.Context) {
	if app.handler == nil || app.handler.requestProcessor == nil {
		return
	}
	rp := app.handler.requestProcessor
	clientIP := rp.extractClientIP(c)
	httpReq := rp.buildHTTPRequest(c.Request(), clientIP)
	httpReq.StatusCode = http.StatusNotFound
	httpReq.ResponseFormat = "error"
	rp.QueueStore(httpReq)
}

// databaseMiddleware injects database dependencies into context
func (app *Application) databaseMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("db", app.db)
			c.Set("dbDefined", app.dbDefined)
			return next(c)
		}
	}
}

// rateLimitMiddleware caps each client IP at 5 requests per second with a burst of 10.
// Static assets are skipped so a browser page load is not blocked by favicon fetches.
func rateLimitMiddleware() echo.MiddlewareFunc {
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:  5,
		Burst: 10,
	})
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		Skipper: func(c echo.Context) bool {
			switch c.Request().URL.Path {
			case "/favicon.ico", "/favicon-16x16.png", "/favicon-32x32.png", "/apple-touch-icon.png",
				"/android-chrome-192x192.png", "/android-chrome-512x512.png",
				"/site.webmanifest", "/robots.txt", "/sitemap.xml":
				return true
			default:
				return false
			}
		},
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
	})
}

// clacksOverheadMiddleware adds the GNU Terry Pratchett Clacks header to every response.
func clacksOverheadMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Clacks-Overhead", "GNU Terry Pratchett")
			return next(c)
		}
	}
}

// securityHeadersMiddleware sets a tight CSP for the metadata page (inline CSS only, no scripts).
func securityHeadersMiddleware() echo.MiddlewareFunc {
	const csp = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			return next(c)
		}
	}
}

// setupRoutes configures all application routes
func (app *Application) setupRoutes(e *echo.Echo) {
	// Cap request bodies (1 MiB): we inspect metadata, not process payloads.
	e.Use(app.requestBodyLimitMiddleware())

	// Embedded static files
	assetFS := GetAssetFS()
	e.GET("/favicon.ico", app.serveEmbeddedAsset(assetFS, "favicon.ico"))
	e.GET("/favicon-16x16.png", app.serveEmbeddedAsset(assetFS, "favicon-16x16.png"))
	e.GET("/favicon-32x32.png", app.serveEmbeddedAsset(assetFS, "favicon-32x32.png"))
	e.GET("/apple-touch-icon.png", app.serveEmbeddedAsset(assetFS, "apple-touch-icon.png"))
	e.GET("/android-chrome-192x192.png", app.serveEmbeddedAsset(assetFS, "android-chrome-192x192.png"))
	e.GET("/android-chrome-512x512.png", app.serveEmbeddedAsset(assetFS, "android-chrome-512x512.png"))
	e.GET("/site.webmanifest", app.serveEmbeddedAsset(assetFS, "site.webmanifest"))
	e.GET("/robots.txt", app.serveEmbeddedAsset(assetFS, "robots.txt"))
	e.GET("/sitemap.xml", app.serveEmbeddedAsset(assetFS, "sitemap.xml"))
	e.GET("/privacy", app.privacyHandler)

	// Accept Echo's built-in Any methods plus common scanner/WebDAV verbs.
	// Truly unknown methods still reach rootMethodFallback on 405 for "/".
	e.Any("/", app.rootHandler)
	e.Match(extraRootMethods, "/", app.rootHandler)
}

// privacyHandler serves the short privacy notice.
func (app *Application) privacyHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "privacy", nil)
}

// serveEmbeddedAsset creates a handler for serving embedded static assets
func (app *Application) serveEmbeddedAsset(assetFS fs.FS, filename string) echo.HandlerFunc {
	return func(c echo.Context) error {
		file, err := assetFS.Open(filename)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				log.Printf("Error closing file %s: %v", filename, closeErr)
			}
		}()

		return c.Stream(http.StatusOK, getContentType(filename), file)
	}
}

// getContentType returns the appropriate content type for a file
func getContentType(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(filename, ".png"):
		return "image/png"
	case strings.HasSuffix(filename, ".webmanifest"):
		return "application/manifest+json"
	case strings.HasSuffix(filename, ".txt"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(filename, ".xml"):
		return "application/xml; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// rootHandler wraps the main handler and adds request dumping
func (app *Application) rootHandler(c echo.Context) error {
	// Dump request for debugging (existing functionality)
	//_ = dumpRequest(c.Request())

	// Delegate to main handler
	return app.handler.HandleRoot(c)
}

// startServers starts HTTP, HTTPS (TCP), and HTTP/3 (QUIC) servers based on configuration
func (app *Application) startServers() {
	// Set default ports if not configured
	httpPort := app.config.ListenPortHTTP
	if httpPort <= 0 {
		httpPort = 8080
	}

	httpsPort := app.config.ListenPortHTTPS
	if httpsPort <= 0 {
		httpsPort = 443
	}

	// Setup signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Always start HTTP server
	httpServer := app.setupServer()
	if app.config.ListenPortHTTPS != 0 {
		httpServer.Pre(browserHTTPSRedirectMiddleware(httpsPort, app.config.HostWhitelist))
	}
	go func() {
		log.Printf("Starting HTTP server on port %d", httpPort)
		if err := httpServer.Start(fmt.Sprintf(":%d", httpPort)); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	log.Printf("HTTP server started successfully on port %d", httpPort)

	// Start HTTPS (HTTP/1.1 + HTTP/2) and HTTP/3 if TLS is enabled
	var httpsServer *echo.Echo
	var h3Server *http3.Server
	if app.config.ListenPortHTTPS != 0 {
		httpsServer = app.setupServer()
		configureAutoTLS(httpsServer, app.config.HostWhitelist)
		h3Server = newHTTP3Server(httpsServer, httpsPort)
		httpsServer.Use(altSvcMiddleware(h3Server))

		addr := fmt.Sprintf(":%d", httpsPort)
		go func() {
			log.Printf("Starting HTTPS server (HTTP/1.1+HTTP/2) on port %d", httpsPort)
			if err := httpsServer.StartAutoTLS(addr); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTPS server error: %v", err)
			}
		}()
		go func() {
			log.Printf("Starting HTTP/3 (QUIC) server on UDP port %d", httpsPort)
			if err := h3Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP/3 server error: %v", err)
			}
		}()
		log.Printf("HTTPS/HTTP3 servers started successfully on port %d", httpsPort)
	}

	// Wait for interrupt signal
	log.Printf("Servers running. Press Ctrl+C to stop.")
	<-quit
	log.Printf("Received shutdown signal, starting graceful shutdown")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown HTTP server
	log.Printf("Shutting down HTTP server")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	} else {
		log.Printf("HTTP server shut down successfully")
	}

	// Shutdown HTTPS server if running
	if httpsServer != nil {
		log.Printf("Shutting down HTTPS server")
		if err := httpsServer.Shutdown(ctx); err != nil {
			log.Printf("HTTPS server shutdown error: %v", err)
		} else {
			log.Printf("HTTPS server shut down successfully")
		}
	}

	if h3Server != nil {
		log.Printf("Shutting down HTTP/3 server")
		if err := h3Server.Close(); err != nil {
			log.Printf("HTTP/3 server shutdown error: %v", err)
		} else {
			log.Printf("HTTP/3 server shut down successfully")
		}
	}

	log.Printf("All servers shut down successfully")
}

func applyServerTimeouts(e *echo.Echo) {
	// Defend against slowloris / slow-body clients at the net/http layer.
	// Echo's Timeout middleware is discouraged; these server timeouts are the right tool.
	for _, s := range []*http.Server{e.Server, e.TLSServer} {
		s.ReadHeaderTimeout = 5 * time.Second
		s.ReadTimeout = 10 * time.Second
		s.WriteTimeout = 15 * time.Second
		s.IdleTimeout = 60 * time.Second
	}
}

func configureAutoTLS(e *echo.Echo, hostWhitelist []string) {
	e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(hostWhitelist...)
	e.AutoTLSManager.Cache = autocert.DirCache("/var/www/.cache")
}

func newHTTP3Server(e *echo.Echo, listenPort int) *http3.Server {
	tlsConf := &tls.Config{
		GetCertificate: e.AutoTLSManager.GetCertificate,
		NextProtos:     []string{acme.ALPNProto, http3.NextProtoH3},
		MinVersion:     tls.VersionTLS13,
	}
	return &http3.Server{
		Handler:     e,
		Addr:        fmt.Sprintf(":%d", listenPort),
		Port:        listenPort,
		TLSConfig:   http3.ConfigureTLSConfig(tlsConf),
		IdleTimeout: 60 * time.Second,
	}
}

// browserHTTPSRedirectMiddleware sends browsers (including Googlebot) on plain HTTP to HTTPS
// so one scheme is indexed. curl and scripts keep getting JSON over HTTP. Only hosts in the
// AutoTLS whitelist are redirected, as those are the only ones with a certificate.
func browserHTTPSRedirectMiddleware(httpsPort int, hostWhitelist []string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.TLS != nil || (req.Method != http.MethodGet && req.Method != http.MethodHead) ||
				!IsWebBrowser(req.UserAgent()) {
				return next(c)
			}
			host := req.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if !slices.Contains(hostWhitelist, host) {
				return next(c)
			}
			if httpsPort != 443 {
				host = net.JoinHostPort(host, strconv.Itoa(httpsPort))
			}
			return c.Redirect(http.StatusMovedPermanently, "https://"+host+req.RequestURI)
		}
	}
}

func altSvcMiddleware(h3 *http3.Server) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if h3 != nil {
				_ = h3.SetQUICHeaders(c.Response().Header())
			}
			return next(c)
		}
	}
}

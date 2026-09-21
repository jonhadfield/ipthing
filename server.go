package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
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

// version is set by the build process via ldflags
var version = "dev"

func main() {
	// Initialize application
	app, err := initializeApplication()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.cleanup()

	// Log successful startup
	log.Printf("ipthing application initialized successfully")
	log.Printf("Version: %s", version)

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
	e.Use(app.databaseMiddleware())

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

// extraRootMethods are common scanner/WebDAV verbs beyond Echo's Any() set.
var extraRootMethods = []string{
	"COPY", "MOVE", "MKCOL", "LOCK", "UNLOCK", "PROPPATCH", "SEARCH", "PURGE",
	"LINK", "UNLINK", "VIEW", "CHECKOUT", "CHECKIN", "MERGE", "ACL", "ORDERPATCH",
	"UPDATE", "VERSION-CONTROL", "BASELINE-CONTROL", "LABEL", "MKACTIVITY",
	"MKWORKSPACE", "BIND", "REBIND", "UNBIND",
}

// rootMethodFallback serves / for methods Echo would otherwise answer with 405.
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
		e.DefaultHTTPErrorHandler(err, c)
	}
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

// setupRoutes configures all application routes
func (app *Application) setupRoutes(e *echo.Echo) {
	// Cap request bodies: we inspect metadata, not process payloads.
	e.Use(middleware.BodyLimit("64KB"))

	// Embedded static files
	assetFS := GetAssetFS()
	e.GET("/favicon.ico", app.serveEmbeddedAsset(assetFS, "favicon.ico"))
	e.GET("/favicon-16x16.png", app.serveEmbeddedAsset(assetFS, "favicon-16x16.png"))
	e.GET("/favicon-32x32.png", app.serveEmbeddedAsset(assetFS, "favicon-32x32.png"))
	e.GET("/apple-touch-icon.png", app.serveEmbeddedAsset(assetFS, "apple-touch-icon.png"))

	// Accept Echo's built-in Any methods plus common scanner/WebDAV verbs.
	// Truly unknown methods still reach rootMethodFallback on 405 for "/".
	e.Any("/", app.rootHandler)
	e.Match(extraRootMethods, "/", app.rootHandler)
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
		Handler:   e,
		Addr:      fmt.Sprintf(":%d", listenPort),
		Port:      listenPort,
		TLSConfig: http3.ConfigureTLSConfig(tlsConf),
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

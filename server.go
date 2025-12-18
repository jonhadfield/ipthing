package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
	syslogWriter *syslog.Writer
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

	// Load configuration
	config, err := ReadConfig("config.json")
	if err != nil {
		log.Printf("Warning: %v", err)
		config = GetDefaultConfig()
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

	// Configure Echo logger to use syslog
	configureEchoLogger(e, app.syslogWriter)

	// Set the logger for request processor now that we have the echo instance
	app.handler.requestProcessor.logger = e.Logger

	// Configure middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		LogMethod:   true,
		LogRemoteIP: true,
		LogLatency:  true,
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

	// Setup routes
	app.setupRoutes(e)

	return e
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
	// Embedded static files
	assetFS := GetAssetFS()
	e.GET("/favicon.ico", app.serveEmbeddedAsset(assetFS, "favicon.ico"))
	e.GET("/favicon-16x16.png", app.serveEmbeddedAsset(assetFS, "favicon-16x16.png"))
	e.GET("/favicon-32x32.png", app.serveEmbeddedAsset(assetFS, "favicon-32x32.png"))
	e.GET("/apple-touch-icon.png", app.serveEmbeddedAsset(assetFS, "apple-touch-icon.png"))

	// Main route
	e.GET("/", app.rootHandler)
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

// startServers starts both HTTP and HTTPS servers based on configuration
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

	// Start HTTPS server if TLS is enabled
	var httpsServer *echo.Echo
	if app.config.ListenPortHTTPS != 0 {
		httpsServer = app.setupServer()
		go func() {
			log.Printf("Starting HTTPS server on port %d", httpsPort)
			startTLS(httpsServer, app.config.HostWhitelist, httpsPort)
		}()
		log.Printf("HTTPS server started successfully on port %d", httpsPort)
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

	log.Printf("All servers shut down successfully")
}

// startTLS starts the server with TLS/HTTPS support
func startTLS(e *echo.Echo, hostWhitelist []string, listenPort int) {
	e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(hostWhitelist...)
	e.AutoTLSManager.Cache = autocert.DirCache("/var/www/.cache")
	e.Logger.Fatal(e.StartAutoTLS(fmt.Sprintf(":%d", listenPort)))
}

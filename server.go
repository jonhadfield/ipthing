package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/joho/godotenv/autoload"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	// Initialize application
	app, err := initializeApplication()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.cleanup()

	// Setup and start servers
	app.startServers()
}

// Application holds all application dependencies
type Application struct {
	config    *Config
	db        Database
	dbDefined bool
	template  *EmbeddedRenderer
	handler   *Handler
}

// initializeApplication sets up all application dependencies
func initializeApplication() (*Application, error) {
	// Load configuration
	config, err := ReadConfig("config.json")
	if err != nil {
		log.Printf("Warning: %v", err)
		config = GetDefaultConfig()
	}

	// Initialize embedded template renderer
	template, err := NewEmbeddedRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedded templates: %w", err)
	}

	// Initialize database
	db, dbDefined, err := NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	if dbDefined {
		if err := db.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}

		if err := db.Migrate(); err != nil {
			return nil, fmt.Errorf("failed to run database migrations: %w", err)
		}

		if err := db.Verify(); err != nil {
			return nil, fmt.Errorf("database verification failed: %w", err)
		}
	}

	// Initialize request processor and handler
	requestProcessor := NewRequestProcessor(db, dbDefined, nil) // logger will be set later
	handler := NewHandler(requestProcessor)

	return &Application{
		config:    config,
		db:        db,
		dbDefined: dbDefined,
		template:  template,
		handler:   handler,
	}, nil
}

// cleanup closes database connections and other resources
func (app *Application) cleanup() {
	if app.dbDefined && app.db != nil {
		app.db.Close()
	}
}

// setupServer configures the Echo server with middleware and routes
func (app *Application) setupServer() *echo.Echo {
	e := echo.New()

	// Set the logger for request processor now that we have the echo instance
	app.handler.requestProcessor.logger = e.Logger

	// Configure middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(app.databaseMiddleware())

	// Configure TLS
	e.AutoTLSManager.Cache = autocert.DirCache(AutoTLSCacheDir)

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
		defer file.Close()

		return c.Stream(http.StatusOK, getContentType(filename), file)
	}
}

// getContentType returns the appropriate content type for a file
func getContentType(filename string) string {
	switch {
	case strings.HasSuffix(filename, ExtensionICO):
		return MimeTypeIcon
	case strings.HasSuffix(filename, ExtensionPNG):
		return MimeTypePNG
	case strings.HasSuffix(filename, ExtensionWebmanifest):
		return MimeTypeManifest
	default:
		return MimeTypeOctetStream
	}
}

// rootHandler wraps the main handler and adds request dumping
func (app *Application) rootHandler(c echo.Context) error {
	// Dump request for debugging (existing functionality)
	//_ = dumpRequest(c.Request())

	// Delegate to main handler
	return app.handler.HandleRoot(c, app.config.BehindProxy)
}

// startServers starts both HTTP and HTTPS servers based on configuration
func (app *Application) startServers() {
	// Set default ports if not configured
	httpPort := app.config.ListenPortHTTP
	if httpPort <= 0 {
		httpPort = DefaultHTTPPort
	}

	httpsPort := app.config.ListenPortHTTPS
	if httpsPort <= 0 {
		httpsPort = DefaultHTTPSPort
	}

	// Create a slice to track servers for shutdown
	var servers []*echo.Echo

	// Always start HTTP server
	httpServer := app.setupServer()
	servers = append(servers, httpServer)
	go func() {
		log.Printf("Starting HTTP server on port %d", httpPort)
		if err := httpServer.Start(fmt.Sprintf(":%d", httpPort)); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server if TLS is enabled
	var httpsServer *echo.Echo
	if app.config.ListenPortHTTPS != 0 {
		httpsServer = app.setupServer()
		servers = append(servers, httpsServer)
		go func() {
			log.Printf("Starting HTTPS server on port %d", httpsPort)
			if err := startTLS(httpsServer, app.config.HostWhitelist, httpsPort); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTPS server error: %v", err)
			}
		}()
	}

	// Set up graceful shutdown
	quit := make(chan os.Signal, MaxSignalBufferSize)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	log.Printf("Servers started. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-quit
	log.Println("Shutting down servers...")

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
	defer cancel()

	// Shutdown all servers
	for i, server := range servers {
		if server != nil {
			if i == 0 {
				log.Printf("Shutting down HTTP server on port %d", httpPort)
			} else {
				log.Printf("Shutting down HTTPS server on port %d", httpsPort)
			}

			if err := server.Shutdown(ctx); err != nil {
				log.Printf("Server shutdown error: %v", err)
				// Force close if graceful shutdown fails
				if err := server.Close(); err != nil {
					log.Printf("Server force close error: %v", err)
				}
			}
		}
	}

	log.Println("Servers stopped")
}

// startTLS starts the server with TLS/HTTPS support
func startTLS(e *echo.Echo, hostWhitelist []string, listenPort int) error {
	e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(hostWhitelist...)
	e.AutoTLSManager.Cache = autocert.DirCache(AutoTLSCacheDir)
	return e.StartAutoTLS(fmt.Sprintf(":%d", listenPort))
}

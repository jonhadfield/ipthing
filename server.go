package main

import (
	"encoding/json"
	"fmt"
	_ "github.com/joho/godotenv/autoload"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/acme/autocert"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type Req struct {
	IPAddr         string `json:"ip_addr,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`
	Country        string `json:"country,omitempty"`
	Visitor        string `json:"visitor,omitempty"`
	Host           string `json:"host,omitempty"`
	Accept         string `json:"accept,omitempty"`
	AcceptEncoding string `json:"accept_encoding,omitempty"`
	AcceptLanguage string `json:"accept_language,omitempty"`
	DNT            string `json:"dnt,omitempty"`
	Language       string `json:"language,omitempty"`
	Referer        string `json:"referer,omitempty"`
	Method         string `json:"method,omitempty"`
	MIMEType       string `json:"mime_type,omitempty"`
	Charset        string `json:"charset,omitempty"`
	XFF            string `json:"xff,omitempty"`
	XRI            string `json:"xri,omitempty"`
}

func stripPrivateIPs(xff string) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.Contains(parts[i], ":") {
			return strings.Join(parts[:i], ",")
		}
	}

	return xff

}

func stripXFF(xff string, trustedPos int, stripPrivate bool) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	if len(parts) <= trustedPos {
		return ""
	}

	realTrustedPos := len(parts) - trustedPos

	return strings.Join(parts[:realTrustedPos], ",")
}

func populateReq(r *http.Request) *Req {
	sXFF := stripXFF(r.Header.Get("X-Forwarded-For"), 1, false)

	req := &Req{
		IPAddr:         r.Header.Get("Cf-Connecting-Ip"),
		Country:        r.Header.Get("Cf-IPcountry"),
		Visitor:        r.Header.Get("Cf-Visitor"),
		UserAgent:      r.UserAgent(),
		Host:           r.Host,
		Accept:         r.Header.Get("Accept"),
		AcceptEncoding: r.Header.Get("Accept-Encoding"),
		AcceptLanguage: r.Header.Get("Accept-Language"),
		DNT:            r.Header.Get("DNT"),
		Language:       r.Header.Get("Language"),
		Referer:        r.Header.Get("Referer"),
		Method:         r.Method,
		MIMEType:       r.Header.Get("Content-Type"),
		Charset:        r.Header.Get("Charset"),
		XFF:            sXFF,
	}

	return req
}

type Config struct {
	UseTLS                   bool     `json:"useTLS"`
	HostWhitelist            []string `json:"hostWhitelist"`
	ListenPort               int      `json:"listenPort"`
	DatabasePath             string   `json:"databasePath,omitempty"`
	DatabaseType             string   `json:"databaseType,omitempty"`
	DatabaseConnectionString string   `json:"databaseConnectionString,omitempty"`
}

func readConfig(filePath string) (*Config, error) {
	envConfig := os.Getenv("IPTHING_CONFIG")
	var config Config
	if envConfig != "" {
		err := json.Unmarshal([]byte(envConfig), &config)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config from environment variable: %w", err)
		}
	} else {
		f, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err = json.Unmarshal(f, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}

	// Override with individual environment variables
	if dbType := os.Getenv("IPTHING_DB_TYPE"); dbType != "" {
		config.DatabaseType = dbType
	}

	if dbPath := os.Getenv("IPTHING_SQLITE_PATH"); dbPath != "" {
		config.DatabasePath = dbPath
	}

	if pgConn := os.Getenv("IPTHING_POSTGRES_URL"); pgConn != "" {
		config.DatabaseConnectionString = pgConn
	}

	// Alternative PostgreSQL connection string env var names
	if config.DatabaseConnectionString == "" {
		if pgConn := os.Getenv("DATABASE_URL"); pgConn != "" {
			log.Println("using DATABASE_URL for PostgreSQL connection string")
			config.DatabaseConnectionString = pgConn
			config.DatabaseType = "postgres"
		}
	}

	log.Printf("port: %d", config.ListenPort)
	log.Printf("use tls: %t", config.UseTLS)

	return &config, nil
}

func main() {
	t := &Template{
		templates: template.Must(template.ParseGlob("public/views/*.html")),
	}

	config, err := readConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	// Initialize database
	db, err := NewDatabase(config)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	if err := db.Connect(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}

	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("db", db)
			return next(c)
		}
	})
	e.AutoTLSManager.Cache = autocert.DirCache("/var/www/.cache")
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	// Cache certificates to avoid issues with rate limits (https://letsencrypt.org/docs/rate-limits)
	e.Renderer = t
	e.File("/favicon.ico", "public/assets/favicon.ico")
	e.File("/favicon-16x16.png", "public/assets/favicon-16x16.png")
	e.File("/favicon-32x32.png", "public/assets/favicon-32x32.png")
	e.File("/apple-touch-icon.png", "public/assets/apple-touch-icon.png")
	e.GET("/", func(c echo.Context) error {
		r := regexp.MustCompile(`.*(Mozilla|AppleWebKit|Trident|Presto|Gecko|KHTML|Blink|Lynx|Links|w3m|elinks).*`)

		_ = dumpRequest(c.Request())
		firstUntrusted, err := parseXFF(e, c.Request(), true, true, true)
		if err != nil {
			return err
		}
		e.Logger.Info(firstUntrusted)

		// Get database from context
		db := c.Get("db").(Database)
		req := populateReq(c.Request())

		// Determine the actual client IP
		clientIP := req.IPAddr
		if clientIP == "" {
			clientIP = firstUntrusted
		}
		if clientIP == "" {
			clientIP = c.RealIP()
		}

		// Store request data in database
		if clientIP != "" {
			go func() {
				// Prepare query params as JSON first for duplicate checking
				queryParams := make(map[string][]string)
				for k, v := range c.Request().URL.Query() {
					queryParams[k] = v
				}
				queryParamsJSON, _ := json.Marshal(queryParams)

				// Check for duplicate request
				fingerprint := &RequestFingerprint{
					IP:          clientIP,
					Path:        c.Request().URL.Path,
					QueryParams: string(queryParamsJSON),
					UserAgent:   c.Request().UserAgent(),
				}

				// Check for duplicates within the last 5 minutes
				isDuplicate, err := db.IsDuplicateRequest(fingerprint, 5*time.Minute)
				if err != nil {
					e.Logger.Errorf("Failed to check for duplicate request: %v", err)
				}

				if isDuplicate {
					e.Logger.Debugf("Skipping duplicate request from %s to %s", clientIP, c.Request().URL.Path)
					return
				}

				// Get or fetch IP info
				ipInfo, err := getOrFetchIPInfo(db, clientIP)
				if err != nil {
					e.Logger.Errorf("Failed to get IP info: %v", err)
				} else {
					e.Logger.Infof("IP info for %s: %+v", clientIP, ipInfo)
				}

				// Prepare headers as JSON
				headers := make(map[string][]string)
				for k, v := range c.Request().Header {
					headers[k] = v
				}
				headersJSON, _ := json.Marshal(headers)

				// Save HTTP request
				httpReq := &HTTPRequest{
					IP:          clientIP,
					Method:      c.Request().Method,
					Path:        c.Request().URL.Path,
					UserAgent:   c.Request().UserAgent(),
					Referer:     c.Request().Header.Get("Referer"),
					Headers:     string(headersJSON),
					QueryParams: string(queryParamsJSON),
					Timestamp:   time.Now(),
				}

				if err := db.SaveHTTPRequest(httpReq); err != nil {
					e.Logger.Errorf("Failed to save HTTP request: %v", err)
				}
			}()
		}

		switch {
		case !r.MatchString(c.Request().UserAgent()):
			consoleData, _ := json.MarshalIndent(req, "", "  ")
			return c.Render(http.StatusOK, "console", string(consoleData))
		default:
			return c.Render(http.StatusOK, "web", req)
		}
	})

	if config.UseTLS {
		startTLS(e, config.HostWhitelist, config.ListenPort)
	} else {
		startHTTP(e, config.ListenPort)
	}
}

func startTLS(e *echo.Echo, hostWhitelist []string, listenPort int) {
	e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(hostWhitelist...)
	e.AutoTLSManager.Cache = autocert.DirCache("/var/www/.cache")
	e.Logger.Fatal(e.StartAutoTLS(fmt.Sprintf(":%d", listenPort)))

}

func startHTTP(e *echo.Echo, listenPort int) {
	if err := e.Start(fmt.Sprintf(":%d", 80)); err != nil {
		log.Fatal(err)
	}
}

func parseXFF(e *echo.Echo, r *http.Request, trustLoopback, trustLinkLocal, trustPrivateNet bool) (string, error) {
	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(trustLoopback),
		echo.TrustLinkLocal(trustLinkLocal),
		echo.TrustPrivateNet(trustPrivateNet),
	)

	ip := e.IPExtractor(r)
	echo.ExtractIPFromXFFHeader()

	return ip, nil
}

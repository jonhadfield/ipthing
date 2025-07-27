package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

const (
	envIPThingConfig = "IPTHING_CONFIG"
)

type Config struct {
	BehindProxy     bool     `json:"behindProxy"`
	HostWhitelist   []string `json:"hostWhitelist"`
	ListenPortHTTPS int      `json:"listenPortHTTPS"`
	ListenPortHTTP  int      `json:"listenPortHTTP"`
	// Legacy field for backward compatibility
	ListenPort               int    `json:"listenPort,omitempty"`
	DatabasePath             string `json:"databasePath,omitempty"`
	DatabaseType             string `json:"databaseType,omitempty"`
	DatabaseConnectionString string `json:"databaseConnectionString,omitempty"`
}

// ReadConfig reads configuration from file or environment variable with fallbacks
func ReadConfig(filePath string) (*Config, error) {
	log.Printf("ReadConfig called with filePath: %s", filePath)
	log.Printf("Environment check - IPTHING_HTTP_PORT: %s", os.Getenv("IPTHING_HTTP_PORT"))
	
	config, err := loadConfigFromSources(filePath)
	if err != nil {
		return nil, err
	}

	// If no config was loaded, use default config
	if config == nil {
		log.Println("No config loaded from sources, using default config")
		config = GetDefaultConfig()
	}

	// Apply environment variable overrides
	applyEnvironmentOverrides(config)

	// Apply legacy environment variable support
	applyLegacyEnvironmentVariables(config)

	// Handle legacy listenPort field for backward compatibility
	handleLegacyPortConfig(config)

	logConfigSettings(config)

	return config, nil
}

// loadConfigFromSources attempts to load config from environment variable first, then file
func loadConfigFromSources(filePath string) (*Config, error) {
	envConfig := os.Getenv(envIPThingConfig)
	var config Config

	if envConfig != "" {
		err := json.Unmarshal([]byte(envConfig), &config)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config from environment variable: %w", err)
		}
		return &config, nil
	}

	if filePath != "" {
		f, err := os.ReadFile(filePath)
		if err != nil {
			// Don't return error if file doesn't exist, just return nil config
			// This allows environment variables to still be used
			log.Printf("Config file not found: %v", err)
			return nil, nil
		}

		if err = json.Unmarshal(f, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		return &config, nil
	}

	return nil, nil
}

// applyEnvironmentOverrides applies individual environment variable overrides
func applyEnvironmentOverrides(config *Config) {
	if config == nil {
		return
	}

	if dbType := os.Getenv("IPTHING_DB_TYPE"); dbType != "" {
		config.DatabaseType = dbType
		log.Printf("Applied IPTHING_DB_TYPE: %s", dbType)
	}

	if dbPath := os.Getenv("IPTHING_SQLITE_PATH"); dbPath != "" {
		config.DatabasePath = dbPath
	}

	if pgConn := os.Getenv("IPTHING_POSTGRES_URL"); pgConn != "" {
		config.DatabaseConnectionString = pgConn
	}

	// Port configuration environment variables
	if httpPort := os.Getenv("IPTHING_HTTP_PORT"); httpPort != "" {
		if port, err := parseInt(httpPort); err == nil {
			config.ListenPortHTTP = port
			log.Printf("Applied IPTHING_HTTP_PORT: %d", port)
		} else {
			log.Printf("Failed to parse IPTHING_HTTP_PORT: %s, error: %v", httpPort, err)
		}
	}

	if httpsPort := os.Getenv("IPTHING_HTTPS_PORT"); httpsPort != "" {
		if port, err := parseInt(httpsPort); err == nil {
			config.ListenPortHTTPS = port
			log.Printf("Applied IPTHING_HTTPS_PORT: %d", port)
		} else {
			log.Printf("Failed to parse IPTHING_HTTPS_PORT: %s, error: %v", httpsPort, err)
		}
	}

	// Host whitelist configuration
	if hostWhitelist := os.Getenv("IPTHING_HOST_WHITELIST"); hostWhitelist != "" {
		hosts := strings.Split(hostWhitelist, ",")
		var cleanedHosts []string
		for _, host := range hosts {
			trimmed := strings.TrimSpace(host)
			if trimmed != "" {
				cleanedHosts = append(cleanedHosts, trimmed)
			}
		}
		if len(cleanedHosts) > 0 {
			config.HostWhitelist = cleanedHosts
		}
	}
}

// applyLegacyEnvironmentVariables handles legacy environment variable names
func applyLegacyEnvironmentVariables(config *Config) {
	if config == nil {
		return
	}

	// Alternative PostgreSQL connection string env var names
	if config.DatabaseConnectionString == "" {
		if pgConn := os.Getenv("DATABASE_URL"); pgConn != "" {
			log.Println("using DATABASE_URL for PostgreSQL connection string")
			config.DatabaseConnectionString = pgConn
			config.DatabaseType = "postgres"
		}
	}
}

// logConfigSettings logs the current configuration settings
func logConfigSettings(config *Config) {
	if config == nil {
		return
	}

	log.Printf("HTTP port: %d", config.ListenPortHTTP)
	log.Printf("HTTPS port: %d", config.ListenPortHTTPS)
	if len(config.HostWhitelist) > 0 {
		log.Printf("Host whitelist: %v", config.HostWhitelist)
	}
}

// GetDefaultConfig returns a default configuration
func GetDefaultConfig() *Config {
	return &Config{
		ListenPortHTTP:  DefaultHTTPPort,
		ListenPortHTTPS: DefaultHTTPSPort,
	}
}

// handleLegacyPortConfig handles backward compatibility for the old listenPort field
func handleLegacyPortConfig(config *Config) {
	if config == nil {
		return
	}

	// If legacy listenPort is set but new ports are not, use legacy value
	if config.ListenPort > 0 {
		if config.ListenPortHTTP <= 0 {
			config.ListenPortHTTP = config.ListenPort
		}
		if config.ListenPortHTTPS <= 0 {
			config.ListenPortHTTPS = config.ListenPort
		}
	}
}

// parseInt is a helper function to parse integer strings
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

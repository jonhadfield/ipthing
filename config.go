package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
)

const (
	envIPThingConfig = "IPTHING_CONFIG"
)

type Config struct {
	UseTLS          bool     `json:"useTLS"`
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
	config, err := loadConfigFromSources(filePath)
	if err != nil {
		return nil, err
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
			return nil, fmt.Errorf("failed to read config file: %w", err)
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
		}
	}

	if httpsPort := os.Getenv("IPTHING_HTTPS_PORT"); httpsPort != "" {
		if port, err := parseInt(httpsPort); err == nil {
			config.ListenPortHTTPS = port
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
	log.Printf("use tls: %t", config.UseTLS)
}

// GetDefaultConfig returns a default configuration
func GetDefaultConfig() *Config {
	return &Config{
		UseTLS:          false,
		ListenPortHTTP:  8080,
		ListenPortHTTPS: 443,
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

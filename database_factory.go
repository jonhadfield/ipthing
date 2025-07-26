package main

import (
	"fmt"
	"strings"
	"time"
)

func NewDatabase(config *Config) (Database, error) {
	dbType := strings.ToLower(config.DatabaseType)
	if dbType == "" {
		dbType = "sqlite"
	}

	switch dbType {
	case "sqlite":
		dbPath := config.DatabasePath
		if dbPath == "" {
			dbPath = "ipthing.db"
		}
		return NewSQLiteDB(dbPath), nil

	case "postgres", "postgresql":
		if config.DatabaseConnectionString == "" {
			return nil, fmt.Errorf("PostgreSQL connection string is required")
		}
		return NewPostgresDB(config.DatabaseConnectionString), nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.DatabaseType)
	}
}

func shouldUpdateIPInfo(lastUpdate time.Time) bool {
	return time.Since(lastUpdate) > 24*time.Hour
}


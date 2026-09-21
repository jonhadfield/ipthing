package main

import (
	"fmt"
	"log"
	"strings"
)

func NewDatabase(config *Config) (Database, bool, error) {
	if config == nil {
		log.Println("No configuration provided, running without database")
		return &NoOpDB{}, false, nil
	}

	dbType := strings.ToLower(config.DatabaseType)

	// Check if any database configuration is provided
	if dbType == "" && config.DatabasePath == "" && config.DatabaseConnectionString == "" {
		log.Println("No database configuration provided, running without database")
		return &NoOpDB{}, false, nil
	}

	// If database path or connection string is provided but no type, infer the type
	if dbType == "" {
		if config.DatabaseConnectionString != "" {
			dbType = "postgres"
		} else if config.DatabasePath != "" {
			dbType = "sqlite"
		}
	}

	log.Printf("Using database: %s", dbType)

	switch dbType {
	case "sqlite":
		dbPath := config.DatabasePath
		if dbPath == "" {
			dbPath = "ipthing.db"
		}
		return NewSQLiteDB(dbPath), true, nil

	case "postgres", "postgresql":
		if config.DatabaseConnectionString == "" {
			return nil, false, fmt.Errorf("PostgreSQL connection string is required")
		}
		return NewPostgresDB(config.DatabaseConnectionString), true, nil

	case "mariadb", "mysql":
		if config.DatabaseConnectionString == "" {
			return nil, false, fmt.Errorf("MariaDB/MySQL connection string is required")
		}
		return NewMariaDB(config.DatabaseConnectionString), true, nil

	default:
		return nil, false, fmt.Errorf("unsupported database type: %s", config.DatabaseType)
	}
}

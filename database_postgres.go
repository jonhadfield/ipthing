package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

type PostgresDB struct {
	BaseDB
	connectionString string
}

func NewPostgresDB(connectionString string) *PostgresDB {
	return &PostgresDB{
		connectionString: connectionString,
	}
}

func (p *PostgresDB) Connect() error {
	db, err := sql.Open("postgres", p.connectionString)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	p.db = db
	return nil
}

func (p *PostgresDB) Migrate() error {
	ipInfoSchema := `
	CREATE TABLE IF NOT EXISTS ip_info (
		ip TEXT PRIMARY KEY,
		city TEXT,
		region TEXT,
		country TEXT,
		loc TEXT,
		org TEXT,
		postal TEXT,
		timezone TEXT,
		hostname TEXT,
		bogon BOOLEAN,
		last_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	httpRequestSchema := `
	CREATE TABLE IF NOT EXISTS http_requests (
		id SERIAL PRIMARY KEY,
		ip TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		user_agent TEXT,
		referer TEXT,
		headers TEXT,
		query_params TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		tls_version INTEGER,
		tls_cipher_suite INTEGER,
		tls_server_name TEXT,
		tls_negotiated_protocol TEXT,
		proto TEXT,
		content_length BIGINT,
		remote_addr TEXT,
		request_uri TEXT,
		host TEXT,
		scheme TEXT,
		content_type TEXT,
		body TEXT,
		FOREIGN KEY (ip) REFERENCES ip_info(ip)
	);`

	indexSchema := `
	CREATE INDEX IF NOT EXISTS idx_requests_ip ON http_requests(ip);
	CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON http_requests(timestamp);
	CREATE INDEX IF NOT EXISTS idx_requests_fingerprint ON http_requests(ip, path, query_params, user_agent);`

	if _, err := p.db.Exec(ipInfoSchema); err != nil {
		return fmt.Errorf("failed to create ip_info table: %w", err)
	}

	if _, err := p.db.Exec(httpRequestSchema); err != nil {
		return fmt.Errorf("failed to create http_requests table: %w", err)
	}

	if _, err := p.db.Exec(indexSchema); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

func (p *PostgresDB) SaveHTTPRequest(req *HTTPRequest) error {
	query := `
	INSERT INTO http_requests (ip, method, path, user_agent, referer, headers, query_params, timestamp,
		tls_version, tls_cipher_suite, tls_server_name, tls_negotiated_protocol,
		proto, content_length, remote_addr, request_uri, host, scheme, content_type, body)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	RETURNING id`

	err := p.db.QueryRow(query, req.IP, req.Method, req.Path, req.UserAgent, req.Referer,
		req.Headers, req.QueryParams, req.Timestamp,
		req.TLSVersion, req.TLSCipherSuite, req.TLSServerName, req.TLSNegotiatedProtocol,
		req.Proto, req.ContentLength, req.RemoteAddr, req.RequestURI, req.Host, req.Scheme,
		req.ContentType, req.Body).Scan(&req.ID)
	return err
}

func (p *PostgresDB) Verify() error {
	// Verify and fix ip_info table
	if err := p.verifyAndFixTable("ip_info", map[string]string{
		"ip":          "TEXT PRIMARY KEY",
		"city":        "TEXT",
		"region":      "TEXT",
		"country":     "TEXT",
		"loc":         "TEXT",
		"org":         "TEXT",
		"postal":      "TEXT",
		"timezone":    "TEXT",
		"hostname":    "TEXT",
		"bogon":       "BOOLEAN",
		"last_update": "TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
	}); err != nil {
		return fmt.Errorf("ip_info table verification/fix failed: %w", err)
	}

	// Verify and fix http_requests table
	if err := p.verifyAndFixTable("http_requests", map[string]string{
		"id":                      "SERIAL PRIMARY KEY",
		"ip":                      "TEXT NOT NULL",
		"method":                  "TEXT NOT NULL",
		"path":                    "TEXT NOT NULL",
		"user_agent":              "TEXT",
		"referer":                 "TEXT",
		"headers":                 "TEXT",
		"query_params":            "TEXT",
		"timestamp":               "TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
		"tls_version":             "INTEGER",
		"tls_cipher_suite":        "INTEGER",
		"tls_server_name":         "TEXT",
		"tls_negotiated_protocol": "TEXT",
		"proto":                   "TEXT",
		"content_length":          "BIGINT",
		"remote_addr":             "TEXT",
		"request_uri":             "TEXT",
		"host":                    "TEXT",
		"scheme":                  "TEXT",
		"content_type":            "TEXT",
		"body":                    "TEXT",
	}); err != nil {
		return fmt.Errorf("http_requests table verification/fix failed: %w", err)
	}

	// Verify and fix indexes
	expectedIndexes := map[string]string{
		"idx_requests_ip":          "CREATE INDEX IF NOT EXISTS idx_requests_ip ON http_requests(ip)",
		"idx_requests_timestamp":   "CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON http_requests(timestamp)",
		"idx_requests_fingerprint": "CREATE INDEX IF NOT EXISTS idx_requests_fingerprint ON http_requests(ip, path, query_params, user_agent)",
	}

	for indexName, createSQL := range expectedIndexes {
		if err := p.verifyAndFixIndex(indexName, createSQL); err != nil {
			return fmt.Errorf("index %s verification/fix failed: %w", indexName, err)
		}
	}

	return nil
}

func (p *PostgresDB) verifyAndFixTable(tableName string, expectedColumns map[string]string) error {
	// Check if table exists
	var count int
	err := p.db.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name = $1`, tableName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if count == 0 {
		// Table doesn't exist, create it
		return p.createTable(tableName, expectedColumns)
	}

	// Get existing table columns
	rows, err := p.db.Query(`
		SELECT column_name 
		FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1`, tableName)
	if err != nil {
		return fmt.Errorf("failed to get table columns: %w", err)
	}
	defer rows.Close()

	actualColumns := make(map[string]bool)
	for rows.Next() {
		var columnName string
		err := rows.Scan(&columnName)
		if err != nil {
			return fmt.Errorf("failed to scan column name: %w", err)
		}
		actualColumns[columnName] = true
	}

	// Add missing columns
	for columnName, columnDef := range expectedColumns {
		if !actualColumns[columnName] {
			if err := p.addColumn(tableName, columnName, columnDef); err != nil {
				return fmt.Errorf("failed to add column %s to table %s: %w", columnName, tableName, err)
			}
		}
	}

	return nil
}

func (p *PostgresDB) verifyAndFixIndex(indexName, createSQL string) error {
	var count int
	err := p.db.QueryRow(`
		SELECT COUNT(*) 
		FROM pg_indexes 
		WHERE schemaname = 'public' AND indexname = $1`, indexName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}

	if count == 0 {
		// Index doesn't exist, create it
		if _, err := p.db.Exec(createSQL); err != nil {
			return fmt.Errorf("failed to create index %s: %w", indexName, err)
		}
	}

	return nil
}

func (p *PostgresDB) createTable(tableName string, columns map[string]string) error {
	var columnDefs []string
	for columnName, columnDef := range columns {
		columnDefs = append(columnDefs, columnName+" "+columnDef)
	}

	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(columnDefs, ", "))
	if _, err := p.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}

	return nil
}

func (p *PostgresDB) addColumn(tableName, columnName, columnDef string) error {
	// Remove constraints that aren't supported in ALTER TABLE ADD COLUMN
	cleanDef := columnDef
	if strings.Contains(strings.ToUpper(cleanDef), "PRIMARY KEY") ||
		strings.Contains(strings.ToUpper(cleanDef), "SERIAL") {
		// Can't add primary key or serial via ALTER TABLE, skip this column
		return nil
	}

	alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, cleanDef)
	if _, err := p.db.Exec(alterSQL); err != nil {
		return fmt.Errorf("failed to add column: %w", err)
	}

	return nil
}

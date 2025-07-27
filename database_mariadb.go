package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type MariaDB struct {
	BaseDB
	connectionString string
}

func NewMariaDB(connectionString string) *MariaDB {
	return &MariaDB{
		connectionString: connectionString,
	}
}

func (m *MariaDB) Connect() error {
	db, err := sql.Open("mysql", m.connectionString)
	if err != nil {
		return fmt.Errorf("failed to open MariaDB database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping MariaDB database: %w", err)
	}

	m.db = db
	return nil
}

func (m *MariaDB) Migrate() error {
	ipInfoSchema := `
	CREATE TABLE IF NOT EXISTS ip_info (
		ip VARCHAR(45) PRIMARY KEY,
		city VARCHAR(255),
		region VARCHAR(255),
		country VARCHAR(255),
		loc VARCHAR(255),
		org VARCHAR(255),
		postal VARCHAR(20),
		timezone VARCHAR(100),
		hostname VARCHAR(255),
		bogon BOOLEAN,
		last_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	);`

	httpRequestSchema := `
	CREATE TABLE IF NOT EXISTS http_requests (
		id INT AUTO_INCREMENT PRIMARY KEY,
		ip VARCHAR(45) NOT NULL,
		method VARCHAR(10) NOT NULL,
		path VARCHAR(1024) NOT NULL,
		user_agent TEXT,
		referer TEXT,
		headers TEXT,
		query_params TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		tls_version INT,
		tls_cipher_suite INT,
		tls_server_name VARCHAR(255),
		tls_negotiated_protocol VARCHAR(50),
		proto VARCHAR(20),
		content_length BIGINT,
		remote_addr VARCHAR(100),
		request_uri TEXT,
		host VARCHAR(255),
		scheme VARCHAR(10),
		content_type VARCHAR(255),
		body TEXT,
		FOREIGN KEY (ip) REFERENCES ip_info(ip)
	);`

	indexSchema := `
	CREATE INDEX IF NOT EXISTS idx_requests_ip ON http_requests(ip);
	CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON http_requests(timestamp);
	CREATE INDEX IF NOT EXISTS idx_requests_fingerprint ON http_requests(ip, path(255), query_params(255), user_agent(255));`

	if _, err := m.db.Exec(ipInfoSchema); err != nil {
		return fmt.Errorf("failed to create ip_info table: %w", err)
	}

	if _, err := m.db.Exec(httpRequestSchema); err != nil {
		return fmt.Errorf("failed to create http_requests table: %w", err)
	}

	if _, err := m.db.Exec(indexSchema); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

func (m *MariaDB) SaveHTTPRequest(req *HTTPRequest) error {
	query := `
	INSERT INTO http_requests (ip, method, path, user_agent, referer, headers, query_params, timestamp,
		tls_version, tls_cipher_suite, tls_server_name, tls_negotiated_protocol,
		proto, content_length, remote_addr, request_uri, host, scheme, content_type, body)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := m.db.Exec(query, req.IP, req.Method, req.Path, req.UserAgent, req.Referer,
		req.Headers, req.QueryParams, req.Timestamp,
		req.TLSVersion, req.TLSCipherSuite, req.TLSServerName, req.TLSNegotiatedProtocol,
		req.Proto, req.ContentLength, req.RemoteAddr, req.RequestURI, req.Host, req.Scheme,
		req.ContentType, req.Body)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	req.ID = id
	return nil
}

func (m *MariaDB) Verify() error {
	// Verify and fix ip_info table
	if err := m.verifyAndFixTable("ip_info", map[string]string{
		"ip":          "VARCHAR(45) PRIMARY KEY",
		"city":        "VARCHAR(255)",
		"region":      "VARCHAR(255)",
		"country":     "VARCHAR(255)",
		"loc":         "VARCHAR(255)",
		"org":         "VARCHAR(255)",
		"postal":      "VARCHAR(20)",
		"timezone":    "VARCHAR(100)",
		"hostname":    "VARCHAR(255)",
		"bogon":       "BOOLEAN",
		"last_update": "TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP",
	}); err != nil {
		return fmt.Errorf("ip_info table verification/fix failed: %w", err)
	}

	// Verify and fix http_requests table
	if err := m.verifyAndFixTable("http_requests", map[string]string{
		"id":                      "INT AUTO_INCREMENT PRIMARY KEY",
		"ip":                      "VARCHAR(45) NOT NULL",
		"method":                  "VARCHAR(10) NOT NULL",
		"path":                    "VARCHAR(1024) NOT NULL",
		"user_agent":              "TEXT",
		"referer":                 "TEXT",
		"headers":                 "TEXT",
		"query_params":            "TEXT",
		"timestamp":               "TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
		"tls_version":             "INT",
		"tls_cipher_suite":        "INT",
		"tls_server_name":         "VARCHAR(255)",
		"tls_negotiated_protocol": "VARCHAR(50)",
		"proto":                   "VARCHAR(20)",
		"content_length":          "BIGINT",
		"remote_addr":             "VARCHAR(100)",
		"request_uri":             "TEXT",
		"host":                    "VARCHAR(255)",
		"scheme":                  "VARCHAR(10)",
		"content_type":            "VARCHAR(255)",
		"body":                    "TEXT",
	}); err != nil {
		return fmt.Errorf("http_requests table verification/fix failed: %w", err)
	}

	// Verify and fix indexes
	expectedIndexes := map[string]string{
		"idx_requests_ip":          "CREATE INDEX IF NOT EXISTS idx_requests_ip ON http_requests(ip)",
		"idx_requests_timestamp":   "CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON http_requests(timestamp)",
		"idx_requests_fingerprint": "CREATE INDEX IF NOT EXISTS idx_requests_fingerprint ON http_requests(ip, path(255), query_params(255), user_agent(255))",
	}

	for indexName, createSQL := range expectedIndexes {
		if err := m.verifyAndFixIndex(indexName, createSQL); err != nil {
			return fmt.Errorf("index %s verification/fix failed: %w", indexName, err)
		}
	}

	return nil
}

func (m *MariaDB) verifyAndFixTable(tableName string, expectedColumns map[string]string) error {
	// Get current database name
	var dbName string
	err := m.db.QueryRow("SELECT DATABASE()").Scan(&dbName)
	if err != nil {
		return fmt.Errorf("failed to get database name: %w", err)
	}

	// Check if table exists
	var count int
	err = m.db.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = ? AND table_name = ?`, dbName, tableName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if count == 0 {
		// Table doesn't exist, create it
		return m.createTable(tableName, expectedColumns)
	}

	// Get existing table columns
	rows, err := m.db.Query(`
		SELECT column_name 
		FROM information_schema.columns 
		WHERE table_schema = ? AND table_name = ?`, dbName, tableName)
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
			if err := m.addColumn(tableName, columnName, columnDef); err != nil {
				return fmt.Errorf("failed to add column %s to table %s: %w", columnName, tableName, err)
			}
		}
	}

	return nil
}

func (m *MariaDB) verifyAndFixIndex(indexName, createSQL string) error {
	// Get current database name
	var dbName string
	err := m.db.QueryRow("SELECT DATABASE()").Scan(&dbName)
	if err != nil {
		return fmt.Errorf("failed to get database name: %w", err)
	}

	var count int
	err = m.db.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.statistics 
		WHERE table_schema = ? AND index_name = ?`, dbName, indexName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}

	if count == 0 {
		// Index doesn't exist, create it
		if _, err := m.db.Exec(createSQL); err != nil {
			return fmt.Errorf("failed to create index %s: %w", indexName, err)
		}
	}

	return nil
}

func (m *MariaDB) createTable(tableName string, columns map[string]string) error {
	var columnDefs []string
	for columnName, columnDef := range columns {
		columnDefs = append(columnDefs, columnName+" "+columnDef)
	}

	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(columnDefs, ", "))
	if _, err := m.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}

	return nil
}

func (m *MariaDB) addColumn(tableName, columnName, columnDef string) error {
	// Remove constraints that aren't supported in ALTER TABLE ADD COLUMN
	cleanDef := columnDef
	if strings.Contains(strings.ToUpper(cleanDef), "PRIMARY KEY") ||
		strings.Contains(strings.ToUpper(cleanDef), "AUTO_INCREMENT") {
		// Can't add primary key or auto_increment via ALTER TABLE, skip this column
		return nil
	}

	alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, cleanDef)
	if _, err := m.db.Exec(alterSQL); err != nil {
		return fmt.Errorf("failed to add column: %w", err)
	}

	return nil
}

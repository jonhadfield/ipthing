package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteDB struct {
	BaseDB
	dbPath string
}

func NewSQLiteDB(dbPath string) *SQLiteDB {
	return &SQLiteDB{
		dbPath: dbPath,
	}
}

func (s *SQLiteDB) Connect() error {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", s.dbPath))
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}

	s.db = db
	return nil
}

func (s *SQLiteDB) Migrate() error {
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
		id INTEGER PRIMARY KEY AUTOINCREMENT,
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
		content_length INTEGER,
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

	if _, err := s.db.Exec(ipInfoSchema); err != nil {
		return fmt.Errorf("failed to create ip_info table: %w", err)
	}

	if _, err := s.db.Exec(httpRequestSchema); err != nil {
		return fmt.Errorf("failed to create http_requests table: %w", err)
	}

	if _, err := s.db.Exec(indexSchema); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

func (s *SQLiteDB) SaveIPInfo(info *IPInfo) error {
	query := `
	INSERT OR REPLACE INTO ip_info (ip, city, region, country, loc, org, postal, timezone, hostname, bogon, last_update)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query, info.IP, info.City, info.Region, info.Country, info.Loc, info.Org, info.Postal, info.Timezone, info.Hostname, info.Bogon, info.LastUpdate)
	return err
}

func (s *SQLiteDB) GetIPInfo(ip string) (*IPInfo, error) {
	query := `
	SELECT ip, city, region, country, loc, org, postal, timezone, hostname, bogon, last_update
	FROM ip_info WHERE ip = ?`

	var info IPInfo
	err := s.db.QueryRow(query, ip).Scan(&info.IP, &info.City, &info.Region, &info.Country, &info.Loc, &info.Org, &info.Postal, &info.Timezone, &info.Hostname, &info.Bogon, &info.LastUpdate)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func (s *SQLiteDB) SaveHTTPRequest(req *HTTPRequest) error {
	query := `
	INSERT INTO http_requests (ip, method, path, user_agent, referer, headers, query_params, timestamp,
		tls_version, tls_cipher_suite, tls_server_name, tls_negotiated_protocol,
		proto, content_length, remote_addr, request_uri, host, scheme, content_type, body)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.db.Exec(query, req.IP, req.Method, req.Path, req.UserAgent, req.Referer,
		req.Headers, req.QueryParams, req.Timestamp,
		req.TLSVersion, req.TLSCipherSuite, req.TLSServerName, req.TLSNegotiatedProtocol,
		req.Proto, req.ContentLength, req.RemoteAddr, req.RequestURI, req.Host, req.Scheme,
		req.ContentType, req.Body)
	if err != nil {
		return err
	}

	req.ID, _ = result.LastInsertId()
	return nil
}

func (s *SQLiteDB) IsDuplicateRequest(fingerprint *RequestFingerprint, timeWindow time.Duration) (bool, error) {
	query := `
	SELECT COUNT(*) FROM http_requests
	WHERE ip = ? AND path = ? AND query_params = ? AND user_agent = ?
	AND timestamp > ?`

	cutoffTime := time.Now().Add(-timeWindow)
	var count int
	err := s.db.QueryRow(query, fingerprint.IP, fingerprint.Path, fingerprint.QueryParams, fingerprint.UserAgent, cutoffTime).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (s *SQLiteDB) Verify() error {
	// Verify and fix ip_info table
	if err := s.verifyAndFixTable("ip_info", map[string]string{
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
	if err := s.verifyAndFixTable("http_requests", map[string]string{
		"id":                      "INTEGER PRIMARY KEY AUTOINCREMENT",
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
		"content_length":          "INTEGER",
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
		if err := s.verifyAndFixIndex(indexName, createSQL); err != nil {
			return fmt.Errorf("index %s verification/fix failed: %w", indexName, err)
		}
	}

	return nil
}

func (s *SQLiteDB) verifyAndFixTable(tableName string, expectedColumns map[string]string) error {
	// Check if table exists
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if count == 0 {
		// Table doesn't exist, create it
		return s.createTable(tableName, expectedColumns)
	}

	// Get existing table columns
	rows, err := s.db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return fmt.Errorf("failed to get table info: %w", err)
	}
	defer rows.Close()

	actualColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue interface{}
		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		actualColumns[name] = true
	}

	// Add missing columns
	for columnName, columnDef := range expectedColumns {
		if !actualColumns[columnName] {
			if err := s.addColumn(tableName, columnName, columnDef); err != nil {
				return fmt.Errorf("failed to add column %s to table %s: %w", columnName, tableName, err)
			}
		}
	}

	return nil
}

func (s *SQLiteDB) verifyAndFixIndex(indexName, createSQL string) error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", indexName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}

	if count == 0 {
		// Index doesn't exist, create it
		if _, err := s.db.Exec(createSQL); err != nil {
			return fmt.Errorf("failed to create index %s: %w", indexName, err)
		}
	}

	return nil
}

func (s *SQLiteDB) createTable(tableName string, columns map[string]string) error {
	var columnDefs []string
	for columnName, columnDef := range columns {
		columnDefs = append(columnDefs, columnName+" "+columnDef)
	}

	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(columnDefs, ", "))
	if _, err := s.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}

	return nil
}

func (s *SQLiteDB) addColumn(tableName, columnName, columnDef string) error {
	// Remove constraints that aren't supported in ALTER TABLE ADD COLUMN
	cleanDef := columnDef
	if strings.Contains(strings.ToUpper(cleanDef), "PRIMARY KEY") {
		// Can't add primary key via ALTER TABLE, skip this column
		return nil
	}

	alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, cleanDef)
	if _, err := s.db.Exec(alterSQL); err != nil {
		return fmt.Errorf("failed to add column: %w", err)
	}

	return nil
}

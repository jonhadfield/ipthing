package main

import (
	"database/sql"
	"fmt"
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
		has_cookies INTEGER,
		claimed_xff TEXT,
		cf_connecting_ip TEXT,
		duration_ms INTEGER,
		response_format TEXT,
		status_code INTEGER,
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

	for _, stmt := range []string{
		`ALTER TABLE http_requests ADD COLUMN has_cookies INTEGER`,
		`ALTER TABLE http_requests ADD COLUMN claimed_xff TEXT`,
		`ALTER TABLE http_requests ADD COLUMN cf_connecting_ip TEXT`,
		`ALTER TABLE http_requests ADD COLUMN duration_ms INTEGER`,
		`ALTER TABLE http_requests ADD COLUMN response_format TEXT`,
		`ALTER TABLE http_requests ADD COLUMN status_code INTEGER`,
	} {
		// SQLite errors if the column already exists; ignore that.
		_, _ = s.db.Exec(stmt)
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
		proto, content_length, remote_addr, request_uri, host, scheme, content_type, body,
		has_cookies, claimed_xff, cf_connecting_ip, duration_ms, response_format, status_code)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.db.Exec(query, req.IP, req.Method, req.Path, req.UserAgent, req.Referer,
		req.Headers, req.QueryParams, req.Timestamp,
		req.TLSVersion, req.TLSCipherSuite, req.TLSServerName, req.TLSNegotiatedProtocol,
		req.Proto, req.ContentLength, req.RemoteAddr, req.RequestURI, req.Host, req.Scheme,
		req.ContentType, req.Body,
		req.HasCookies, req.ClaimedXFF, req.CfConnectingIP, req.DurationMs, req.ResponseFormat, req.StatusCode)
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

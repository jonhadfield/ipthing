package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/lib/pq"
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
		has_cookies BOOLEAN,
		claimed_xff TEXT,
		cf_connecting_ip TEXT,
		duration_ms BIGINT,
		response_format TEXT,
		status_code INTEGER,
		FOREIGN KEY (ip) REFERENCES ip_info(ip)
	);`

	indexSchema := `
	CREATE INDEX IF NOT EXISTS idx_requests_ip ON http_requests(ip);
	CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON http_requests(timestamp);
	CREATE INDEX IF NOT EXISTS idx_requests_fingerprint ON http_requests(ip, path, query_params, user_agent);`

	if _, err := p.db.Exec(ipInfoSchema); err != nil {
		if err := p.handleSchemaDDLError(err, "ip_info"); err != nil {
			return err
		}
	}

	if _, err := p.db.Exec(httpRequestSchema); err != nil {
		if err := p.handleSchemaDDLError(err, "http_requests"); err != nil {
			return err
		}
	}

	if _, err := p.db.Exec(indexSchema); err != nil {
		if !isPostgresInsufficientPrivilege(err) {
			return fmt.Errorf("failed to create indexes: %w", err)
		}
		log.Printf("skipping index migration (insufficient privilege): %v", err)
	}

	// Add columns if upgrading an older schema that CREATE TABLE IF NOT EXISTS won't alter.
	for _, stmt := range []string{
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS tls_version INTEGER`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS tls_cipher_suite INTEGER`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS tls_server_name TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS tls_negotiated_protocol TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS proto TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS content_length BIGINT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS remote_addr TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS request_uri TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS host TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS scheme TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS content_type TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS body TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS has_cookies BOOLEAN`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS claimed_xff TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS cf_connecting_ip TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS duration_ms BIGINT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS response_format TEXT`,
		`ALTER TABLE http_requests ADD COLUMN IF NOT EXISTS status_code INTEGER`,
	} {
		if _, err := p.db.Exec(stmt); err != nil {
			// App roles may lack ownership of tables created by neondb_owner; DDL is run separately.
			if isPostgresInsufficientPrivilege(err) {
				continue
			}
			return fmt.Errorf("failed to migrate http_requests columns: %w", err)
		}
	}

	return nil
}

// handleSchemaDDLError tolerates missing CREATE rights when the table already exists
// (typical for least-privilege app roles on Neon).
func (p *PostgresDB) handleSchemaDDLError(err error, table string) error {
	if !isPostgresInsufficientPrivilege(err) {
		return fmt.Errorf("failed to create %s table: %w", table, err)
	}
	var exists bool
	qerr := p.db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
		table,
	).Scan(&exists)
	if qerr != nil {
		return fmt.Errorf("failed to create %s table: %w (and could not verify existing table: %v)", table, err, qerr)
	}
	if !exists {
		return fmt.Errorf("failed to create %s table: %w", table, err)
	}
	log.Printf("skipping %s DDL (insufficient privilege; table already exists)", table)
	return nil
}

func isPostgresInsufficientPrivilege(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "42501" {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "must be owner") || strings.Contains(msg, "permission denied")
}

func (p *PostgresDB) SaveHTTPRequest(req *HTTPRequest) error {
	query := `
	INSERT INTO http_requests (ip, method, path, user_agent, referer, headers, query_params, timestamp,
		tls_version, tls_cipher_suite, tls_server_name, tls_negotiated_protocol,
		proto, content_length, remote_addr, request_uri, host, scheme, content_type, body,
		has_cookies, claimed_xff, cf_connecting_ip, duration_ms, response_format, status_code)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
		$21, $22, $23, $24, $25, $26)
	RETURNING id`

	return p.db.QueryRow(query,
		req.IP, req.Method, req.Path, req.UserAgent, req.Referer, req.Headers, req.QueryParams, req.Timestamp,
		req.TLSVersion, req.TLSCipherSuite, req.TLSServerName, req.TLSNegotiatedProtocol,
		req.Proto, req.ContentLength, req.RemoteAddr, req.RequestURI, req.Host, req.Scheme,
		req.ContentType, req.Body,
		req.HasCookies, req.ClaimedXFF, req.CfConnectingIP, req.DurationMs, req.ResponseFormat, req.StatusCode,
	).Scan(&req.ID)
}

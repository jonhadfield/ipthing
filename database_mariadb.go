package main

import (
	"database/sql"
	"fmt"

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
	INSERT INTO http_requests (ip, method, path, user_agent, referer, headers, query_params, timestamp)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := m.db.Exec(query, req.IP, req.Method, req.Path, req.UserAgent, req.Referer, req.Headers, req.QueryParams, req.Timestamp)
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

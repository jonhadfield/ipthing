package main

import (
	"database/sql"
	"time"
)

type Database interface {
	Connect() error
	Close() error
	Migrate() error
	Verify() error

	SaveIPInfo(info *IPInfo) error
	GetIPInfo(ip string) (*IPInfo, error)

	SaveHTTPRequest(req *HTTPRequest) error
	IsDuplicateRequest(fingerprint *RequestFingerprint, timeWindow time.Duration) (bool, error)
}

type BaseDB struct {
	db *sql.DB
}

func (b *BaseDB) Close() error {
	if b.db != nil {
		return b.db.Close()
	}
	return nil
}

func (b *BaseDB) SaveIPInfo(info *IPInfo) error {
	query := `
	INSERT INTO ip_info (ip, city, region, country, loc, org, postal, timezone, hostname, bogon, last_update)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	ON CONFLICT (ip) DO UPDATE SET
		city = EXCLUDED.city,
		region = EXCLUDED.region,
		country = EXCLUDED.country,
		loc = EXCLUDED.loc,
		org = EXCLUDED.org,
		postal = EXCLUDED.postal,
		timezone = EXCLUDED.timezone,
		hostname = EXCLUDED.hostname,
		bogon = EXCLUDED.bogon,
		last_update = EXCLUDED.last_update`

	_, err := b.db.Exec(query, info.IP, info.City, info.Region, info.Country, info.Loc, info.Org, info.Postal, info.Timezone, info.Hostname, info.Bogon, info.LastUpdate)
	return err
}

func (b *BaseDB) GetIPInfo(ip string) (*IPInfo, error) {
	query := `
	SELECT ip, city, region, country, loc, org, postal, timezone, hostname, bogon, last_update
	FROM ip_info WHERE ip = $1`

	var info IPInfo
	err := b.db.QueryRow(query, ip).Scan(&info.IP, &info.City, &info.Region, &info.Country, &info.Loc, &info.Org, &info.Postal, &info.Timezone, &info.Hostname, &info.Bogon, &info.LastUpdate)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func (b *BaseDB) SaveHTTPRequest(req *HTTPRequest) error {
	query := `
	INSERT INTO http_requests (ip, method, path, user_agent, referer, headers, query_params, timestamp,
		tls_version, tls_cipher_suite, tls_server_name, tls_negotiated_protocol,
		proto, content_length, remote_addr, request_uri, host, scheme, content_type, body)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`

	result, err := b.db.Exec(query, req.IP, req.Method, req.Path, req.UserAgent, req.Referer,
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

func (b *BaseDB) IsDuplicateRequest(fingerprint *RequestFingerprint, timeWindow time.Duration) (bool, error) {
	query := `
	SELECT COUNT(*) FROM http_requests
	WHERE ip = $1 AND path = $2 AND query_params = $3 AND user_agent = $4
	AND timestamp > $5`

	cutoffTime := time.Now().Add(-timeWindow)
	var count int
	err := b.db.QueryRow(query, fingerprint.IP, fingerprint.Path, fingerprint.QueryParams, fingerprint.UserAgent, cutoffTime).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

package main

import (
	"database/sql"
	"time"
)

// NoOpDB is a no-operation database that implements the Database interface
// but doesn't actually store or retrieve any data
type NoOpDB struct{}

func (n *NoOpDB) Connect() error {
	// No connection needed
	return nil
}

func (n *NoOpDB) Close() error {
	// Nothing to close
	return nil
}

func (n *NoOpDB) Migrate() error {
	// No migrations needed
	return nil
}

func (n *NoOpDB) SaveIPInfo(info *IPInfo) error {
	// Silently ignore
	return nil
}

func (n *NoOpDB) GetIPInfo(ip string) (*IPInfo, error) {
	// Always return not found
	return nil, sql.ErrNoRows
}

func (n *NoOpDB) SaveHTTPRequest(req *HTTPRequest) error {
	// Silently ignore
	return nil
}

func (n *NoOpDB) IsDuplicateRequest(fingerprint *RequestFingerprint, timeWindow time.Duration) (bool, error) {
	// Never duplicate since we're not storing anything
	return false, nil
}

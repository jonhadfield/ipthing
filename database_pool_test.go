package main

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestConfigureRemoteDBPool(t *testing.T) {
	db, err := sql.Open("sqlite", "file:pooltest?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	configureRemoteDBPool(db)

	// database/sql exposes MaxOpen via Stats; idle limits have no getters, so
	// pin the package constants here so a silent change is caught.
	stats := db.Stats()
	assert.Equal(t, remoteDBMaxOpenConns, stats.MaxOpenConnections)
	assert.Equal(t, 8, remoteDBMaxOpenConns)
	assert.Equal(t, 1, remoteDBMaxIdleConns)
	assert.Equal(t, 2*time.Minute, remoteDBConnMaxIdle)
}

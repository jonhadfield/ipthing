package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestSQLite(t *testing.T) *SQLiteDB {
	t.Helper()
	db := NewSQLiteDB(filepath.Join(t.TempDir(), "ipthing.db"))
	require.NoError(t, db.Connect())
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate())
	return db
}

func TestSQLite_RoundTrip(t *testing.T) {
	db := openTestSQLite(t)

	now := time.Now().UTC().Truncate(time.Second)
	info := &IPInfo{
		IP:         "203.0.113.50",
		City:       "Dallas",
		Region:     "Texas",
		Country:    "US",
		Loc:        "32.78,-96.80",
		Org:        "AS65536 Example",
		Postal:     "75201",
		Timezone:   "America/Chicago",
		Hostname:   "example.invalid",
		Bogon:      false,
		LastUpdate: now,
	}
	require.NoError(t, db.SaveIPInfo(info))

	got, err := db.GetIPInfo(info.IP)
	require.NoError(t, err)
	assert.Equal(t, info.IP, got.IP)
	assert.Equal(t, info.City, got.City)
	assert.Equal(t, info.Region, got.Region)
	assert.Equal(t, info.Country, got.Country)
	assert.Equal(t, info.Loc, got.Loc)
	assert.Equal(t, info.Org, got.Org)
	assert.Equal(t, info.Postal, got.Postal)
	assert.Equal(t, info.Timezone, got.Timezone)
	assert.Equal(t, info.Hostname, got.Hostname)
	assert.Equal(t, info.Bogon, got.Bogon)
	assert.WithinDuration(t, now, got.LastUpdate, time.Second)

	_, err = db.GetIPInfo("198.51.100.1")
	assert.ErrorIs(t, err, sql.ErrNoRows)

	req := &HTTPRequest{
		IP:          info.IP,
		Method:      "GET",
		Path:        "/",
		UserAgent:   "curl/8.0.0",
		Headers:     `{"Accept":["*/*"]}`,
		QueryParams: `{"q":["1"]}`,
		Timestamp:   now,
		Proto:       "HTTP/2.0",
		RemoteAddr:  "203.0.113.50:443",
		Host:        "ipthing.net",
		Scheme:      "https",
		IPFamily:    "ipv4",
		HasCookies:  true,
		CookieNames: "session",
		JA4:         "t13d",
		StatusCode:  200,
	}
	require.NoError(t, db.SaveHTTPRequest(req))
	assert.Greater(t, req.ID, int64(0))

	fp := &RequestFingerprint{
		IP:          req.IP,
		Path:        req.Path,
		QueryParams: req.QueryParams,
		UserAgent:   req.UserAgent,
	}
	dup, err := db.IsDuplicateRequest(fp, 5*time.Minute)
	require.NoError(t, err)
	assert.True(t, dup)

	other, err := db.IsDuplicateRequest(&RequestFingerprint{
		IP:          req.IP,
		Path:        "/other",
		QueryParams: req.QueryParams,
		UserAgent:   req.UserAgent,
	}, 5*time.Minute)
	require.NoError(t, err)
	assert.False(t, other)
}

func TestSQLite_MigrateIsIdempotent(t *testing.T) {
	db := openTestSQLite(t)
	require.NoError(t, db.Migrate())
}

func TestSQLite_ReplaceIPInfo(t *testing.T) {
	db := openTestSQLite(t)
	require.NoError(t, db.SaveIPInfo(&IPInfo{IP: "203.0.113.1", City: "Old", LastUpdate: time.Now()}))
	require.NoError(t, db.SaveIPInfo(&IPInfo{IP: "203.0.113.1", City: "New", LastUpdate: time.Now()}))

	got, err := db.GetIPInfo("203.0.113.1")
	require.NoError(t, err)
	assert.Equal(t, "New", got.City)
}

func TestNewDatabase(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		db, defined, err := NewDatabase(nil)
		require.NoError(t, err)
		assert.False(t, defined)
		_, ok := db.(*NoOpDB)
		assert.True(t, ok)
	})

	t.Run("empty config", func(t *testing.T) {
		db, defined, err := NewDatabase(&Config{})
		require.NoError(t, err)
		assert.False(t, defined)
		_, ok := db.(*NoOpDB)
		assert.True(t, ok)
	})

	t.Run("sqlite from path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "from-path.db")
		db, defined, err := NewDatabase(&Config{DatabasePath: path})
		require.NoError(t, err)
		assert.True(t, defined)
		sqlite, ok := db.(*SQLiteDB)
		require.True(t, ok)
		assert.Equal(t, path, sqlite.dbPath)
	})

	t.Run("sqlite explicit type default path", func(t *testing.T) {
		db, defined, err := NewDatabase(&Config{DatabaseType: "sqlite"})
		require.NoError(t, err)
		assert.True(t, defined)
		sqlite, ok := db.(*SQLiteDB)
		require.True(t, ok)
		assert.Equal(t, "ipthing.db", sqlite.dbPath)
	})

	t.Run("postgres from connection string", func(t *testing.T) {
		db, defined, err := NewDatabase(&Config{
			DatabaseConnectionString: "postgres://user:pass@localhost/ipthing",
		})
		require.NoError(t, err)
		assert.True(t, defined)
		_, ok := db.(*PostgresDB)
		assert.True(t, ok)
	})

	t.Run("postgres missing connection string", func(t *testing.T) {
		_, _, err := NewDatabase(&Config{DatabaseType: "postgres"})
		require.Error(t, err)
	})

	t.Run("mariadb missing connection string", func(t *testing.T) {
		_, _, err := NewDatabase(&Config{DatabaseType: "mariadb"})
		require.Error(t, err)
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, _, err := NewDatabase(&Config{DatabaseType: "oracle"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported database type")
	})
}

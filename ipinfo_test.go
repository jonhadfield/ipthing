package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldUpdateIPInfo(t *testing.T) {
	require.Equal(t, 24*time.Hour, ipInfoCacheTTL)

	tests := []struct {
		name         string
		lastUpdate   time.Time
		shouldUpdate bool
	}{
		{
			name:         "within TTL",
			lastUpdate:   time.Now().Add(-(ipInfoCacheTTL - time.Hour)),
			shouldUpdate: false,
		},
		{
			name:         "just inside TTL",
			lastUpdate:   time.Now().Add(-(ipInfoCacheTTL - time.Minute)),
			shouldUpdate: false,
		},
		{
			name:         "past TTL",
			lastUpdate:   time.Now().Add(-(ipInfoCacheTTL + time.Hour)),
			shouldUpdate: true,
		},
		{
			name:         "zero time - never updated",
			lastUpdate:   time.Time{},
			shouldUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.shouldUpdate, shouldUpdateIPInfo(tt.lastUpdate))
		})
	}
}

func TestGetOrFetchIPInfo_UsesFreshCache(t *testing.T) {
	db := &memIPInfoDB{info: &IPInfo{
		IP:         "203.0.113.1",
		City:       "Cached",
		LastUpdate: time.Now().Add(-time.Hour),
	}}

	// No HTTP server: a fetch would fail. Fresh cache must short-circuit.
	info, err := getOrFetchIPInfo(t.Context(), db, "203.0.113.1")
	require.NoError(t, err)
	assert.Equal(t, "Cached", info.City)
	assert.Equal(t, 0, db.saves)
}

func TestGetOrFetchIPInfo_RefreshesStaleCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ip":   "203.0.113.2",
			"city": "Fresh",
		})
	}))
	t.Cleanup(srv.Close)

	prevURL, prevClient, prevSem := ipInfoURLFormat, ipInfoHTTPClient, ipInfoFetchSem
	ipInfoURLFormat = srv.URL + "/%s"
	ipInfoHTTPClient = srv.Client()
	ipInfoFetchSem = make(chan struct{}, maxConcurrentIPInfoFetches)
	t.Cleanup(func() {
		ipInfoURLFormat = prevURL
		ipInfoHTTPClient = prevClient
		ipInfoFetchSem = prevSem
	})

	db := &memIPInfoDB{info: &IPInfo{
		IP:         "203.0.113.2",
		City:       "Stale",
		LastUpdate: time.Now().Add(-(ipInfoCacheTTL + time.Hour)),
	}}

	info, err := getOrFetchIPInfo(t.Context(), db, "203.0.113.2")
	require.NoError(t, err)
	assert.Equal(t, "Fresh", info.City)
	assert.Equal(t, 1, db.saves)
	assert.False(t, shouldUpdateIPInfo(info.LastUpdate))
}

func TestFetchIPInfo_CapsConcurrency(t *testing.T) {
	const limit = 2
	var inFlight, maxInFlight atomic.Int32
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inFlight.Add(1)
		for {
			prev := maxInFlight.Load()
			if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		_ = json.NewEncoder(w).Encode(map[string]string{"ip": "203.0.113.9", "city": "X"})
	}))
	t.Cleanup(srv.Close)

	prevURL, prevClient, prevSem := ipInfoURLFormat, ipInfoHTTPClient, ipInfoFetchSem
	ipInfoURLFormat = srv.URL + "/%s"
	ipInfoHTTPClient = srv.Client()
	ipInfoFetchSem = make(chan struct{}, limit)
	t.Cleanup(func() {
		ipInfoURLFormat = prevURL
		ipInfoHTTPClient = prevClient
		ipInfoFetchSem = prevSem
	})

	const callers = 6
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			_, err := fetchIPInfo(t.Context(), "203.0.113.9")
			errs <- err
		})
	}

	// Wait until the semaphore is saturated, then free the handlers.
	require.Eventually(t, func() bool {
		return maxInFlight.Load() == limit && inFlight.Load() == limit
	}, time.Second, 5*time.Millisecond)

	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(limit), maxInFlight.Load())
	assert.Equal(t, maxConcurrentIPInfoFetches, 8)
}

// memIPInfoDB is an in-memory stub for cache TTL tests.
type memIPInfoDB struct {
	NoOpDB
	info  *IPInfo
	saves int
}

func (m *memIPInfoDB) GetIPInfo(ip string) (*IPInfo, error) {
	if m.info == nil || m.info.IP != ip {
		return (&NoOpDB{}).GetIPInfo(ip)
	}
	cp := *m.info
	return &cp, nil
}

func (m *memIPInfoDB) SaveIPInfo(info *IPInfo) error {
	m.saves++
	cp := *info
	m.info = &cp
	return nil
}

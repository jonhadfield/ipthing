package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingDB stalls SaveHTTPRequest so the store queue can fill.
type blockingDB struct {
	NoOpDB
	entered chan struct{}
	release chan struct{}
	saved   atomic.Int32
}

func (b *blockingDB) IsDuplicateRequest(_ *RequestFingerprint, _ time.Duration) (bool, error) {
	return false, nil
}

func (b *blockingDB) GetIPInfo(string) (*IPInfo, error) {
	return &IPInfo{IP: "203.0.113.1", LastUpdate: time.Now()}, nil
}

func (b *blockingDB) SaveHTTPRequest(*HTTPRequest) error {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	b.saved.Add(1)
	return nil
}

func TestQueueStore_DropsWhenFull(t *testing.T) {
	db := &blockingDB{
		entered: make(chan struct{}, storeWorkers),
		release: make(chan struct{}),
	}
	e := echo.New()
	rp := NewRequestProcessor(db, true, e.Logger)
	t.Cleanup(rp.Close)

	// Saturate workers, then fill the buffered queue.
	for range storeWorkers {
		rp.QueueStore(&HTTPRequest{IP: "203.0.113.1", Path: "/"})
	}
	require.Eventually(t, func() bool {
		return len(db.entered) == storeWorkers
	}, time.Second, 5*time.Millisecond)

	for range storeQueueSize {
		rp.QueueStore(&HTTPRequest{IP: "203.0.113.1", Path: "/"})
	}

	// Next enqueue should drop rather than block the caller.
	done := make(chan struct{})
	go func() {
		rp.QueueStore(&HTTPRequest{IP: "203.0.113.1", Path: "/dropped"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("QueueStore blocked when queue was full")
	}

	close(db.release)
	rp.Close()
	// Workers (storeWorkers) + queued (storeQueueSize) should have been accepted; drop not saved.
	assert.Equal(t, int32(storeWorkers+storeQueueSize), db.saved.Load())
}

func TestQueueStore_CloseIsIdempotent(t *testing.T) {
	rp := NewRequestProcessor(&NoOpDB{}, true, nil)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { rp.Close() })
	}
	wg.Wait()
}

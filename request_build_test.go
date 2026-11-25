package main

import (
	"crypto/tls"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHTTPRequest_TLSAndJAFingerprints(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tls", nil)
	req.RemoteAddr = "203.0.113.50:443"
	req.Host = "ipthing.net"
	req.TLS = &tls.ConnectionState{
		Version:            tls.VersionTLS13,
		CipherSuite:        tls.TLS_AES_128_GCM_SHA256,
		ServerName:         "ipthing.net",
		NegotiatedProtocol: "h2",
		DidResume:          true,
		CurveID:            tls.X25519,
	}

	store := newTLSHelloStore()
	store.byAddr.Store(req.RemoteAddr, clientHelloFP{JA3: "ja3-digest", JA4: "t13d"})

	rp := NewRequestProcessor(&NoOpDB{}, false, nil)
	rp.SetTLSHelloStore(store)

	httpReq := rp.buildHTTPRequest(t.Context(), req, "203.0.113.50")
	assert.Equal(t, "https", httpReq.Scheme)
	assert.Equal(t, uint16(tls.VersionTLS13), httpReq.TLSVersion)
	assert.Equal(t, uint16(tls.TLS_AES_128_GCM_SHA256), httpReq.TLSCipherSuite)
	assert.Equal(t, "ipthing.net", httpReq.TLSServerName)
	assert.Equal(t, "h2", httpReq.TLSNegotiatedProtocol)
	assert.True(t, httpReq.TLSDidResume)
	assert.Equal(t, tls.X25519.String(), httpReq.TLSCurve)
	assert.Equal(t, "ja3-digest", httpReq.JA3)
	assert.Equal(t, "t13d", httpReq.JA4)
}

func TestBuildHTTPRequest_TruncatesLongPaths(t *testing.T) {
	long := "/" + strings.Repeat("a", maxLoggedPathLen+50)
	req := httptest.NewRequest(http.MethodGet, long+"?x=1", nil)
	req.RequestURI = long + "?x=1"
	req.RemoteAddr = "203.0.113.50:1"

	rp := NewRequestProcessor(&NoOpDB{}, false, nil)
	httpReq := rp.buildHTTPRequest(t.Context(), req, "203.0.113.50")

	assert.Len(t, httpReq.Path, maxLoggedPathLen)
	assert.Len(t, httpReq.RequestURI, maxLoggedPathLen)
	assert.Equal(t, "http", httpReq.Scheme)
}

func TestStoreRequestAsync_PersistsNewIP(t *testing.T) {
	db := &asyncStoreDB{}
	rp := NewRequestProcessor(db, true, echo.New().Logger)
	t.Cleanup(rp.Close)

	req := &HTTPRequest{
		IP:          "203.0.113.77",
		Method:      "GET",
		Path:        "/async",
		UserAgent:   "test",
		QueryParams: "{}",
		Timestamp:   time.Now(),
	}
	rp.storeRequestAsync(req)

	assert.Equal(t, int32(1), db.savedIP.Load())
	assert.Equal(t, int32(1), db.savedReq.Load())
}

func TestStoreRequestAsync_SkipsDuplicate(t *testing.T) {
	db := &asyncStoreDB{duplicate: true}
	e := echo.New()
	rp := NewRequestProcessor(db, true, e.Logger)
	t.Cleanup(rp.Close)

	rp.storeRequestAsync(&HTTPRequest{
		IP:          "203.0.113.77",
		Path:        "/",
		QueryParams: "{}",
		UserAgent:   "test",
	})

	assert.Equal(t, int32(0), db.savedIP.Load())
	assert.Equal(t, int32(0), db.savedReq.Load())
}

func TestStoreRequestAsync_DuplicateCheckError(t *testing.T) {
	db := &asyncStoreDB{dupErr: errors.New("dup boom")}
	rp := NewRequestProcessor(db, true, echo.New().Logger)
	t.Cleanup(rp.Close)

	rp.storeRequestAsync(&HTTPRequest{IP: "203.0.113.1", Path: "/", QueryParams: "{}"})
	assert.Equal(t, int32(0), db.savedReq.Load())
}

func TestStoreRequestAsync_SaveIPInfoError(t *testing.T) {
	db := &asyncStoreDB{saveIPErr: errors.New("ip boom")}
	rp := NewRequestProcessor(db, true, echo.New().Logger)
	t.Cleanup(rp.Close)

	rp.storeRequestAsync(&HTTPRequest{IP: "203.0.113.1", Path: "/", QueryParams: "{}"})
	assert.Equal(t, int32(0), db.savedReq.Load())
}

func TestStoreRequestAsync_SaveHTTPRequestError(t *testing.T) {
	db := &asyncStoreDB{
		existing:   &IPInfo{IP: "203.0.113.1"},
		saveReqErr: errors.New("req boom"),
	}
	rp := NewRequestProcessor(db, true, echo.New().Logger)
	t.Cleanup(rp.Close)

	rp.storeRequestAsync(&HTTPRequest{IP: "203.0.113.1", Path: "/", QueryParams: "{}"})
	assert.Equal(t, int32(0), db.savedIP.Load())
	assert.Equal(t, int32(1), db.saveReqAttempts.Load())
}

func TestNoOpDB_SaveHTTPRequest(t *testing.T) {
	db := &NoOpDB{}
	require.NoError(t, db.Connect())
	require.NoError(t, db.Migrate())
	require.NoError(t, db.Close())
	require.NoError(t, db.SaveHTTPRequest(&HTTPRequest{IP: "203.0.113.1"}))
}

type asyncStoreDB struct {
	NoOpDB
	duplicate       bool
	dupErr          error
	saveIPErr       error
	saveReqErr      error
	existing        *IPInfo
	savedIP         atomic.Int32
	savedReq        atomic.Int32
	saveReqAttempts atomic.Int32
}

func (d *asyncStoreDB) IsDuplicateRequest(*RequestFingerprint, time.Duration) (bool, error) {
	if d.dupErr != nil {
		return false, d.dupErr
	}
	return d.duplicate, nil
}

func (d *asyncStoreDB) GetIPInfo(string) (*IPInfo, error) {
	if d.existing != nil {
		return d.existing, nil
	}
	return nil, sql.ErrNoRows
}

func (d *asyncStoreDB) SaveIPInfo(*IPInfo) error {
	if d.saveIPErr != nil {
		return d.saveIPErr
	}
	d.savedIP.Add(1)
	return nil
}

func (d *asyncStoreDB) SaveHTTPRequest(*HTTPRequest) error {
	d.saveReqAttempts.Add(1)
	if d.saveReqErr != nil {
		return d.saveReqErr
	}
	d.savedReq.Add(1)
	return nil
}

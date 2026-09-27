package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPFamily(t *testing.T) {
	assert.Equal(t, "ipv4", ipFamily("203.0.113.50"))
	assert.Equal(t, "ipv6", ipFamily("2001:db8::1"))
	assert.Equal(t, "", ipFamily("not-an-ip"))
}

func TestCookieNames(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "session=secret; theme=dark")
	names := cookieNames(h)
	assert.Equal(t, []string{"session", "theme"}, names)
	assert.Nil(t, cookieNames(http.Header{}))
}

func TestJA3FromClientHelloDeterministic(t *testing.T) {
	hello := &tls.ClientHelloInfo{
		CipherSuites:      []uint16{tls.TLS_AES_128_GCM_SHA256, 0x0a0a, tls.TLS_AES_256_GCM_SHA384},
		SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
		Extensions:        []uint16{0, 23, 0x1a1a, 16},
		SupportedCurves:   []tls.CurveID{tls.X25519, tls.CurveP256},
		SupportedPoints:   []uint8{0},
	}
	a := ja3FromClientHello(hello)
	b := ja3FromClientHello(hello)
	assert.Equal(t, a, b)
	assert.Len(t, a, 32)
}

func TestTLSHelloStoreRememberAndLookup(t *testing.T) {
	store := newTLSHelloStore()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	hello := &tls.ClientHelloInfo{
		CipherSuites:      []uint16{tls.TLS_AES_128_GCM_SHA256},
		SupportedVersions: []uint16{tls.VersionTLS13},
		Extensions:        []uint16{0, 16},
		SupportedProtos:   []string{"h2"},
		Conn:              server,
	}
	store.Remember(hello)

	fp, ok := store.Lookup(server.RemoteAddr().String())
	require.True(t, ok)
	assert.NotEmpty(t, fp.JA3)
	assert.NotEmpty(t, fp.JA4)
	assert.True(t, len(fp.JA4) > 10)
}

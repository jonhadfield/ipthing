package main

import (
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/exaring/ja4plus"
)

// clientHelloFP holds TLS ClientHello fingerprints for one TCP/QUIC connection.
type clientHelloFP struct {
	JA3 string
	JA4 string
}

// tlsHelloStore maps RemoteAddr → ClientHello fingerprints for the life of a connection.
// crypto/tls does not pass ClientHello data to HTTP handlers; this bridges that gap.
type tlsHelloStore struct {
	byAddr sync.Map // string → clientHelloFP
}

func newTLSHelloStore() *tlsHelloStore {
	return &tlsHelloStore{}
}

// Remember stores JA3/JA4 from a ClientHello (call from GetConfigForClient).
func (s *tlsHelloStore) Remember(hello *tls.ClientHelloInfo) {
	if s == nil || hello == nil || hello.Conn == nil {
		return
	}
	s.byAddr.Store(hello.Conn.RemoteAddr().String(), clientHelloFP{
		JA3: ja3FromClientHello(hello),
		JA4: ja4plus.JA4(hello),
	})
}

// Lookup returns fingerprints for a request's RemoteAddr, if known.
func (s *tlsHelloStore) Lookup(remoteAddr string) (clientHelloFP, bool) {
	if s == nil || remoteAddr == "" {
		return clientHelloFP{}, false
	}
	v, ok := s.byAddr.Load(remoteAddr)
	if !ok {
		return clientHelloFP{}, false
	}
	fp, ok := v.(clientHelloFP)
	return fp, ok
}

// ConnState clears cached fingerprints when connections close.
func (s *tlsHelloStore) ConnState(conn net.Conn, state http.ConnState) {
	if s == nil || conn == nil {
		return
	}
	switch state {
	case http.StateClosed, http.StateHijacked:
		s.byAddr.Delete(conn.RemoteAddr().String())
	}
}

// ja3FromClientHello builds a JA3 digest from crypto/tls.ClientHelloInfo.
// GREASE values are omitted. The TLS version used is the highest advertised
// supported_versions entry (ClientHelloInfo does not expose the record version).
func ja3FromClientHello(hello *tls.ClientHelloInfo) string {
	version := uint16(0)
	for _, v := range hello.SupportedVersions {
		if isTLSGrease(v) {
			continue
		}
		if v > version {
			version = v
		}
	}

	ciphers := make([]string, 0, len(hello.CipherSuites))
	for _, c := range hello.CipherSuites {
		if !isTLSGrease(c) {
			ciphers = append(ciphers, strconv.FormatUint(uint64(c), 10))
		}
	}

	exts := make([]string, 0, len(hello.Extensions))
	for _, e := range hello.Extensions {
		if !isTLSGrease(e) {
			exts = append(exts, strconv.FormatUint(uint64(e), 10))
		}
	}

	curves := make([]string, 0, len(hello.SupportedCurves))
	for _, c := range hello.SupportedCurves {
		if !isTLSGrease(uint16(c)) {
			curves = append(curves, strconv.FormatUint(uint64(c), 10))
		}
	}

	points := make([]string, 0, len(hello.SupportedPoints))
	for _, p := range hello.SupportedPoints {
		points = append(points, strconv.FormatUint(uint64(p), 10))
	}

	raw := fmt.Sprintf("%d,%s,%s,%s,%s",
		version,
		strings.Join(ciphers, "-"),
		strings.Join(exts, "-"),
		strings.Join(curves, "-"),
		strings.Join(points, "-"),
	)
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// isTLSGrease reports GREASE values (RFC 8701).
func isTLSGrease(v uint16) bool {
	return v&0x000f == 0x000a && v>>8 == (v&0x00ff)
}

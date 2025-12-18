package main

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTLSVersionString(t *testing.T) {
	tests := []struct {
		version  uint16
		expected string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0000, "Unknown"},
	}

	for _, tt := range tests {
		result := getTLSVersionString(tt.version)
		assert.Equal(t, tt.expected, result, "Version %x should return %s", tt.version, tt.expected)
	}
}

func TestGetTLSCipherSuiteString(t *testing.T) {
	tests := []struct {
		cipherSuite uint16
		expected    string
	}{
		// TLS 1.3 cipher suites
		{tls.TLS_AES_128_GCM_SHA256, "TLS_AES_128_GCM_SHA256"},
		{tls.TLS_AES_256_GCM_SHA384, "TLS_AES_256_GCM_SHA384"},
		{tls.TLS_CHACHA20_POLY1305_SHA256, "TLS_CHACHA20_POLY1305_SHA256"},

		// TLS 1.2 cipher suites
		{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
		{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"},
		{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
		{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"},
		{tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"},

		// Unknown cipher suite
		{0x0000, "Unknown"},
	}

	for _, tt := range tests {
		result := getTLSCipherSuiteString(tt.cipherSuite)
		assert.Equal(t, tt.expected, result, "Cipher suite %x should return %s", tt.cipherSuite, tt.expected)
	}
}

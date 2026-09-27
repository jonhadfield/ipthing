package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

const ptrLookupTimeout = 400 * time.Millisecond

// ipFamily returns "ipv4", "ipv6", or "" for an IP string (no port).
func ipFamily(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if parsed.To4() != nil {
		return "ipv4"
	}
	return "ipv6"
}

// lookupPTR resolves a reverse DNS name for ip with a short timeout.
// Returns "" on failure; never blocks longer than ptrLookupTimeout.
// Skips private/loopback/link-local addresses.
func lookupPTR(ctx context.Context, ip string) string {
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, ptrLookupTimeout)
	defer cancel()

	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// cookieNames returns Cookie header names only (values discarded).
func cookieNames(h http.Header) []string {
	if h.Get("Cookie") == "" {
		return nil
	}
	req := &http.Request{Header: h}
	cookies := req.Cookies()
	if len(cookies) == 0 {
		return nil
	}
	names := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return names
}

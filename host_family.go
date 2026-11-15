package main

import (
	"fmt"
	"strings"
)

// familyHostHint returns "ipv4" or "ipv6" when the request Host is a
// dedicated dual-stack name (ipv4.* / ipv6.*), otherwise "".
func familyHostHint(host string) string {
	h := strings.ToLower(host)
	if i := strings.IndexByte(h, ':'); i >= 0 {
		// Strip :port; leave IPv6 literals in brackets alone (not used for Host).
		if !strings.HasPrefix(h, "[") {
			h = h[:i]
		}
	}
	switch {
	case strings.HasPrefix(h, "ipv4."):
		return "ipv4"
	case strings.HasPrefix(h, "ipv6."):
		return "ipv6"
	default:
		return ""
	}
}

// locMapURL builds an OpenStreetMap deep link from an ipinfo "lat,lon" string.
func locMapURL(loc string) string {
	parts := strings.Split(loc, ",")
	if len(parts) != 2 {
		return ""
	}
	lat := strings.TrimSpace(parts[0])
	lon := strings.TrimSpace(parts[1])
	if lat == "" || lon == "" {
		return ""
	}
	return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%s&mlon=%s#map=10/%s/%s", lat, lon, lat, lon)
}

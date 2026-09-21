package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	ipInfoCacheTTL             = 24 * time.Hour
	maxConcurrentIPInfoFetches = 8
)

// Overridable in tests.
var (
	ipInfoURLFormat  = "https://ipinfo.io/%s/json"
	ipInfoHTTPClient = &http.Client{Timeout: 5 * time.Second}
	ipInfoFetchSem   = make(chan struct{}, maxConcurrentIPInfoFetches)
)

// shouldUpdateIPInfo reports whether cached geo data is older than the TTL.
func shouldUpdateIPInfo(lastUpdate time.Time) bool {
	return time.Since(lastUpdate) > ipInfoCacheTTL
}

func fetchIPInfo(ctx context.Context, ip string) (*IPInfo, error) {
	select {
	case ipInfoFetchSem <- struct{}{}:
		defer func() { <-ipInfoFetchSem }()
	case <-ctx.Done():
		return nil, fmt.Errorf("ipinfo fetch queued: %w", ctx.Err())
	}

	url := fmt.Sprintf(ipInfoURLFormat, ip)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create IP info request: %w", err)
	}

	resp, err := ipInfoHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch IP info: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipinfo API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var info IPInfo
	if err = json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("failed to parse IP info: %w", err)
	}

	info.LastUpdate = time.Now()

	return &info, nil
}

func getOrFetchIPInfo(ctx context.Context, db Database, ip string) (*IPInfo, error) {
	info, err := db.GetIPInfo(ip)
	if err == nil && !shouldUpdateIPInfo(info.LastUpdate) {
		return info, nil
	}

	newInfo, err := fetchIPInfo(ctx, ip)
	if err != nil {
		if info != nil {
			return info, nil
		}
		return nil, err
	}

	if err := db.SaveIPInfo(newInfo); err != nil {
		return newInfo, nil
	}

	return newInfo, nil
}

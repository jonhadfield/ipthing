package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const ipInfoAPIURL = "https://ipinfo.io/%s/json"

func fetchIPInfo(ip string) (*IPInfo, error) {
	url := fmt.Sprintf(ipInfoAPIURL, ip)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch IP info: %w", err)
	}
	defer resp.Body.Close()

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

func getOrFetchIPInfo(db Database, ip string) (*IPInfo, error) {
	info, err := db.GetIPInfo(ip)
	if err == nil && !shouldUpdateIPInfo(info.LastUpdate) {
		return info, nil
	}

	newInfo, err := fetchIPInfo(ip)
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

package main

import "time"

type IPInfo struct {
	IP         string `json:"ip"`
	City       string `json:"city"`
	Region     string `json:"region"`
	Country    string `json:"country"`
	Loc        string `json:"loc"`
	Org        string `json:"org"`
	Postal     string `json:"postal"`
	Timezone   string `json:"timezone"`
	Hostname   string `json:"hostname"`
	Bogon      bool   `json:"bogon"`
	LastUpdate time.Time
}

type HTTPRequest struct {
	ID          int64
	IP          string
	Method      string
	Path        string
	UserAgent   string
	Referer     string
	Headers     string
	QueryParams string
	Timestamp   time.Time
}

type RequestFingerprint struct {
	IP          string
	Path        string
	QueryParams string
	UserAgent   string
}

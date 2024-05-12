package main

import (
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPopulateReq(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cf-Connecting-Ip", "192.168.1.1")
	req.Header.Set("Cf-IPcountry", "US")
	req.Header.Set("Cf-Visitor", "Visitor")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("DNT", "1")
	req.Header.Set("Language", "en")
	req.Header.Set("Referer", "http://localhost")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Charset", "UTF-8")
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	expected := &Req{
		IPAddr:         "192.168.1.1",
		Country:        "US",
		Visitor:        "Visitor",
		UserAgent:      "",
		Host:           "example.com",
		Accept:         "application/json",
		AcceptEncoding: "gzip, deflate, br",
		AcceptLanguage: "en-US,en;q=0.9",
		DNT:            "1",
		Language:       "en",
		Referer:        "http://localhost",
		Method:         "GET",
		MIMEType:       "application/json",
		Charset:        "UTF-8",
		XFF:            "",
	}

	actual := populateReq(c.Request())
	assert.Equal(t, expected, actual)
}

func TestParseXFF(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Add("X-Forwarded-For", "203.0.113.199, 192.168.1.100")
	addr, err := parseXFF(echo.New(), req, true, true, true)
	require.NoError(t, err)
	require.NotEmpty(t, addr)
	require.Equal(t, "203.0.113.199", addr)
}

func TestStripTrustedFromXFF(t *testing.T) {
	in := []struct {
		xff        string
		trustedPos int
		expected   string
	}{
		{
			xff:        "1.1.1.1,2.2.2.2,3.3.3.3",
			trustedPos: 1,
			expected:   "1.1.1.1,2.2.2.2",
		},
	}

	for x := range in {
		actual := stripTrustedFromXFF(in[x].xff, in[x].trustedPos)
		assert.Equal(t, in[x].expected, actual)
	}
}

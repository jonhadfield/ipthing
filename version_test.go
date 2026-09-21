package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestVersionString(t *testing.T) {
	prevTag, prevSHA, prevVersion := buildTag, buildSHA, version
	t.Cleanup(func() {
		buildTag, buildSHA, version = prevTag, prevSHA, prevVersion
	})

	buildTag, buildSHA, version = "v1.0.0", "abc1234", "ignored"
	assert.Equal(t, "v1.0.0-abc1234", versionString())

	buildTag, buildSHA = "v1.0.0", ""
	assert.Equal(t, "v1.0.0", versionString())

	buildTag, buildSHA = "", "abc1234"
	assert.Equal(t, "abc1234", versionString())

	buildTag, buildSHA, version = "", "", "dev"
	assert.Equal(t, "dev", versionString())
}

func TestVersionHeaderMiddleware(t *testing.T) {
	prevTag, prevSHA := buildTag, buildSHA
	buildTag, buildSHA = "v1.2.3", "deadbee"
	t.Cleanup(func() {
		buildTag, buildSHA = prevTag, prevSHA
	})

	e := echo.New()
	e.Use(versionHeaderMiddleware())
	e.GET("/", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "v1.2.3-deadbee", rec.Header().Get(versionHeader))
}

package main

import (
	"github.com/labstack/echo/v4"
)

// Build metadata. Prefer setting buildTag and buildSHA via -ldflags;
// version remains the longer log string used at startup.
var (
	version  = "dev"
	buildTag = ""
	buildSHA = ""
)

const versionHeader = "X-IPThing-Version"

// versionString returns a compact tag+sha identifier for response headers.
func versionString() string {
	tag := buildTag
	sha := buildSHA
	switch {
	case tag != "" && sha != "":
		return tag + "-" + sha
	case tag != "":
		return tag
	case sha != "":
		return sha
	case version != "":
		return version
	default:
		return "dev"
	}
}

// versionHeaderMiddleware adds X-IPThing-Version on every response.
func versionHeaderMiddleware() echo.MiddlewareFunc {
	v := versionString()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set(versionHeader, v)
			return next(c)
		}
	}
}

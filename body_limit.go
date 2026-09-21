package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// maxRequestBodyBytes caps inbound request bodies.
//
// This service only inspects connection metadata and never processes payloads.
// 1 MiB matches common API practice (reject early, avoid buffering abuse) while
// still allowing modest scanner/probe POST bodies to be measured and logged.
const maxRequestBodyBytes int64 = 1 << 20 // 1 MiB

func bodyTooLargeError() *echo.HTTPError {
	return echo.NewHTTPError(
		http.StatusRequestEntityTooLarge,
		fmt.Sprintf("request body exceeds %d byte limit", maxRequestBodyBytes),
	)
}

// requestBodyLimitMiddleware rejects oversized bodies with 413 and records
// rejected attempts (with claimed Content-Length) when a database is configured.
// Known Content-Length over the limit is rejected without reading. Chunked or
// otherwise unknown-length bodies are capped via MaxBytesReader and drained so
// the limit cannot be bypassed when handlers do not read the body.
func (app *Application) requestBodyLimitMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			if req.ContentLength > maxRequestBodyBytes {
				app.recordRejectedBody(c)
				return bodyTooLargeError()
			}

			if req.Body != nil && req.Body != http.NoBody {
				req.Body = http.MaxBytesReader(c.Response(), req.Body, maxRequestBodyBytes)
				// Enforce limit for chunked / unknown length (handlers do not read bodies).
				if req.ContentLength < 0 {
					if err := drainRequestBody(req.Body); err != nil {
						if isBodyTooLarge(err) {
							app.recordRejectedBody(c)
							return bodyTooLargeError()
						}
						return err
					}
					req.Body = http.NoBody
				}
			}

			return next(c)
		}
	}
}

func drainRequestBody(body io.ReadCloser) error {
	_, err := io.Copy(io.Discard, body)
	closeErr := body.Close()
	return errors.Join(err, closeErr)
}

func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// recordRejectedBody persists a 413 attempt so payload-size probing is visible in analytics.
func (app *Application) recordRejectedBody(c echo.Context) {
	if app.handler == nil || app.handler.requestProcessor == nil {
		return
	}
	rp := app.handler.requestProcessor
	clientIP := rp.extractClientIP(c)
	httpReq := rp.buildHTTPRequest(c.Request(), clientIP)
	httpReq.StatusCode = http.StatusRequestEntityTooLarge
	httpReq.ResponseFormat = "error"
	// Prefer claimed Content-Length; if absent (chunked), record the limit-breach size.
	if httpReq.ContentLength < 0 {
		httpReq.ContentLength = maxRequestBodyBytes + 1
	}
	rp.QueueStore(httpReq)
}

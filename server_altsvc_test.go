package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"unsafe"

	"github.com/labstack/echo/v4"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAltSvcMiddleware_NilServer(t *testing.T) {
	e := echo.New()
	called := false
	next := func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusNoContent)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, altSvcMiddleware(nil)(next)(c))
	assert.True(t, called)
	assert.Empty(t, rec.Header().Get("Alt-Svc"))
}

func TestAltSvcMiddleware_SetsQUICHeaders(t *testing.T) {
	e := echo.New()
	h3 := &http3.Server{Port: 443}
	seedAltSvcHeader(t, h3, `h3=":443"; ma=2592000`)

	called := false
	next := func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, altSvcMiddleware(h3)(next)(c))
	assert.True(t, called)
	assert.Contains(t, rec.Header().Get("Alt-Svc"), `h3=":443"`)
}

// seedAltSvcHeader stores a value into http3.Server's unexported altSvcHeader
// so SetQUICHeaders can succeed without binding a real QUIC listener.
func seedAltSvcHeader(t *testing.T, h3 *http3.Server, value string) {
	t.Helper()
	field := reflect.ValueOf(h3).Elem().FieldByName("altSvcHeader")
	require.True(t, field.IsValid(), "altSvcHeader field missing; quic-go API changed")
	ptr := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr()))
	store := ptr.Elem().Addr().MethodByName("Store")
	require.True(t, store.IsValid(), "altSvcHeader.Store missing")
	v := value
	store.Call([]reflect.Value{reflect.ValueOf(&v)})
}

package main

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"io"
	"net/http"
	"strings"
	"text/template"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type Req struct {
	IPAddr         string
	UserAgent      string
	Country        string
	Visitor        string
	Host           string
	Connection     string
	Accept         string
	AcceptEncoding string
	AcceptLanguage string
	DNT            string
	Language       string
	Referer        string
	Method         string
	MIMEType       string
	Charset        string
	XFF            string
	XRI            string
}

func poplateReq(r *http.Request, reqIP string) *Req {
	return &Req{
		IPAddr:         r.Header.Get("Cf-Connecting-Ip"),
		Country:        r.Header.Get("Cf-IPcountry"),
		Visitor:        r.Header.Get("Cf-Visitor"),
		UserAgent:      r.UserAgent(),
		Host:           r.Host,
		Connection:     r.Header.Get("Connection"),
		Accept:         r.Header.Get("Accept"),
		AcceptEncoding: r.Header.Get("Accept-Encoding"),
		AcceptLanguage: r.Header.Get("Accept-Language"),
		DNT:            r.Header.Get("DNT"),
		Language:       r.Header.Get("Language"),
		Referer:        r.Header.Get("Referer"),
		Method:         r.Method,
		MIMEType:       r.Header.Get("Content-Type"),
		Charset:        r.Header.Get("Charset"),
		XFF:            r.Header.Get("X-Forwarded-For"),
		XRI:            r.Header.Get("X-Real-IP"),
	}
}

func main() {
	t := &Template{
		templates: template.Must(template.ParseGlob("public/views/*.html")),
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Renderer = t
	e.File("/favicon.ico", "public/assets/favicon.ico")
	e.File("/favicon-16x16.png", "public/assets/favicon-16x16.png")
	e.File("/favicon-32x32.png", "public/assets/favicon-32x32.png")
	e.File("/apple-touch-icon.png", "public/assets/apple-touch-icon.png")
	e.GET("/", func(c echo.Context) error {
		e.IPExtractor = echo.ExtractIPFromXFFHeader(
			echo.TrustLoopback(false),   // e.g. ipv4 start with 127.
			echo.TrustLinkLocal(false),  // e.g. ipv4 start with 169.254
			echo.TrustPrivateNet(false), // e.g. ipv4 start with 10. or 192.168
			//echo.TrustIPRange(lbIPRange),
		)
		e.Logger.Info("IPExtractor: ", e.IPExtractor(c.Request()))

		for k, v := range c.Request().Header {
			fmt.Printf("%s: %s\n", k, v)
		}

		ua := c.Request().UserAgent()
		switch {
		case strings.Contains(ua, "curl"):
			consoleData, _ := json.MarshalIndent(poplateReq(c.Request(), e.IPExtractor(c.Request())), "", "  ")
			return c.Render(http.StatusOK, "console", string(consoleData))
		default:
			return c.Render(http.StatusOK, "web", poplateReq(c.Request(), e.IPExtractor(c.Request())))
		}

	})
	e.Logger.Fatal(e.Start(":1323"))
}

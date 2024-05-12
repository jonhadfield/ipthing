package main

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"io"
	"net/http"
	"regexp"
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
	IPAddr         string `json:"ip_addr,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`
	Country        string `json:"country,omitempty"`
	Visitor        string `json:"visitor,omitempty"`
	Host           string `json:"host,omitempty"`
	Accept         string `json:"accept,omitempty"`
	AcceptEncoding string `json:"accept_encoding,omitempty"`
	AcceptLanguage string `json:"accept_language,omitempty"`
	DNT            string `json:"dnt,omitempty"`
	Language       string `json:"language,omitempty"`
	Referer        string `json:"referer,omitempty"`
	Method         string `json:"method,omitempty"`
	MIMEType       string `json:"mime_type,omitempty"`
	Charset        string `json:"charset,omitempty"`
	XFF            string `json:"xff,omitempty"`
	XRI            string `json:"xri,omitempty"`
}

func stripPrivateIPs(xff string) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.Contains(parts[i], ":") {
			return strings.Join(parts[:i], ",")
		}
	}

	return xff

}

func stripXFF(xff string, trustedPos int, stripPrivate bool) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	if len(parts) <= trustedPos {
		return ""
	}

	realTrustedPos := len(parts) - trustedPos

	return strings.Join(parts[:realTrustedPos], ",")
}

func populateReq(r *http.Request) *Req {
	sXFF := stripXFF(r.Header.Get("X-Forwarded-For"), 1, false)

	req := &Req{
		IPAddr:         r.Header.Get("Cf-Connecting-Ip"),
		Country:        r.Header.Get("Cf-IPcountry"),
		Visitor:        r.Header.Get("Cf-Visitor"),
		UserAgent:      r.UserAgent(),
		Host:           r.Host,
		Accept:         r.Header.Get("Accept"),
		AcceptEncoding: r.Header.Get("Accept-Encoding"),
		AcceptLanguage: r.Header.Get("Accept-Language"),
		DNT:            r.Header.Get("DNT"),
		Language:       r.Header.Get("Language"),
		Referer:        r.Header.Get("Referer"),
		Method:         r.Method,
		MIMEType:       r.Header.Get("Content-Type"),
		Charset:        r.Header.Get("Charset"),
		XFF:            sXFF,
	}

	return req
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
		r := regexp.MustCompile(`.*(Mozilla|AppleWebKit|Trident|Presto|Gecko|KHTML|Blink|Lynx|Links|w3m|elinks).*`)

		//firstUntrusted, err := parseXFF(e, c.Request(), true, true, true)
		//if err != nil {
		//	return err
		//}
		//e.Logger.Info(firstUntrusted)

		switch {
		case !r.MatchString(c.Request().UserAgent()):
			consoleData, _ := json.MarshalIndent(populateReq(c.Request()), "", "  ")
			return c.Render(http.StatusOK, "console", string(consoleData))
		default:
			return c.Render(http.StatusOK, "web", populateReq(c.Request()))
		}
	})

	e.Logger.Fatal(e.Start(":1323"))
}

func parseXFF(e *echo.Echo, r *http.Request, trustLoopback, trustLinkLocal, trustPrivateNet bool) (string, error) {
	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(trustLoopback),
		echo.TrustLinkLocal(trustLinkLocal),
		echo.TrustPrivateNet(trustPrivateNet),
	)

	ip := e.IPExtractor(r)
	echo.ExtractIPFromXFFHeader()

	return ip, nil
}

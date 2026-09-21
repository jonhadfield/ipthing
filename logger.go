package main

import (
	"io"
	"log"
	"os"

	"github.com/labstack/echo/v4"
	echoLog "github.com/labstack/gommon/log"
)

// syslogSink is the subset of syslog.Writer used by the app.
// log/syslog is unavailable on Windows, so the concrete type is platform-specific.
type syslogSink interface {
	Info(m string) error
	Close() error
}

// SyslogWriter wraps a syslogSink to implement io.Writer.
type SyslogWriter struct {
	writer syslogSink
}

// Write implements io.Writer.
func (s *SyslogWriter) Write(p []byte) (n int, err error) {
	if s.writer != nil {
		msg := string(p)
		if len(msg) > 0 && msg[len(msg)-1] == '\n' {
			msg = msg[:len(msg)-1]
		}
		return len(p), s.writer.Info(msg)
	}
	return len(p), nil
}

// setupLogger configures both standard log and Echo logger to use syslog when available.
func setupLogger(sink syslogSink) io.Writer {
	var writer io.Writer

	if sink != nil {
		writer = &SyslogWriter{writer: sink}
		log.SetOutput(writer)
		log.SetFlags(0) // Syslog handles timestamps
	} else {
		writer = os.Stdout
		log.SetOutput(writer)
		log.SetFlags(log.LstdFlags)
	}

	return writer
}

// configureEchoLogger sets up Echo's logger to use syslog when available.
func configureEchoLogger(e *echo.Echo, sink syslogSink) {
	if sink != nil {
		e.Logger.SetOutput(&SyslogWriter{writer: sink})
		e.Logger.SetLevel(echoLog.INFO)
		e.Logger.SetHeader("${time_rfc3339} ${level}")
	} else {
		e.Logger.SetOutput(os.Stdout)
		e.Logger.SetLevel(echoLog.INFO)
	}
}

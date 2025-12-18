package main

import (
	"io"
	"log"
	"log/syslog"
	"os"

	"github.com/labstack/echo/v4"
	echoLog "github.com/labstack/gommon/log"
)

// SyslogWriter wraps syslog.Writer to implement io.Writer
type SyslogWriter struct {
	writer *syslog.Writer
}

// Write implements io.Writer interface
func (s *SyslogWriter) Write(p []byte) (n int, err error) {
	if s.writer != nil {
		// Remove trailing newline if present
		msg := string(p)
		if len(msg) > 0 && msg[len(msg)-1] == '\n' {
			msg = msg[:len(msg)-1]
		}
		return len(p), s.writer.Info(msg)
	}
	return len(p), nil
}

// setupSyslog configures syslog for the application
func setupSyslog() (*syslog.Writer, error) {
	// Try to connect to syslog
	writer, err := syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "ipthing")
	if err != nil {
		// If syslog is not available, log to stderr
		log.Printf("Warning: Failed to connect to syslog: %v", err)
		log.Printf("Falling back to stderr logging")
		return nil, err
	}

	return writer, nil
}

// setupLogger configures both standard log and Echo logger to use syslog
func setupLogger(syslogWriter *syslog.Writer) io.Writer {
	var writer io.Writer

	if syslogWriter != nil {
		// Use syslog writer
		writer = &SyslogWriter{writer: syslogWriter}
		log.SetOutput(writer)
		log.SetFlags(0) // Syslog handles timestamps
	} else {
		// Fall back to stdout with timestamps
		writer = os.Stdout
		log.SetOutput(writer)
		log.SetFlags(log.LstdFlags)
	}

	return writer
}

// configureEchoLogger sets up Echo's logger to use syslog
func configureEchoLogger(e *echo.Echo, syslogWriter *syslog.Writer) {
	if syslogWriter != nil {
		// Configure Echo logger to use syslog
		e.Logger.SetOutput(&SyslogWriter{writer: syslogWriter})
		e.Logger.SetLevel(echoLog.INFO)
		e.Logger.SetHeader("${time_rfc3339} ${level}")
	} else {
		// Use default Echo logger configuration
		e.Logger.SetOutput(os.Stdout)
		e.Logger.SetLevel(echoLog.INFO)
	}
}

//go:build !windows

package main

import (
	"log"
	"log/syslog"
)

func setupSyslog() (syslogSink, error) {
	writer, err := syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "ipthing")
	if err != nil {
		log.Printf("Warning: Failed to connect to syslog: %v", err)
		log.Printf("Falling back to stderr logging")
		return nil, err
	}
	return writer, nil
}

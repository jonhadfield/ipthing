//go:build windows

package main

import "errors"

func setupSyslog() (syslogSink, error) {
	return nil, errors.New("syslog is not available on Windows")
}

package main

import (
	"fmt"
	"net/http"
)

func dumpRequest(r *http.Request) error {
	if r.Header != nil {
		for key, values := range r.Header {
			fmt.Println("Header:", key, values)
		}
	}

	if r.TLS != nil {
		fmt.Println("TLS Version:", r.TLS.Version)
		fmt.Println("Cipher Suite:", r.TLS.CipherSuite)
	} else {
		fmt.Println("No TLS information available.")
	}

	return nil
}

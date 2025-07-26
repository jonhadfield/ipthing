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

	return nil
}

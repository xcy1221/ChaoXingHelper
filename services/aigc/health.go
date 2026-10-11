package main

import (
	"net"
	"net/http"
	"os"
	"time"
)

func checkHealth() bool {
	address := os.Getenv("AIGC_LISTEN")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + "/api/health")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

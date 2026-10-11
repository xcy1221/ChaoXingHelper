package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func (r *ChaoXingRunner) browserState() string {
	if !r.onDemand {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(r.dir, "account", "browser-status.json"))
	var state struct {
		State string `json:"state"`
	}
	if err != nil || json.Unmarshal(data, &state) != nil {
		return "error"
	}
	switch state.State {
	case "sleeping", "starting", "running", "stopping", "login", "error":
		return state.State
	}
	return "error"
}

// Shared exclusive lease prevents idle shutdown from racing a queue claim.
func browserLease(dir, owner string) (func(), error) {
	name := filepath.Join(dir, "account", "browser-use.lock")
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString(owner)
	f.Close()
	if err != nil {
		os.Remove(name)
		return nil, err
	}
	return func() { os.Remove(name) }, nil
}
func resetWorkerLease(dir string) {
	name := filepath.Join(dir, "account", "browser-use.lock")
	if data, err := os.ReadFile(name); err == nil && string(data) == "worker" {
		os.Remove(name)
	}
}
func (r *ChaoXingRunner) waitBrowser(ctx context.Context) error {
	address, err := url.Parse(r.config.CDPURL)
	if err != nil {
		return errors.New("浏览器地址无效")
	}
	address.Scheme = "http"
	address.Path = "/json/version"
	address.RawQuery = ""
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	wait, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return until(wait, time.Second, func() (bool, error) {
		req, err := http.NewRequestWithContext(wait, http.MethodGet, address.String(), nil)
		if err != nil {
			return false, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()
		var version struct {
			Socket string `json:"webSocketDebuggerUrl"`
		}
		return resp.StatusCode == 200 && json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&version) == nil && bridgeSocketURL(version.Socket), nil
	})
}

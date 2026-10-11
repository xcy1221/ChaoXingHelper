package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func TestControlBindRequiresOwnPrivateAddress(t *testing.T) {
	local := []net.Addr{&net.IPNet{IP: net.ParseIP("172.29.1.2"), Mask: net.CIDRMask(24, 32)}, &net.IPNet{IP: net.ParseIP("172.30.1.2"), Mask: net.CIDRMask(24, 32)}}
	ip, err := controlBindIP([]net.IP{net.ParseIP("172.29.1.2")}, local)
	if err != nil || ip.String() != "172.29.1.2" {
		t.Fatal("control address rejected", err)
	}
	for _, lookup := range [][]net.IP{
		{net.ParseIP("127.0.0.1")},
		{net.ParseIP("8.8.8.8")},
		{net.ParseIP("172.29.1.9")},
		{net.ParseIP("172.29.1.2"), net.ParseIP("172.30.1.2")},
	} {
		if _, err := controlBindIP(lookup, local); err == nil {
			t.Fatal("invalid or ambiguous interface accepted")
		}
	}
}
func TestCDPBridgeVersionAndIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, "127.0.0.1:") || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("upstream host or private headers not normalized")
		}
		json.NewEncoder(w).Encode(map[string]string{"Browser": "Chromium/fixture", "webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/fixture"})
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	bridge := httptest.NewServer(newCDPBridge(target))
	defer bridge.Close()
	req, _ := http.NewRequest("GET", bridge.URL+"/json/version", nil)
	req.Header.Set("Cookie", "fixture")
	req.Header.Set("Authorization", "fixture")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var version map[string]string
	err = json.NewDecoder(resp.Body).Decode(&version)
	resp.Body.Close()
	expected := "ws" + strings.TrimPrefix(bridge.URL, "http") + "/devtools/browser/fixture"
	if err != nil || resp.StatusCode != 200 || version["webSocketDebuggerUrl"] != expected || version["Browser"] != "Chromium/fixture" {
		t.Fatal("version rewrite failed", version, err)
	}
	for _, item := range []struct {
		path, method, host, origin string
		code                       int
	}{
		{"/json/list", "GET", "", "", 404},
		{"/json/version", "POST", "", "", 403},
		{"/json/version", "GET", "public.example:9223", "", 403},
		{"/json/version", "GET", "", "https://mooc1.chaoxing.com", 403},
		{"/json/version?unexpected=1", "GET", "", "", 403},
	} {
		r, _ := http.NewRequest(item.method, bridge.URL+item.path, nil)
		if item.host != "" {
			r.Host = item.host
		}
		if item.origin != "" {
			r.Header.Set("Origin", item.origin)
		}
		reply, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		reply.Body.Close()
		if reply.StatusCode != item.code {
			t.Fatalf("%s=%d", item.path, reply.StatusCode)
		}
	}
}
func TestCDPBridgeWebSocket(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		data, _, err := wsutil.ReadClientData(conn)
		if err != nil {
			t.Error(err)
			return
		}
		if err = wsutil.WriteServerText(conn, data); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	bridge := httptest.NewServer(newCDPBridge(target))
	defer bridge.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, _, err := ws.Dial(ctx, "ws"+strings.TrimPrefix(bridge.URL, "http")+"/devtools/browser/fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	payload := []byte("{\"method\":\"Target.getTargets\"}")
	if err = wsutil.WriteClientText(conn, payload); err != nil {
		t.Fatal(err)
	}
	data, _, err := wsutil.ReadServerData(conn)
	if err != nil || string(data) != string(payload) {
		t.Fatal("websocket forwarding failed", err)
	}
}
func TestCDPBridgeRejectsInvalidVersion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://public.example:9222/devtools/browser/fixture"})
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	bridge := httptest.NewServer(newCDPBridge(target))
	defer bridge.Close()
	resp, err := http.Get(bridge.URL + "/json/version")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatal("invalid socket accepted")
	}
}

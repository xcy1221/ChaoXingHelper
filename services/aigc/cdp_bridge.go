package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

const controlAlias = "chromium-control"

var browserSocketPath = regexp.MustCompile("^/devtools/browser/[a-zA-Z0-9-]+$")

type bridgeHostKey struct{}

func controlBindIP(resolved []net.IP, local []net.Addr) (net.IP, error) {
	own := map[string]bool{}
	for _, addr := range local {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil && ip.To4() != nil {
			own[ip.String()] = true
		}
	}
	found := map[string]net.IP{}
	for _, ip := range resolved {
		if ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() && own[ip.String()] {
			found[ip.String()] = ip
		}
	}
	if len(found) != 1 {
		return nil, errors.New("内部控制网络地址不唯一或不可用")
	}
	for _, ip := range found {
		return ip, nil
	}
	return nil, errors.New("控制网络不可用")
}
func bridgeHostAllowed(raw string) bool {
	u, err := url.Parse("http://" + raw)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	return host == controlAlias || host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate()))
}
func bridgeSocketURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "ws" && bridgeHostAllowed(u.Host) && u.User == nil &&
		browserSocketPath.MatchString(u.Path) && u.RawQuery == "" && u.Fragment == ""
}
func newCDPBridge(upstream *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = upstream.Host
		r.Header.Del("Cookie")
		r.Header.Del("Authorization")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "浏览器控制端暂不可用", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Request.URL.Path != "/json/version" || resp.StatusCode != http.StatusOK {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		resp.Body.Close()
		if err != nil || len(data) > 1<<20 {
			return errors.New("浏览器元数据无效")
		}
		var version map[string]json.RawMessage
		if json.Unmarshal(data, &version) != nil {
			return errors.New("浏览器元数据无效")
		}
		var rawSocket string
		if json.Unmarshal(version["webSocketDebuggerUrl"], &rawSocket) != nil || !bridgeSocketURL(rawSocket) {
			return errors.New("浏览器控制地址无效")
		}
		host, ok := resp.Request.Context().Value(bridgeHostKey{}).(string)
		if !ok || !bridgeHostAllowed(host) {
			return errors.New("控制通道主机无效")
		}
		socket, _ := url.Parse(rawSocket)
		socket.Host = host
		version["webSocketDebuggerUrl"], _ = json.Marshal(socket.String())
		rewritten, err := json.Marshal(version)
		if err != nil {
			return err
		}
		resp.Body = io.NopCloser(bytes.NewReader(rewritten))
		resp.ContentLength = int64(len(rewritten))
		resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
		resp.Header.Set("Cache-Control", "no-store")
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || !bridgeHostAllowed(r.Host) || r.Header.Get("Origin") != "" {
			http.Error(w, "控制通道请求无效", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/json/version" && !browserSocketPath.MatchString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), bridgeHostKey{}, r.Host)))
	})
}
func checkCDPBridge() bool {
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://" + controlAlias + ":9223/json/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var version struct {
		Socket string `json:"webSocketDebuggerUrl"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&version) != nil {
		return false
	}
	return bridgeSocketURL(version.Socket)
}
func runCDPBridge() error {
	resolved, err := net.LookupIP(controlAlias)
	if err != nil {
		return errors.New("内部控制网络无法解析")
	}
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return err
	}
	ip, err := controlBindIP(resolved, interfaces)
	if err != nil {
		return err
	}
	target, _ := url.Parse("http://127.0.0.1:9222")
	server := &http.Server{Addr: net.JoinHostPort(ip.String(), "9223"), Handler: newCDPBridge(target),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

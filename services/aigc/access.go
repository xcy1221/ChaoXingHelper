package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

type accessIdentity struct {
	User, IP string
	External bool
}
type identityKey struct{}

var accessUser = regexp.MustCompile("^[a-zA-Z0-9_-]{1,40}$")

func loadGatewayToken() (string, error) {
	file := os.Getenv("AIGC_GATEWAY_TOKEN_FILE")
	if file == "" {
		return "", nil
	}
	data, err := os.ReadFile(file)
	token := strings.TrimSpace(string(data))
	if err != nil || len(token) < 32 {
		return "", errors.New("网关认证配置不可用")
	}
	return token, nil
}
func (a *app) identify(r *http.Request) (accessIdentity, bool) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if a.gatewayToken == "" {
		return accessIdentity{IP: ip}, true
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Aigc-Gateway")), []byte(a.gatewayToken)) != 1 {
		return accessIdentity{}, false
	}
	ip = r.Header.Get("X-Aigc-Client-IP")
	if net.ParseIP(ip) == nil {
		return accessIdentity{}, false
	}
	scope := r.Header.Get("X-Aigc-Scope")
	if scope == "internal" {
		return accessIdentity{IP: ip}, true
	}
	user := r.Header.Get("X-Aigc-User")
	if scope != "external" || !accessUser.MatchString(user) {
		return accessIdentity{}, false
	}
	if a.publicUsersFile != "" {
		data, err := os.ReadFile(a.publicUsersFile)
		var users map[string]int64
		if err != nil || json.Unmarshal(data, &users) != nil || users[user] <= time.Now().Unix() {
			return accessIdentity{}, false
		}
	}
	return accessIdentity{User: user, IP: ip, External: true}, true
}
func identity(r *http.Request) accessIdentity {
	v, _ := r.Context().Value(identityKey{}).(accessIdentity)
	return v
}
func (a *app) access(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.identify(r)
		remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		localHealth := r.URL.Path == "/api/health" && net.ParseIP(remoteIP).IsLoopback() && r.Header.Get("X-Aigc-Gateway") == ""
		if !ok && !localHealth {
			fail(w, 403, "请通过授权入口访问")
			return
		}
		clean := path.Clean(r.URL.Path)
		if p.External && (clean == "/records.html" || clean == "/records.js" || clean == "/api/records" || strings.HasPrefix(clean, "/api/records/") || clean == "/api/access-records" || strings.HasPrefix(clean, "/api/access-records/")) {
			fail(w, 404, "页面不存在")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, p)))
	})
}
func canReadJob(r *http.Request, j Job) bool {
	p := identity(r)
	return !p.External || j.Owner == p.User
}

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExternalWebKeepsAPIsAndLegacyRollback(t *testing.T) {
	a := testApp(t, UnconfiguredRunner{})
	a.externalWeb = true
	for _, path := range []string{"/", "/index.html", "/style.css", "/records.html", "/app.js"} {
		if w := gatewayRequest(a, "GET", path, ""); w.Code != 404 {
			t.Fatalf("static remains served: %s=%d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/health", "/api/records", "/api/access-records"} {
		if w := gatewayRequest(a, "GET", path, ""); w.Code != 200 {
			t.Fatalf("backend endpoint lost: %s=%d", path, w.Code)
		}
	}
	a.externalWeb = false
	if w := gatewayRequest(a, "GET", "/", ""); w.Code != 200 {
		t.Fatal("legacy rollback page unavailable")
	}
}

func TestStaticPageAuditMaintainsIdentityAndPrivateRecords(t *testing.T) {
	a := testApp(t, testRunner{})
	a.gatewayToken = strings.Repeat("p", 48)
	a.externalWeb = true
	request := func(page, scope, token string) int {
		r := httptest.NewRequest("GET", "https://example.test:18443/api/page-view", nil)
		r.Header.Set("X-Aigc-Gateway", token)
		r.Header.Set("X-Aigc-Client-IP", "203.0.113.11")
		r.Header.Set("X-Aigc-Scope", scope)
		r.Header.Set("X-Aigc-User", "fixture")
		r.Header.Set("X-Aigc-Page", page)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		return w.Code
	}
	if code := request("/chaoxing/?credential=must-not-be-recorded", "external", a.gatewayToken); code != 204 {
		t.Fatalf("page audit=%d", code)
	}
	var n int
	if err := a.store.db.QueryRow("SELECT count(*) FROM access_events WHERE user='fixture' AND path='/' AND method='GET' AND status=200").Scan(&n); err != nil || n != 1 {
		t.Fatalf("static page missing or duplicated: %d %v", n, err)
	}
	for _, test := range []struct {
		page, scope, token string
		want               int
	}{
		{"/records.html", "external", a.gatewayToken, 403},
		{"/chaoxing/api/health", "external", a.gatewayToken, 403},
		{"/chaoxing/../records.html", "external", a.gatewayToken, 403},
		{"//attacker.test/", "external", a.gatewayToken, 403},
		{"/", "external", "invalid", 403},
		{"", "external", a.gatewayToken, 403},
		{"/chaoxing/records.html", "internal", a.gatewayToken, 204},
	} {
		if code := request(test.page, test.scope, test.token); code != test.want {
			t.Fatalf("audit boundary=%d want=%d", code, test.want)
		}
	}
}

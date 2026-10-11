package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRevocationAndHealthBoundary(t *testing.T) {
	a := testApp(t, testRunner{})
	a.gatewayToken = strings.Repeat("z", 48)
	a.publicUsersFile = filepath.Join(t.TempDir(), "users.json")
	data, _ := json.Marshal(map[string]int64{"alice": time.Now().Unix() + 3600, "expired": time.Now().Unix() - 1})
	os.WriteFile(a.publicUsersFile, data, 0600)
	if gatewayRequest(a, "GET", "/api/health", "alice").Code != 200 {
		t.Fatal("active user rejected")
	}
	for _, user := range []string{"expired", "unknown"} {
		if gatewayRequest(a, "GET", "/api/health", user).Code != 403 {
			t.Fatal("inactive user admitted")
		}
	}
	os.WriteFile(a.publicUsersFile, []byte("{}"), 0600)
	if gatewayRequest(a, "GET", "/api/health", "alice").Code != 403 {
		t.Fatal("revoked connection admitted")
	}
	for addr, want := range map[string]int{"127.0.0.1:1234": 200, "192.168.1.11:1234": 403} {
		r := httptest.NewRequest("GET", "/api/health", nil)
		r.RemoteAddr = addr
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("health %s=%d", addr, w.Code)
		}
	}
}

func gatewayRequest(a *app, method, path, user string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("X-Aigc-Gateway", a.gatewayToken)
	r.Header.Set("X-Aigc-Client-IP", "203.0.113.11")
	scope := "external"
	if user == "" {
		scope = "internal"
	}
	r.Header.Set("X-Aigc-Scope", scope)
	r.Header.Set("X-Aigc-User", user)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}
func gatewayUpload(t *testing.T, a *app, user, key string) Job {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "sample.docx")
	part.Write(docx("private content"))
	writer.Close()
	r := httptest.NewRequest("POST", "/api/jobs", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Aigc-Gateway", a.gatewayToken)
	r.Header.Set("X-Aigc-Client-IP", "203.0.113.11")
	r.Header.Set("X-Aigc-Scope", "external")
	r.Header.Set("X-Aigc-User", user)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return parseJob(t, w)
}
func TestGatewayCannotBeSpoofed(t *testing.T) {
	a := testApp(t, testRunner{})
	a.gatewayToken = strings.Repeat("x", 48)
	for _, token := range []string{"", "wrong"} {
		r := httptest.NewRequest("GET", "/api/records", nil)
		r.Header.Set("X-Aigc-Gateway", token)
		r.Header.Set("X-Aigc-Scope", "internal")
		r.Header.Set("X-Aigc-Client-IP", "192.168.1.22")
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("spoof=%d", w.Code)
		}
	}
	for _, p := range []string{"/records.html", "/records.js", "/api/records", "/api/%72ecords", "/api/records/", "/api/access-records", "/api/%61ccess-records", "/api/access-records/"} {
		if w := gatewayRequest(a, "GET", p, "alice"); w.Code != 404 {
			t.Fatalf("%s=%d", p, w.Code)
		}
	}
	if w := gatewayRequest(a, "GET", "/api/records", ""); w.Code != 200 {
		t.Fatalf("internal=%d", w.Code)
	}
	var health map[string]any
	w := gatewayRequest(a, "GET", "/api/health", "alice")
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["canViewRecords"] != false {
		t.Fatal("external records link exposed")
	}
}
func TestResultOwnershipAndScopedIdempotency(t *testing.T) {
	a := testApp(t, testRunner{})
	a.gatewayToken = strings.Repeat("y", 48)
	alice := gatewayUpload(t, a, "alice", "same-request-key")
	bob := gatewayUpload(t, a, "bob", "same-request-key")
	repeat := gatewayUpload(t, a, "alice", "same-request-key")
	if alice.ID == bob.ID || repeat.ID != alice.ID {
		t.Fatal("idempotency crossed identities")
	}
	j, err := a.store.get(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Owner != "alice" || j.ClientIP != "203.0.113.11" {
		t.Fatalf("audit=%+v", j)
	}
	if err := os.WriteFile(filepath.Join(a.dir, "reports", alice.ID+".report"), []byte("%PDF-1.7\nprivate report"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.store.finish(alice.ID, Result{Summary: "private result"}, true); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "/report"} {
		path := "/api/jobs/" + alice.ID + suffix
		if w := gatewayRequest(a, http.MethodGet, path, "bob"); w.Code != 404 {
			t.Fatalf("other user %s=%d", suffix, w.Code)
		}
		if w := gatewayRequest(a, http.MethodGet, path, "alice"); w.Code != 200 {
			t.Fatalf("owner %s=%d", suffix, w.Code)
		}
		if w := gatewayRequest(a, http.MethodGet, path, ""); w.Code != 200 {
			t.Fatalf("internal %s=%d", suffix, w.Code)
		}
	}
	w := gatewayRequest(a, "GET", "/api/records", "")
	if !strings.Contains(w.Body.String(), `"user":"alice"`) || !strings.Contains(w.Body.String(), `"clientIP":"203.0.113.11"`) {
		t.Fatal("missing audit identity")
	}
}
func TestExistingDatabaseMigrationRetainsReports(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "jobs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE jobs (
 id TEXT PRIMARY KEY,name TEXT NOT NULL,hash TEXT NOT NULL,key TEXT UNIQUE NOT NULL,status TEXT NOT NULL,
 message TEXT NOT NULL DEFAULT '',summary TEXT NOT NULL DEFAULT '',report INTEGER NOT NULL DEFAULT 0,
 created INTEGER NOT NULL,submission TEXT NOT NULL DEFAULT '',detection TEXT NOT NULL DEFAULT '');
 INSERT INTO jobs(id,name,hash,key,status,summary,report,created) VALUES('old-id','old.docx','hash','key','succeeded','retained',1,123);`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	j, err := s.get("old-id")
	if err != nil {
		t.Fatal(err)
	}
	if !j.ReportAvailable || j.Summary != "retained" || j.Owner != "" || j.ClientIP != "" {
		t.Fatalf("migration changed history=%+v", j)
	}
}

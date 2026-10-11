package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"

	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testRunner struct{ fn func(Job) (Result, error) }

func (testRunner) Ready() bool { return true }
func (r testRunner) Run(_ context.Context, j Job, _ string, advance func(string) error) (Result, error) {
	for _, stage := range []string{"submitting", "detecting", "waiting"} {
		if err := advance(stage); err != nil {
			return Result{}, err
		}
	}
	if r.fn != nil {
		return r.fn(j)
	}
	return Result{Summary: "测试结果 " + j.Name, SubmissionID: "submission-" + j.ID, DetectionID: "detection-" + j.ID, DocumentHash: j.Hash, Report: []byte("%PDF-1.7\n测试报告")}, nil
}
func testApp(t *testing.T, runner Runner) *app {
	t.Helper()
	a, err := newApp(t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.store.db.Close() })
	return a
}
func docx(text string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": "<document>" + text + "</document>"} {
		f, _ := z.Create(name)
		f.Write([]byte(body))
	}
	z.Close()
	return b.Bytes()
}
func uploadRequest(a *app, name, key string, data []byte) *httptest.ResponseRecorder {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	f, _ := writer.CreateFormFile("file", name)
	f.Write(data)
	writer.Close()
	r := httptest.NewRequest("POST", "/api/jobs", &b)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}
func parseJob(t *testing.T, w *httptest.ResponseRecorder) Job {
	t.Helper()
	if w.Code != 202 {
		t.Fatalf("upload=%d %s", w.Code, w.Body.String())
	}
	var j Job
	if err := json.Unmarshal(w.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	return j
}
func TestAnonymousPageAndHealth(t *testing.T) {
	a := testApp(t, UnconfiguredRunner{})
	for _, path := range []string{"/", "/style.css", "/app.js", "/api/health"} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
	w := uploadRequest(a, "a.docx", "request-0001", docx("one"))
	if w.Code != 503 {
		t.Fatalf("unconfigured=%d", w.Code)
	}
}
func TestUploadValidation(t *testing.T) {
	a := testApp(t, testRunner{})
	cases := []struct {
		name, key string
		data      []byte
		code      int
	}{
		{"a.doc", "request-0001", docx("one"), 400},
		{"a.docx", "request-0001", []byte("not zip"), 400},
		{"a.docx", "bad", docx("one"), 400},
		{"a.docx", "request-0001", make([]byte, maxUpload+1), 413},
	}
	for _, c := range cases {
		w := uploadRequest(a, c.name, c.key, c.data)
		if w.Code != c.code {
			t.Fatalf("%s: got %d want %d", c.name, w.Code, c.code)
		}
	}
	r := httptest.NewRequest("POST", "/api/jobs", nil)
	r.Header.Set("Origin", "https://other.example")
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross origin=%d", w.Code)
	}
}
func TestIdempotencyAndPrivateMetadata(t *testing.T) {
	a := testApp(t, testRunner{})
	data := docx("one")
	w := uploadRequest(a, "a.docx", "request-0001", data)
	j := parseJob(t, w)
	duplicate := parseJob(t, uploadRequest(a, "a.docx", "request-0001", data))
	if duplicate.ID != j.ID {
		t.Fatal("duplicate task created")
	}
	if uploadRequest(a, "a.docx", "request-0001", docx("two")).Code != 409 {
		t.Fatal("changed document accepted")
	}
	files, _ := os.ReadDir(filepath.Join(a.dir, "uploads"))
	if len(files) != 1 {
		t.Fatalf("orphan files: %d", len(files))
	}
	for _, private := range []string{"hash", "key", "submission", "detection", a.dir} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatalf("private metadata: %s", private)
		}
	}
}
func seedJob(t *testing.T, s *Store, i int) Job {
	t.Helper()
	j, _, err := s.create(Job{ID: fmt.Sprintf("%032x", i), Name: fmt.Sprintf("%d.docx", i), Hash: fmt.Sprint(i), Key: fmt.Sprint(i)})
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func TestQueueBoundAndExclusiveClaim(t *testing.T) {
	a := testApp(t, testRunner{})
	for i := 1; i <= 20; i++ {
		seedJob(t, a.store, i)
	}
	if _, _, err := a.store.create(Job{ID: "overflow", Key: "overflow"}); !errors.Is(err, errQueueFull) {
		t.Fatal(err)
	}
	first, err := a.store.claim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.store.claim(context.Background()); !errors.Is(err, errNotFound) {
		t.Fatal("second active claim", err)
	}
	if err = a.store.update(first.ID, "uncertain", "核对"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.store.claim(context.Background()); !errors.Is(err, errNotFound) {
		t.Fatal("uncertain did not pause", err)
	}
	paused, _ := a.store.paused()
	if !paused {
		t.Fatal("queue not paused")
	}
	if uploadRequest(a, "new.docx", "request-0001", docx("new")).Code != 503 {
		t.Fatal("paused upload accepted")
	}
}
func TestRestartKeepsQueuedAndPausesActive(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedJob(t, s, 1)
	second := seedJob(t, s, 2)
	first, _ := s.claim(context.Background())
	s.db.Close()
	s, err = openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if err = s.recover(); err != nil {
		t.Fatal(err)
	}
	recovered, _ := s.get(first.ID)
	queued, _ := s.get(second.ID)
	if recovered.Status != "uncertain" || queued.Status != "queued" {
		t.Fatalf("%s %s", recovered.Status, queued.Status)
	}
	if _, err = s.claim(context.Background()); !errors.Is(err, errNotFound) {
		t.Fatal("restart auto-submitted", err)
	}
}
func waitStatus(t *testing.T, s *Store, id, status string) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := s.get(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == status {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("worker timeout")
	return Job{}
}
func startTestWorker(t *testing.T, a *app) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runWorker(ctx, a.store, a.dir, a.runner) }()
	t.Cleanup(func() { cancel(); <-done })
}
func TestSeparateDocumentResultsAndReports(t *testing.T) {
	a := testApp(t, testRunner{})
	one := parseJob(t, uploadRequest(a, "one.docx", "request-0001", docx("one")))
	two := parseJob(t, uploadRequest(a, "two.docx", "request-0002", docx("two")))
	startTestWorker(t, a)
	one = waitStatus(t, a.store, one.ID, "succeeded")
	two = waitStatus(t, a.store, two.ID, "succeeded")
	if one.Hash == two.Hash || one.DetectionID == two.DetectionID || one.Summary == two.Summary {
		t.Fatal("results mixed")
	}
	for _, j := range []Job{one, two} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/jobs/"+j.ID+"/report", nil))
		if w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF")) {
			t.Fatal("missing report")
		}
	}
}
func TestWrongResultPausesNextSubmission(t *testing.T) {
	a := testApp(t, testRunner{fn: func(j Job) (Result, error) {
		return Result{Summary: "wrong", SubmissionID: "s", DetectionID: "d", DocumentHash: "other"}, nil
	}})
	first := parseJob(t, uploadRequest(a, "one.docx", "request-0001", docx("one")))
	second := parseJob(t, uploadRequest(a, "two.docx", "request-0002", docx("two")))
	startTestWorker(t, a)
	waitStatus(t, a.store, first.ID, "uncertain")
	if _, err := a.store.claim(context.Background()); !errors.Is(err, errNotFound) {
		t.Fatal("wrong result queue continued")
	}
	j, _ := a.store.get(second.ID)
	if j.Status != "queued" {
		t.Fatal("second document overwritten")
	}
}

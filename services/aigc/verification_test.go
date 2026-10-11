package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVerificationKeepsWebDisabled(t *testing.T) {
	p := &ChaoXingRunner{config: PlatformConfig{EnableAutomation: true}}
	dir := filepath.Join(t.TempDir(), "absent")
	if verifyPlatform(dir, p, []string{"sample.docx"}, &bytes.Buffer{}) == nil {
		t.Fatal("web-enabled configuration accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("rejected verification wrote data")
	}
}
func TestVerificationRejectsExistingQueueAndBadInput(t *testing.T) {
	a := testApp(t, testRunner{})
	if _, _, err := a.store.create(Job{ID: "11111111111111111111111111111111", Name: "waiting.docx", Hash: "hash", Key: "key"}); err != nil {
		t.Fatal(err)
	}
	if verifyPlatform(a.dir, &ChaoXingRunner{}, []string{"missing.docx"}, &bytes.Buffer{}) == nil {
		t.Fatal("existing queue accepted")
	}
	empty := t.TempDir()
	if verifyPlatform(empty, &ChaoXingRunner{}, []string{"missing.docx"}, &bytes.Buffer{}) == nil {
		t.Fatal("invalid document accepted")
	}
	s, err := openStore(empty)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	var count int
	if s.db.QueryRow("SELECT count(*) FROM jobs").Scan(&count) != nil || count != 0 {
		t.Fatal("invalid input created jobs")
	}
}

func TestRecoveryDoesNotResumeUncertainOrCompletedJobs(t *testing.T) {
	a := testApp(t, testRunner{})
	id := "22222222222222222222222222222222"
	if _, _, err := a.store.create(Job{ID: id, Name: "sample.docx", Hash: "hash", Key: "second"}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.update(id, "uncertain", ""); err != nil {
		t.Fatal(err)
	}
	if verifyQueuedPlatform(a.dir, &ChaoXingRunner{}, []string{id}, &bytes.Buffer{}) == nil {
		t.Fatal("uncertain submission retried")
	}
	if err := a.store.update(id, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	if collectPlatformResult(a.dir, &ChaoXingRunner{}, id, &bytes.Buffer{}) == nil {
		t.Fatal("completed result replaced")
	}
	j, err := a.store.get(id)
	if err != nil || j.Status != "succeeded" {
		t.Fatal("rejection changed task")
	}
}

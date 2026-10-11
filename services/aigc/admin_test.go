package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolutionEndsOnlyCheckedJob(t *testing.T) {
	a := testApp(t, UnconfiguredRunner{})
	first := strings.Repeat("a", 32)
	next := strings.Repeat("b", 32)
	for _, id := range []string{first, next} {
		if _, _, err := a.store.create(Job{ID: id, Name: id + ".docx", Hash: id, Key: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.store.claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.store.recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.claim(context.Background()); err == nil {
		t.Fatal("uncertain job did not pause queue")
	}
	note := "平台无进行中的检测，原报告已另存；private.example/fixture"
	if err := a.store.endUncertain(first, note); err != nil {
		t.Fatal(err)
	}
	job, _ := a.store.get(first)
	data, _ := json.Marshal(job)
	if job.Status != "failed" || job.Message != resolvedMessage || bytes.Contains(data, []byte(note)) {
		t.Fatal("resolution exposed private note or requeued job", string(data))
	}
	var saved string
	if err := a.store.db.QueryRow("SELECT note FROM job_resolutions WHERE job_id=?", first).Scan(&saved); err != nil || saved != note {
		t.Fatal("private audit missing", err)
	}
	if err := a.store.endUncertain(first, note); err == nil {
		t.Fatal("already resolved job changed again")
	}
	claimed, err := a.store.claim(context.Background())
	if err != nil || claimed.ID != next {
		t.Fatal("next job not resumed", err)
	}
}

func TestResolutionRejectsRunningAndOtherStates(t *testing.T) {
	a := testApp(t, UnconfiguredRunner{})
	uncertain := strings.Repeat("c", 32)
	active := strings.Repeat("d", 32)
	for _, id := range []string{uncertain, active} {
		if _, _, err := a.store.create(Job{ID: id, Name: id, Hash: id, Key: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.store.endUncertain(uncertain, "checked"); err == nil {
		t.Fatal("queued job resolved")
	}
	if err := a.store.update(uncertain, "uncertain", "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.update(active, "waiting", "fixture"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{active, uncertain, strings.Repeat("e", 32)} {
		if err := a.store.endUncertain(id, "checked"); err == nil {
			t.Fatal("active or missing job allowed resolution")
		}
	}
	if err := a.store.update(active, "failed", "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.update(uncertain, "succeeded", "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.endUncertain(uncertain, "checked"); err == nil {
		t.Fatal("completed job resolved")
	}
}

func TestResolutionAuditFailureKeepsQueuePaused(t *testing.T) {
	a := testApp(t, UnconfiguredRunner{})
	id := strings.Repeat("f", 32)
	if _, _, err := a.store.create(Job{ID: id, Name: id, Hash: id, Key: id}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.update(id, "uncertain", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.db.Exec("CREATE TRIGGER reject_resolution BEFORE INSERT ON job_resolutions BEGIN SELECT RAISE(ABORT, 'fixture'); END;"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.endUncertain(id, "checked"); err == nil {
		t.Fatal("audit failure was ignored")
	}
	job, _ := a.store.get(id)
	paused, _ := a.store.paused()
	if job.Status != "uncertain" || !paused {
		t.Fatal("failed resolution unpaused queue")
	}
}

func TestQueueCommandRequiresExplicitConfirmation(t *testing.T) {
	dir := t.TempDir()
	id := strings.Repeat("1", 32)
	var output bytes.Buffer
	for _, args := range [][]string{
		{"--resolve-job", id, "--note", "checked"},
		{"--resolve-job", id, "--confirm-platform-checked"},
		{"--resolve-job", "invalid", "--note", "checked", "--confirm-platform-checked"},
		{"--queue-status", "--resolve-job", id},
	} {
		if err := runQueueCommand(dir, args, &output); err == nil {
			t.Fatal("invalid confirmation accepted")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "jobs.sqlite")); !os.IsNotExist(err) {
		t.Fatal("invalid command created database")
	}
	store, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.create(Job{ID: id, Name: "fixture.docx", Hash: id, Key: id}); err != nil {
		t.Fatal(err)
	}
	if err = store.update(id, "uncertain", "fixture"); err != nil {
		t.Fatal(err)
	}
	store.db.Close()
	if err = runQueueCommand(dir, []string{"--queue-status"}, &output); err != nil || !strings.Contains(output.String(), id) {
		t.Fatal("queue status failed", err)
	}
	output.Reset()
	if err = runQueueCommand(dir, []string{"--resolve-job", id, "--note", "private audit fixture", "--confirm-platform-checked"}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "private audit") || !strings.Contains(output.String(), `"paused":false`) {
		t.Fatal("resolution output leaked note or retained pause", output.String())
	}
}

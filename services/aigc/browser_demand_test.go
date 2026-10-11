package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserLeaseBlocksIdleStopAndReleases(t *testing.T) {
	d := t.TempDir()
	os.Mkdir(filepath.Join(d, "account"), 0700)
	release, err := browserLease(d, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = browserLease(d, "manager"); err == nil {
		t.Fatal("idle stop acquired active worker lease")
	}
	resetWorkerLease(d)
	manager, err := browserLease(d, "manager")
	if err != nil {
		t.Fatal(err)
	}
	resetWorkerLease(d)
	if _, err = browserLease(d, "worker"); err == nil {
		t.Fatal("startup removed manager lease")
	}
	manager()
	release()
	next, err := browserLease(d, "worker")
	if err != nil {
		t.Fatal(err)
	}
	next()
}
func TestSleepingBrowserAcceptsQueueButMaintenanceBlocksIt(t *testing.T) {
	d := t.TempDir()
	os.Mkdir(filepath.Join(d, "account"), 0700)
	c := platformFixture()
	c.EnableAutomation = true
	r := &ChaoXingRunner{config: c, dir: d, onDemand: true}
	r.ready.Store(true)
	for state, want := range map[string]bool{"sleeping": true, "starting": true, "running": true, "login": false, "stopping": false, "error": false} {
		os.WriteFile(filepath.Join(d, "account", "browser-status.json"), []byte("{\"state\":\""+state+"\"}"), 0600)
		if r.Ready() != want {
			t.Fatal(state)
		}
	}
}
func TestBrowserWakeWaitIsCancellable(t *testing.T) {
	c := platformFixture()
	c.CDPURL = "http://127.0.0.1:1"
	r := &ChaoXingRunner{config: c}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if r.waitBrowser(ctx) == nil {
		t.Fatal("unavailable browser accepted")
	}
}

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeviceAccessAuditPrivacyAndRetention(t *testing.T) {
	a := testApp(t, testRunner{})
	a.gatewayToken = strings.Repeat("a", 48)
	if w := gatewayRequest(a, "GET", "/?token=must-not-be-recorded", "alice"); w.Code != 200 {
		t.Fatalf("page=%d", w.Code)
	}
	if w := gatewayRequest(a, "GET", "/api/health", "alice"); w.Code != 200 {
		t.Fatal("health")
	}
	var n int
	if err := a.store.db.QueryRow("SELECT count(*) FROM access_events").Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit count=%d error=%v", n, err)
	}
	w := gatewayRequest(a, "GET", "/api/access-records", "")
	var data struct {
		Records []AccessRecord
		Total   int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil || w.Code != 200 || data.Total != 1 {
		t.Fatalf("records=%s %v", w.Body, err)
	}
	if v := data.Records[0]; v.User != "alice" || v.IP != "203.0.113.11" || v.Path != "/" || v.Status != 200 {
		t.Fatalf("audit=%+v", v)
	}
	if strings.Contains(w.Body.String(), "must-not-be-recorded") {
		t.Fatal("query credentials stored")
	}
	if w := gatewayRequest(a, "GET", "/api/access-records", "alice"); w.Code != 404 {
		t.Fatalf("external=%d", w.Code)
	}
	if _, err := a.store.db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<10001)
INSERT INTO access_events(created,user,ip,method,path,status) SELECT 0,'','','GET','/',200 FROM n`); err != nil {
		t.Fatal(err)
	}
	gatewayRequest(a, "GET", "/", "alice")
	if err := a.store.db.QueryRow("SELECT count(*) FROM access_events").Scan(&n); err != nil || n != 10000 {
		t.Fatalf("retention=%d error=%v", n, err)
	}
}

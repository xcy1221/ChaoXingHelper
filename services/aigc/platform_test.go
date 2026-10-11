package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func platformFixture() PlatformConfig {
	return PlatformConfig{
		CDPURL: "http://chromium:9222", CPI: "11", CourseID: "1", ClassID: "2", WorkID: "3", AnswerID: "4",
		QuestionID: "5", StemID: "6", StudentNumber: "7",
		StudentURL: "https://mooc1.chaoxing.com/mooc-ans/mooc2/work/dowork?cpi=11&courseId=1&classId=2&workId=3&answerId=4",
		ReviewURL:  "https://mooc2-ans.chaoxing.com/mooc2-ans/work/library/review-work?courseid=1&clazzid=2&workId=3&workAnswerId=4",
	}
}
func TestPlatformBindings(t *testing.T) {
	c := platformFixture()
	if err := validatePlatformConfig(c); err != nil {
		t.Fatal(err)
	}
	cases := []PlatformConfig{c, c, c, c, c}
	cases[0].StudentURL = strings.Replace(c.StudentURL, "answerId=4", "answerId=9", 1)
	cases[1].ReviewURL = strings.Replace(c.ReviewURL, "mooc2-ans.chaoxing.com", "attacker.example", 1)
	cases[2].CDPURL = "http://8.8.8.8:9222"
	cases[3].StudentURL += "&courseId=1"
	cases[4].CDPURL = "http://user:password@localhost:9222"
	for _, bad := range cases {
		if validatePlatformConfig(bad) == nil {
			t.Fatal("invalid binding accepted")
		}
	}
	r := &ChaoXingRunner{config: c}
	r.ready.Store(true)
	if r.Ready() {
		t.Fatal("read-only config enabled automation")
	}
}
func TestDetectionRequiresFreshCompleteBatch(t *testing.T) {
	c := platformFixture()
	row := DetectionRow{ID: "duplicate11-5-2-10000", Question: "5", Similarity: "0.33%", AIGC: "0%",
		ReportURL: "https://dsa.dayainfo.com/dsa/third/selfDB/getAigcReport?sn=fixture"}
	if !completeDetection(row, c, nil, 9999) {
		t.Fatal("complete row rejected")
	}
	variants := []DetectionRow{row, row, row, row, row, row, row, row}
	variants[0].AIGC = ""
	variants[1].ReportURL = ""
	variants[2].ID = "duplicate11-5-2-9000"
	variants[3].ID = "duplicate12-5-2-10000"
	variants[4].ReportURL = "https://attacker.example/dsa/third/selfDB/getAigcReport?sn=fixture"
	variants[5].AIGC = "100.01%"
	variants[6].ID = "duplicate11-9-2-10000"
	variants[7].ID = "duplicate11-5-2-10000x"
	for _, pending := range variants {
		if completeDetection(pending, c, nil, 9999) {
			t.Fatal("stale/incomplete report accepted", pending)
		}
	}
	if completeDetection(row, c, map[string]bool{row.ID: true}, 9999) {
		t.Fatal("previous batch reused")
	}
}
func TestDetectionUsesCurrentSubmissionWindow(t *testing.T) {
	c := platformFixture()
	row := DetectionRow{ID: "duplicate11-5-2-1200", Question: "5", Similarity: "0%", AIGC: "0%",
		ReportURL: "https://dsa.dayainfo.com/dsa/third/selfDB/getAigcReport?sn=fixture"}
	// 平台批次可能早于本机教师端点击时间，但必须晚于本次提交前的边界。
	if !completeDetection(row, c, nil, 1000) {
		t.Fatal("current submission batch rejected")
	}
	if completeDetection(row, c, nil, 1600) {
		t.Fatal("teacher-click fence did not reproduce platform clock mismatch")
	}
	if completeDetection(row, c, map[string]bool{row.ID: true}, 1000) {
		t.Fatal("old batch accepted inside current time window")
	}
	row.ID = "duplicate11-5-2-999"
	if completeDetection(row, c, nil, 1000) {
		t.Fatal("pre-submission batch accepted")
	}
}

func reportZip(t *testing.T, name, data string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestReportZIPDownload(t *testing.T) {
	data := reportZip(t, "检测结果报告.pdf", "%PDF-1.7\nfixture")
	extension, contentType, err := reportFormat(data)
	if err != nil || extension != ".zip" || contentType != "application/zip" {
		t.Fatal("zip rejected", err)
	}
	a := testApp(t, testRunner{fn: func(j Job) (Result, error) {
		return Result{Summary: "zip", SubmissionID: "s", DetectionID: "d", DocumentHash: j.Hash, Report: data}, nil
	}})
	j := parseJob(t, uploadRequest(a, "doc.docx", "zip-request-01", docx("zip")))
	startTestWorker(t, a)
	waitStatus(t, a.store, j.ID, "succeeded")
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/jobs/"+j.ID+"/report", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" ||
		!strings.HasSuffix(w.Header().Get("Content-Disposition"), ".zip\"") || !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("zip download changed", w.Code, w.Header())
	}
	for _, bad := range [][]byte{
		[]byte("<html>login required</html>"),
		reportZip(t, "report.pdf", "not a PDF"),
		reportZip(t, "../report.pdf", "%PDF-1.7"),
		reportZip(t, "script.html", "<html/>"),
	} {
		if _, _, err = reportFormat(bad); err == nil {
			t.Fatal("invalid report accepted")
		}
	}
}
func TestAttachmentIdentity(t *testing.T) {
	one := []Attachment{{ObjectID: "first", Name: "first.docx"}}
	if sameAttachments(one, []Attachment{{ObjectID: "second", Name: "first.docx"}}) ||
		sameAttachments(one, append(one, Attachment{ObjectID: "second", Name: "second.docx"})) ||
		sameAttachments(nil, nil) {
		t.Fatal("unconfirmed attachment accepted")
	}
	if !sameAttachments(one, one) {
		t.Fatal("same attachment rejected")
	}
}
func TestBadReportPausesWorker(t *testing.T) {
	a := testApp(t, testRunner{fn: func(j Job) (Result, error) {
		return Result{Summary: "bad", SubmissionID: "s", DetectionID: "d", DocumentHash: j.Hash, Report: []byte("<html>")}, nil
	}})
	one := parseJob(t, uploadRequest(a, "one.docx", "report-request-1", docx("one")))
	two := parseJob(t, uploadRequest(a, "two.docx", "report-request-2", docx("two")))
	startTestWorker(t, a)
	waitStatus(t, a.store, one.ID, "uncertain")
	if _, err := a.store.claim(context.Background()); err == nil {
		t.Fatal("next job claimed")
	}
	j, err := a.store.get(two.ID)
	if err != nil || j.Status != "queued" {
		t.Fatal(fmt.Sprint(j, err))
	}
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
)

//go:embed platform/adapter.js
var platformAdapter string

type PlatformConfig struct {
	CDPURL           string `json:"cdpURL"`
	StudentURL       string `json:"studentURL"`
	ReviewURL        string `json:"reviewURL"`
	CPI              string `json:"cpi"`
	CourseID         string `json:"courseId"`
	ClassID          string `json:"classId"`
	WorkID           string `json:"workId"`
	AnswerID         string `json:"answerId"`
	QuestionID       string `json:"questionId"`
	StemID           string `json:"stemId"`
	StudentNumber    string `json:"studentNumber"`
	UploadDir        string `json:"uploadDir"`
	EnableAutomation bool   `json:"enableAutomation"`
}

var numericID = regexp.MustCompile("^[0-9]{1,20}$")
var percent = regexp.MustCompile("^(?:100(?:\\.0+)?|[0-9]{1,2}(?:\\.[0-9]+)?)%$")

func loadPlatformConfig(path string) (PlatformConfig, error) {
	var c PlatformConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return c, errors.New("无法读取私有平台配置")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil {
		return c, errors.New("平台配置格式无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return c, errors.New("平台配置包含多余内容")
	}
	return c, validatePlatformConfig(c)
}
func validatePlatformConfig(c PlatformConfig) error {
	for _, id := range []string{c.CPI, c.CourseID, c.ClassID, c.WorkID, c.AnswerID, c.QuestionID, c.StemID, c.StudentNumber} {
		if !numericID.MatchString(id) {
			return errors.New("平台绑定标识无效")
		}
	}
	for _, spec := range []struct {
		raw, path string
		hosts     []string
		ids       map[string]string
	}{
		{c.StudentURL, "/mooc-ans/mooc2/work/dowork", []string{"mooc1.chaoxing.com", "mooc1-api.chaoxing.com"}, map[string]string{"cpi": c.CPI, "courseId": c.CourseID, "classId": c.ClassID, "workId": c.WorkID, "answerId": c.AnswerID}},
		{c.ReviewURL, "/mooc2-ans/work/library/review-work", []string{"mooc2-ans.chaoxing.com"}, map[string]string{"courseid": c.CourseID, "clazzid": c.ClassID, "workId": c.WorkID, "workAnswerId": c.AnswerID}},
	} {
		u, err := url.Parse(spec.raw)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Path != spec.path || u.Fragment != "" {
			return errors.New("平台页面地址无效")
		}
		ok := false
		for _, host := range spec.hosts {
			if u.Hostname() == host {
				ok = true
			}
		}
		if !ok {
			return errors.New("平台域名不符")
		}
		for k, v := range spec.ids {
			if len(u.Query()[k]) != 1 || u.Query().Get(k) != v {
				return errors.New("平台地址与绑定不符")
			}
		}
	}
	u, err := url.Parse(c.CDPURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "ws") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("浏览器控制地址无效")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && host != "chromium" && host != controlAlias && (ip == nil || (!ip.IsLoopback() && !ip.IsPrivate())) {
		return errors.New("浏览器控制端口必须使用私有地址")
	}
	if c.UploadDir != "" && !filepath.IsAbs(c.UploadDir) && !path.IsAbs(c.UploadDir) {
		return errors.New("浏览器上传目录必须为绝对路径")
	}
	return nil
}

type Attachment struct {
	ObjectID string `json:"objectId"`
	Name     string `json:"name"`
	HTML     string `json:"html"`
}
type DetectionRow struct {
	ID         string `json:"id"`
	Question   string `json:"question"`
	Similarity string `json:"similarity"`
	AIGC       string `json:"aigc"`
	ReportURL  string `json:"reportURL"`
}
type PlatformSnapshot struct {
	Attachments   []Attachment   `json:"attachments"`
	Rows          []DetectionRow `json:"rows"`
	Time          string         `json:"time"`
	ResultVisible bool           `json:"resultVisible"`
	HTML          string         `json:"html"`
}
type ChaoXingRunner struct {
	config   PlatformConfig
	dir      string
	mu       sync.Mutex
	ready    atomic.Bool
	onDemand bool
}

func (r *ChaoXingRunner) Ready() bool {
	state := r.browserState()
	return r.config.EnableAutomation && r.ready.Load() && state != "error" && state != "login" && state != "stopping"
}
func platformCall(ctx context.Context, method string, c PlatformConfig, out any, args ...any) error {
	values := []any{c}
	values = append(values, args...)
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	expression := "(" + strings.TrimSpace(platformAdapter) + ")." + method + "(..." + string(raw) + ")"
	return chromedp.Run(ctx, chromedp.Evaluate(expression, out))
}
func platformTab(parent context.Context, address string) (context.Context, context.CancelFunc, error) {
	ctx, cancel := chromedp.NewContext(parent)
	err := chromedp.Run(ctx, chromedp.Navigate(address), chromedp.WaitReady("body", chromedp.ByQuery))
	return ctx, cancel, err
}
func (r *ChaoXingRunner) session(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if r.onDemand {
		if err := r.waitBrowser(ctx); err != nil {
			return nil, func() {}, err
		}
	}
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(ctx, r.config.CDPURL)
	root, cancelRoot := chromedp.NewContext(alloc)
	err := chromedp.Run(root)
	return root, func() { cancelRoot(); cancelAlloc() }, err
}
func waitStudent(ctx context.Context, c PlatformConfig) error {
	expression := "(()=>{try{(" + strings.TrimSpace(platformAdapter) + ").student(" + string(mustJSON(c)) + ");return true;}catch{return false;}})()"
	return chromedp.Run(ctx, chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(30*time.Second), chromedp.WithPollingInterval(250*time.Millisecond)))
}
func until(ctx context.Context, interval time.Duration, check func() (bool, error)) error {
	for {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (r *ChaoXingRunner) openReview(ctx context.Context) error {
	if err := platformCall(ctx, "openResults", r.config, nil); err != nil {
		return err
	}
	if err := until(ctx, 200*time.Millisecond, func() (bool, error) {
		var ready bool
		err := platformCall(ctx, "resultsReady", r.config, &ready)
		return ready, err
	}); err != nil {
		return err
	}
	return platformCall(ctx, "openDaya", r.config, nil)
}
func (r *ChaoXingRunner) probe(ctx context.Context) (PlatformSnapshot, error) {
	root, closeRoot, err := r.session(ctx)
	defer closeRoot()
	if err != nil {
		return PlatformSnapshot{}, errors.New("专用浏览器无法连接")
	}
	student, closeStudent, err := platformTab(root, r.config.StudentURL)
	defer closeStudent()
	if err != nil {
		return PlatformSnapshot{}, errors.New("学生作答页无法访问")
	}
	if err = waitStudent(student, r.config); err != nil {
		return PlatformSnapshot{}, errors.New("作答编辑器未就绪")
	}
	var current PlatformSnapshot
	if err = platformCall(student, "student", r.config, &current); err != nil {
		return current, errors.New("学生作答页核验失败")
	}
	review, closeReview, err := platformTab(root, r.config.ReviewURL)
	defer closeReview()
	if err != nil {
		return current, errors.New("教师批阅页无法访问")
	}
	var saved PlatformSnapshot
	if err = platformCall(review, "review", r.config, &saved); err != nil {
		return saved, errors.New("教师批阅对象核验失败")
	}
	if !sameAttachments(current.Attachments, saved.Attachments) {
		return saved, errors.New("学生答案与教师端不一致")
	}
	if err = r.openReview(review); err != nil {
		return saved, errors.New("查重浮窗无法打开")
	}
	err = until(ctx, time.Second, func() (bool, error) {
		if err := platformCall(review, "review", r.config, &saved); err != nil {
			return false, err
		}
		return saved.ResultVisible, nil
	})
	if err != nil {
		return saved, errors.New("查重结果未就绪")
	}
	if err != nil {
		return saved, errors.New("查重结果未就绪")
	}
	return saved, nil
}
func sameAttachments(a, b []Attachment) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ObjectID == "" || a[i].ObjectID != b[i].ObjectID || a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}
func (r *ChaoXingRunner) monitor(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	_, err := r.probe(probeCtx)
	r.ready.Store(err == nil)
}
func completeDetection(row DetectionRow, c PlatformConfig, before map[string]bool, after int64) bool {
	parts := strings.Split(row.ID, "-")
	if len(parts) != 4 || parts[0] != "duplicate"+c.CPI || row.Question != c.QuestionID || parts[1] != c.QuestionID || parts[2] != c.ClassID || before[row.ID] {
		return false
	}
	// 批次时间与旧批次均需核对；AIGC 空白不能当作 0%。
	stamp, parseErr := strconv.ParseInt(parts[3], 10, 64)
	if parseErr != nil || stamp < after || !percent.MatchString(row.AIGC) || !percent.MatchString(row.Similarity) {
		return false
	}
	return validReportURL(row.ReportURL)
}
func validReportURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "dsa.dayainfo.com" && u.User == nil &&
		u.Path == "/dsa/third/selfDB/getAigcReport" && u.Fragment == "" && u.Query().Get("sn") != ""
}
func browserReport(ctx context.Context, raw string) ([]byte, error) {
	var userAgent string
	if err := chromedp.Run(ctx, chromedp.Evaluate("navigator.userAgent", &userAgent)); err != nil {
		return nil, errors.New("报告浏览器信息不可用")
	}
	return downloadPlatformReport(ctx, raw, userAgent)
}
func downloadPlatformReport(ctx context.Context, raw, userAgent string) ([]byte, error) {
	if userAgent == "" || len(userAgent) > 512 || strings.ContainsAny(userAgent, "\r\n") {
		return nil, errors.New("报告浏览器信息无效")
	}

	if !validReportURL(raw) {
		return nil, errors.New("报告地址不符")
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !validReportURL(req.URL.String()) {
			return errors.New("报告重定向不符")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://mooc2-ans.chaoxing.com/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("报告下载失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("报告下载失败")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil {
		return nil, err
	}
	_, _, err = reportFormat(data)
	return data, err
}
func (r *ChaoXingRunner) Run(ctx context.Context, j Job, file string, advance func(string) error) (result Result, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() {
		if err != nil {
			r.ready.Store(false)
		}
	}()
	if !r.Ready() {
		return result, errors.New("平台执行器未启用")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != j.Hash {
		return result, errors.New("文档摘要不符")
	}
	root, closeRoot, err := r.session(ctx)
	defer closeRoot()
	if err != nil {
		return result, errors.New("专用浏览器无法连接")
	}
	review, closeReview, err := platformTab(root, r.config.ReviewURL)
	defer closeReview()
	if err != nil {
		return result, err
	}
	var originalReview PlatformSnapshot
	if err = platformCall(review, "review", r.config, &originalReview); err != nil {
		return result, err
	}
	if err = r.openReview(review); err != nil {
		return result, err
	}
	var before PlatformSnapshot
	if err = until(ctx, time.Second, func() (bool, error) {
		err := platformCall(review, "review", r.config, &before)
		return before.ResultVisible, err
	}); err != nil {
		return result, err
	}
	oldRows := map[string]bool{}
	for _, row := range before.Rows {
		oldRows[row.ID] = true
	}
	student, closeStudent, err := platformTab(root, r.config.StudentURL)
	defer closeStudent()
	if err != nil {
		return result, err
	}
	if err = waitStudent(student, r.config); err != nil {
		return result, errors.New("作答编辑器未就绪")
	}
	var old PlatformSnapshot
	if err = platformCall(student, "student", r.config, &old); err != nil {
		return result, err
	}
	if !sameAttachments(old.Attachments, originalReview.Attachments) {
		return result, errors.New("提交前双端答案不一致")
	}
	journal := filepath.Join(r.dir, "account")
	if err = os.MkdirAll(journal, 0700); err != nil {
		return result, err
	}
	snapshot, _ := json.Marshal(old)
	if err = os.WriteFile(filepath.Join(journal, j.ID+"-before.json"), snapshot, 0600); err != nil {
		return result, err
	}
	if err = platformCall(student, "openUpload", r.config, nil); err != nil {
		return result, err
	}
	remoteFile := file
	if r.config.UploadDir != "" {
		if path.IsAbs(r.config.UploadDir) {
			remoteFile = path.Join(r.config.UploadDir, filepath.Base(file))
		} else {
			remoteFile = filepath.Join(r.config.UploadDir, filepath.Base(file))
		}
	}
	selector := "(" + strings.TrimSpace(platformAdapter) + ").uploadInput(" + string(mustJSON(r.config)) + ")"
	if err = chromedp.Run(student,
		chromedp.Poll("(()=>{try{return !!("+selector+");}catch{return false;}})()", nil, chromedp.WithPollingTimeout(30*time.Second), chromedp.WithPollingInterval(250*time.Millisecond)),
		chromedp.SetUploadFiles(selector, []string{remoteFile}, chromedp.ByJSPath)); err != nil {
		return result, err
	}
	expectedName := filepath.Base(file)
	var uploaded Attachment
	uploadCtx, cancelUpload := context.WithTimeout(student, 2*time.Minute)
	err = until(uploadCtx, time.Second, func() (bool, error) {
		var current PlatformSnapshot
		if err := platformCall(uploadCtx, "student", r.config, &current); err != nil {
			return false, err
		}
		matches := 0
		for _, a := range current.Attachments {
			if a.Name == expectedName && a.ObjectID != "" {
				uploaded = a
				matches++
			}
		}
		return matches == 1, nil
	})
	cancelUpload()
	if err != nil {
		return result, errors.New("附件尚未插入，保留原答案并停止")
	}
	if err = platformCall(student, "keepOnly", r.config, nil, uploaded.ObjectID, expectedName); err != nil {
		return result, err
	}
	if err = advance("submitting"); err != nil {
		return result, err
	}
	if err = platformCall(student, "submit", r.config, nil, uploaded.ObjectID, expectedName); err != nil {
		return result, err
	}
	if err = chromedp.Run(student, chromedp.WaitVisible("#popok", chromedp.ByQuery)); err != nil {
		return result, err
	}
	// 平台批次使用平台时钟；以本次提交前为边界，并排除提交前已存在的批次。
	submittedAfter := time.Now().UnixMilli()
	if err = platformCall(student, "confirm", r.config, nil, uploaded.ObjectID, expectedName); err != nil {
		return result, err
	}
	// 不重试提交；重新读取教师端，必须只包含本次附件。
	if err = chromedp.Run(review, chromedp.Navigate(r.config.ReviewURL), chromedp.WaitReady("#personalInfor", chromedp.ByQuery)); err != nil {
		return result, err
	}
	var saved PlatformSnapshot
	verifyCtx, cancelVerify := context.WithTimeout(review, 45*time.Second)
	err = until(verifyCtx, 3*time.Second, func() (bool, error) {
		if err := platformCall(verifyCtx, "review", r.config, &saved); err != nil {
			return false, err
		}
		if sameAttachments([]Attachment{uploaded}, saved.Attachments) {
			return true, nil
		}
		err := chromedp.Run(verifyCtx, chromedp.Reload(), chromedp.WaitReady("#personalInfor", chromedp.ByQuery))
		return false, err
	})
	cancelVerify()
	if err != nil {
		return result, errors.New("教师端无法确认本次提交")
	}
	if err = r.openReview(review); err != nil {
		return result, err
	}
	if err = until(ctx, time.Second, func() (bool, error) {
		var current PlatformSnapshot
		err := platformCall(review, "review", r.config, &current)
		return current.ResultVisible, err
	}); err != nil {
		return result, err
	}
	if err = advance("detecting"); err != nil {
		return result, err
	}
	started := time.Now().UnixMilli()
	receipt := map[string]any{"submittedAfter": submittedAfter, "triggeredAfter": started, "previousBatches": oldRows}
	receiptPath := filepath.Join(journal, j.ID+"-detection.json")
	if err = os.WriteFile(receiptPath, mustJSON(receipt), 0600); err != nil {
		return result, err
	}
	if err = platformCall(review, "trigger", r.config, nil); err != nil {
		return result, err
	}
	if err = advance("waiting"); err != nil {
		return result, err
	}
	var finished DetectionRow
	err = until(ctx, 15*time.Second, func() (bool, error) {
		var current PlatformSnapshot
		if err := platformCall(review, "review", r.config, &current); err != nil {
			return false, err
		}
		if !sameAttachments([]Attachment{uploaded}, current.Attachments) {
			return false, errors.New("等待期间答案被更改")
		}
		receipt["observedAt"] = time.Now().UnixMilli()
		receipt["resultVisible"], receipt["rowCount"] = current.ResultVisible, len(current.Rows)
		if len(current.Rows) == 1 {
			row := current.Rows[0]
			receipt["batch"], receipt["aigc"], receipt["similarity"] = row.ID, row.AIGC, row.Similarity
			receipt["reportValid"] = validReportURL(row.ReportURL)
		}
		if err := os.WriteFile(receiptPath, mustJSON(receipt), 0600); err != nil {
			return false, err
		}
		if current.ResultVisible && len(current.Rows) == 1 && completeDetection(current.Rows[0], r.config, oldRows, submittedAfter) {
			finished = current.Rows[0]
			return true, nil
		}
		// 平台刷新会暂时清空 AIGC 并异步填回；先读完整结果，再请求刷新。
		err := platformCall(review, "refresh", r.config, nil)
		return false, err
	})
	if err != nil {
		return result, err
	}
	report, err := browserReport(review, finished.ReportURL)
	if err != nil {
		return result, err
	}
	return Result{Summary: "疑似 AIGC 占比 " + finished.AIGC + "；相似度 " + finished.Similarity,
		SubmissionID: r.config.AnswerID + ":" + uploaded.ObjectID, DetectionID: finished.ID, DocumentHash: j.Hash, Report: report}, nil
}
func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

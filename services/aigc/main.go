package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

//go:embed web/*
var webFiles embed.FS
var jobID = regexp.MustCompile("^[a-f0-9]{32}$")
var requestKey = regexp.MustCompile("^[a-zA-Z0-9_-]{8,80}$")

const maxUpload = int64(10 << 20)

type app struct {
	store           *Store
	dir             string
	runner          Runner
	uploadSlots     chan struct{}
	gatewayToken    string
	publicUsersFile string
}

func newApp(dir string, runner Runner) (*app, error) {
	for _, sub := range []string{"uploads", "reports"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			return nil, err
		}
	}
	s, err := openStore(dir)
	if err != nil {
		return nil, err
	}
	token, err := loadGatewayToken()
	if err != nil {
		s.db.Close()
		return nil, err
	}
	return &app{store: s, dir: dir, runner: runner, uploadSlots: make(chan struct{}, 2), gatewayToken: token, publicUsersFile: os.Getenv("AIGC_PUBLIC_USERS_FILE")}, nil
}
func jsonReply(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, code int, message string) {
	jsonReply(w, code, map[string]string{"error": message})
}
func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		paused, err := a.store.paused()
		if err != nil {
			fail(w, 500, "存储暂时不可用")
			return
		}
		health := map[string]any{"status": "ok", "ready": a.runner.Ready(), "paused": paused, "maxUploadBytes": maxUpload, "canViewRecords": !identity(r).External && identity(r).IP != ""}
		if platform, ok := a.runner.(*ChaoXingRunner); ok && platform.onDemand {
			health["browserState"] = platform.browserState()
		}
		jsonReply(w, 200, health)
	})
	mux.HandleFunc("POST /api/jobs", a.upload)
	mux.HandleFunc("GET /api/records", a.records)
	mux.HandleFunc("GET /api/access-records", a.accessRecords)
	mux.HandleFunc("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !jobID.MatchString(id) {
			fail(w, 404, "任务不存在")
			return
		}
		j, err := a.store.get(id)
		if errors.Is(err, errNotFound) {
			fail(w, 404, "任务不存在")
			return
		}
		if err != nil {
			fail(w, 500, "任务查询失败")
			return
		}
		if !canReadJob(r, j) {
			fail(w, 404, "任务不存在")
			return
		}
		jsonReply(w, 200, j)
	})
	mux.HandleFunc("GET /api/jobs/{id}/report", a.report)
	web, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(web)))
	return a.access(a.audit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method == "POST" && r.Header.Get("Origin") != "" {
			origin, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || !strings.EqualFold(origin.Host, r.Host) {
				fail(w, 403, "请求来源不匹配")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})))
}
func validateDocx(data []byte) bool {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) > 2000 {
		return false
	}
	var total uint64
	var content, document bool
	for _, f := range reader.File {
		if f.UncompressedSize64 > 64<<20-total {
			return false
		}
		total += f.UncompressedSize64
		stream, openErr := f.Open()
		if openErr != nil {
			return false
		}
		n, readErr := io.Copy(io.Discard, io.LimitReader(stream, int64(f.UncompressedSize64)+1))
		stream.Close()
		if readErr != nil || uint64(n) != f.UncompressedSize64 {
			return false
		}
		if f.Name == "[Content_Types].xml" {
			content = true
		}
		if f.Name == "word/document.xml" {
			document = true
		}
	}
	return content && document
}
func (a *app) upload(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		fail(w, 429, "上传繁忙，请稍后重试")
		return
	}
	if !a.runner.Ready() {
		fail(w, 503, "检测连接尚未就绪，请管理员接入学习通会话与检测流程")
		return
	}
	paused, pauseErr := a.store.paused()
	if pauseErr != nil || paused {
		fail(w, 503, "队列暂停，需核对上一份提交")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !requestKey.MatchString(key) {
		fail(w, 400, "缺少有效请求标识")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		fail(w, 413, "上传无效或超过 10 MB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "请选择 Word 文档")
		return
	}
	defer file.Close()
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if !strings.EqualFold(filepath.Ext(name), ".docx") {
		fail(w, 400, "当前支持 .docx 文档")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
	if err != nil || int64(len(data)) > maxUpload {
		fail(w, 413, "文档超过 10 MB")
		return
	}
	if !validateDocx(data) {
		fail(w, 400, "文件不是有效的 .docx 文档")
		return
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		fail(w, 500, "无法创建任务")
		return
	}
	id := hex.EncodeToString(random)
	hash := sha256.Sum256(data)
	p := identity(r)
	keyScope := key
	if p.External {
		keyScope = p.User + "\x00" + key
	}
	keyHash := sha256.Sum256([]byte(keyScope))
	j := Job{ID: id, Name: name, Hash: hex.EncodeToString(hash[:]), Key: hex.EncodeToString(keyHash[:]), Owner: p.User, ClientIP: p.IP}
	destination := filepath.Join(a.dir, "uploads", id+".docx")
	if err = os.WriteFile(destination, data, 0600); err != nil {
		fail(w, 500, "文件保存失败")
		return
	}
	saved, created, err := a.store.create(j)
	if err != nil || !created {
		os.Remove(destination)
	}
	if errors.Is(err, errConflict) {
		fail(w, 409, "请求标识已用于另一文档")
		return
	}
	if errors.Is(err, errQueueFull) {
		fail(w, 429, "队列已满，请稍后重试")
		return
	}
	if err != nil {
		fail(w, 500, "任务创建失败")
		return
	}
	jsonReply(w, 202, saved)
}
func (a *app) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !jobID.MatchString(id) {
		fail(w, 404, "报告不存在")
		return
	}
	j, err := a.store.get(id)
	if err != nil || !canReadJob(r, j) || j.Status != "succeeded" || !j.ReportAvailable {
		fail(w, 404, "报告尚不可用")
		return
	}
	f, err := os.Open(filepath.Join(a.dir, "reports", id+".report"))
	if errors.Is(err, os.ErrNotExist) {
		f, err = os.Open(filepath.Join(a.dir, "reports", id+".pdf"))
	}
	if err != nil {
		fail(w, 404, "报告不存在")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		fail(w, 500, "报告读取失败")
		return
	}
	header := make([]byte, 5)
	if _, err = f.ReadAt(header, 0); err != nil {
		fail(w, 500, "报告格式无效")
		return
	}
	extension, contentType := ".pdf", "application/pdf"
	if bytes.HasPrefix(header, []byte("PK\x03\x04")) {
		extension, contentType = ".zip", "application/zip"
	} else if !bytes.HasPrefix(header, []byte("%PDF-")) {
		fail(w, 500, "报告格式无效")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"report-%s%s\"", id, extension))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "report"+extension, info.ModTime(), f)
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--cdp-bridge" {
		if err := runCDPBridge(); err != nil {
			log.Fatal("浏览器控制中继启动失败")
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--cdp-healthcheck" {
		if !checkCDPBridge() {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		if !checkHealth() {
			os.Exit(1)
		}
		return
	}
	dir := os.Getenv("AIGC_DATA_DIR")
	if dir == "" {
		dir = "data"
	}
	if len(os.Args) > 1 && (os.Args[1] == "--queue-status" || os.Args[1] == "--resolve-job") {
		if err := runQueueCommand(dir, os.Args[1:], os.Stdout); err != nil {
			log.Print("队列管理失败：", err)
			os.Exit(2)
		}
		return
	}
	address := os.Getenv("AIGC_LISTEN")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	var runner Runner = UnconfiguredRunner{}
	if configPath := os.Getenv("AIGC_PLATFORM_CONFIG"); configPath != "" {
		config, configErr := loadPlatformConfig(configPath)
		if configErr != nil {
			log.Print("私有平台配置未就绪，检测已停用")
		}
		if configErr == nil {
			runner = &ChaoXingRunner{config: config, dir: dir, onDemand: os.Getenv("AIGC_BROWSER_ON_DEMAND") == "1"}
		}
	}
	if len(os.Args) > 1 && (os.Args[1] == "--collect-result" || os.Args[1] == "--verify-queued") {
		platform, ok := runner.(*ChaoXingRunner)
		if !ok {
			log.Fatal("私有平台配置未就绪")
		}
		var err error
		if os.Args[1] == "--collect-result" && len(os.Args) == 3 {
			err = collectPlatformResult(dir, platform, os.Args[2], os.Stdout)
		} else if os.Args[1] == "--verify-queued" {
			err = verifyQueuedPlatform(dir, platform, os.Args[2:], os.Stdout)
		} else {
			err = errors.New("管理员参数无效")
		}
		if err != nil {
			log.Fatal("管理员核验失败：", err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--verify-platform" {
		platform, ok := runner.(*ChaoXingRunner)
		if !ok {
			log.Fatal("私有平台配置未就绪")
		}
		if err := verifyPlatform(dir, platform, os.Args[2:], os.Stdout); err != nil {
			log.Fatal("管理员实测失败：", err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--inspect-platform" {
		platform, ok := runner.(*ChaoXingRunner)
		if !ok {
			log.Fatal("请通过 AIGC_PLATFORM_CONFIG 指定私有平台配置")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		snapshot, probeErr := platform.probe(ctx)
		if probeErr != nil {
			log.Fatal("平台核验失败：", probeErr)
		}
		// CLI 不输出签名 URL 或答案 HTML。
		for i := range snapshot.Attachments {
			snapshot.Attachments[i].HTML = ""
		}
		for i := range snapshot.Rows {
			snapshot.Rows[i].ReportURL = ""
		}
		snapshot.HTML = ""
		json.NewEncoder(os.Stdout).Encode(snapshot)
		return
	}
	a, err := newApp(dir, runner)
	if err != nil {
		log.Fatal("存储初始化失败")
	}
	defer a.store.db.Close()
	if err = a.store.recover(); err != nil {
		log.Fatal("恢复任务失败")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if platform, ok := runner.(*ChaoXingRunner); ok {
		if platform.onDemand {
			resetWorkerLease(dir)
		}
		if platform.config.EnableAutomation {
			if platform.onDemand {
				platform.ready.Store(true)
			} else {
				go platform.monitor(ctx)
			}
		}
	}
	go runWorker(ctx, a.store, a.dir, a.runner)
	server := &http.Server{Addr: address, Handler: a.routes(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("Local: http://%s (平台状态由健康接口提供)", address)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("HTTP 服务失败")
	}
}

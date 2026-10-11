package main

import (
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

type auditResponse struct {
	http.ResponseWriter
	status int
}

func (w *auditResponse) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *auditResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *auditResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (a *app) audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := &auditResponse{ResponseWriter: w}
		next.ServeHTTP(out, r)
		code := out.status
		if code == 0 {
			code = http.StatusOK
		}
		p := path.Clean(r.URL.Path)
		if code < 400 && r.Method == http.MethodGet && p != "/" && p != "/index.html" && !strings.HasSuffix(p, "/report") {
			return
		}
		who := identity(r)
		if len(p) > 512 {
			p = p[:512]
		}
		_, err := a.store.db.Exec(`INSERT INTO access_events(created,user,ip,method,path,status) VALUES(?,?,?,?,?,?);
DELETE FROM access_events WHERE id <= (SELECT max(id)-10000 FROM access_events);`, time.Now().Unix(), who.User, who.IP, r.Method, p, code)
		if err != nil {
			log.Printf("access audit failed: %v", err)
		}
	})
}

type AccessRecord struct {
	CreatedAt int64  `json:"createdAt"`
	User      string `json:"user"`
	IP        string `json:"clientIP"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
}

func (a *app) accessRecords(w http.ResponseWriter, r *http.Request) {
	if identity(r).External {
		fail(w, 404, "页面不存在")
		return
	}
	page := 1
	if q := r.URL.Query().Get("page"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 1000000 {
			fail(w, 400, "页码无效")
			return
		}
		page = n
	}
	var total int
	if err := a.store.db.QueryRow("SELECT count(*) FROM access_events").Scan(&total); err != nil {
		fail(w, 500, "记录查询失败")
		return
	}
	rows, err := a.store.db.Query("SELECT created,user,ip,method,path,status FROM access_events ORDER BY id DESC LIMIT 50 OFFSET ?", (page-1)*50)
	if err != nil {
		fail(w, 500, "记录查询失败")
		return
	}
	defer rows.Close()
	result := []AccessRecord{}
	for rows.Next() {
		var v AccessRecord
		if err := rows.Scan(&v.CreatedAt, &v.User, &v.IP, &v.Method, &v.Path, &v.Status); err != nil {
			fail(w, 500, "记录查询失败")
			return
		}
		if v.User == "" {
			v.User = "内网"
		}
		result = append(result, v)
	}
	if rows.Err() != nil {
		fail(w, 500, "记录查询失败")
		return
	}
	jsonReply(w, 200, map[string]any{"records": result, "total": total, "page": page, "size": 50})
}

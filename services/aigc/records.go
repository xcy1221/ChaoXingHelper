package main

import (
	"net/http"
	"strconv"
)

type Record struct {
	ID              string `json:"id"`
	Name            string `json:"fileName"`
	User            string `json:"user"`
	IP              string `json:"clientIP"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	Summary         string `json:"summary"`
	ReportAvailable bool   `json:"reportAvailable"`
	CreatedAt       int64  `json:"createdAt"`
}

func (a *app) records(w http.ResponseWriter, r *http.Request) {
	if identity(r).External {
		fail(w, 404, "页面不存在")
		return
	}
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000000 {
			fail(w, 400, "页码无效")
			return
		}
		page = n
	}
	const size = 50
	var total int
	if err := a.store.db.QueryRow("SELECT count(*) FROM jobs").Scan(&total); err != nil {
		fail(w, 500, "记录查询失败")
		return
	}
	rows, err := a.store.db.Query("SELECT "+columns+" FROM jobs ORDER BY created DESC,rowid DESC LIMIT ? OFFSET ?", size, (page-1)*size)
	if err != nil {
		fail(w, 500, "记录查询失败")
		return
	}
	defer rows.Close()
	result := []Record{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			fail(w, 500, "记录查询失败")
			return
		}
		user := j.Owner
		if user == "" {
			user = "内网"
		}
		result = append(result, Record{ID: j.ID, Name: j.Name, User: user, IP: j.ClientIP, Status: j.Status, Message: j.Message, Summary: j.Summary, ReportAvailable: j.ReportAvailable, CreatedAt: j.CreatedAt})
	}
	if rows.Err() != nil {
		fail(w, 500, "记录查询失败")
		return
	}
	jsonReply(w, 200, map[string]any{"records": result, "total": total, "page": page, "size": size})
}

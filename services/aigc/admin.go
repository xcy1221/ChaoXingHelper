package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const resolvedMessage = "管理员已核对并结束此任务，未自动重交"

func validResolution(id, note string) bool {
	return jobID.MatchString(id) && strings.TrimSpace(note) != "" && len(note) <= 512
}

func (s *Store) endUncertain(id, note string) error {
	if !validResolution(id, note) {
		return errors.New("任务标识或核对说明无效")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 不处理运行中的任务，不把异常任务放回提交队列。
	result, err := tx.Exec(`UPDATE jobs SET status='failed',message=?
WHERE id=? AND status='uncertain'
AND NOT EXISTS(SELECT 1 FROM jobs WHERE status IN ('uploading','submitting','detecting','waiting'))`, resolvedMessage, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("任务并非待核对状态，或仍有任务运行")
	}
	_, err = tx.Exec("INSERT INTO job_resolutions(job_id,note,checked_at) VALUES(?,?,?)", id, strings.TrimSpace(note), time.Now().Unix())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func runQueueCommand(dir string, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("queue", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	status := flags.Bool("queue-status", false, "")
	resolve := flags.String("resolve-job", "", "")
	note := flags.String("note", "", "")
	checked := flags.Bool("confirm-platform-checked", false, "")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return errors.New("队列管理参数无效")
	}
	if *status {
		if flags.NFlag() != 1 {
			return errors.New("查看队列时不能同时修改任务")
		}
	} else if !*checked || !validResolution(*resolve, *note) {
		return errors.New("需指定待核对任务、说明和平台已核对确认")
	}
	if info, err := os.Stat(filepath.Join(dir, "jobs.sqlite")); err != nil || !info.Mode().IsRegular() {
		return errors.New("现有任务数据库不可用")
	}
	store, err := openStore(dir)
	if err != nil {
		return errors.New("无法打开任务数据库")
	}
	defer store.db.Close()
	if !*status {
		if err = store.endUncertain(*resolve, *note); err != nil {
			return err
		}
	}
	paused, err := store.paused()
	if err != nil {
		return err
	}
	rows, err := store.db.QueryContext(context.Background(), "SELECT "+columns+" FROM jobs WHERE status='uncertain' ORDER BY rowid")
	if err != nil {
		return err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return scanErr
		}
		jobs = append(jobs, job)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		Paused    bool  `json:"paused"`
		Uncertain []Job `json:"uncertain"`
	}{paused, jobs})
}

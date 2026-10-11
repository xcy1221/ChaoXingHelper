package main

import (
	"context"
	"database/sql"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

var errNotFound = errors.New("任务不存在")
var errConflict = errors.New("请求标识已用于另一文档")
var errQueueFull = errors.New("队列已满")

type Job struct {
	ID              string `json:"id"`
	Name            string `json:"fileName"`
	Hash            string `json:"-"`
	Key             string `json:"-"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	Summary         string `json:"summary,omitempty"`
	ReportAvailable bool   `json:"reportAvailable"`
	CreatedAt       int64  `json:"createdAt"`
	SubmissionID    string `json:"-"`
	DetectionID     string `json:"-"`
	Owner           string `json:"-"`
	ClientIP        string `json:"-"`
}
type Store struct{ db *sql.DB }

func migrateJobOwners(db *sql.DB) error {
	rows, err := db.Query("SELECT name FROM pragma_table_info('jobs')")
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range []string{"owner", "client_ip"} {
		if !names[name] {
			if _, err := db.Exec("ALTER TABLE jobs ADD COLUMN " + name + " TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	return nil
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "jobs.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS jobs (
id TEXT PRIMARY KEY,name TEXT NOT NULL,hash TEXT NOT NULL,key TEXT UNIQUE NOT NULL,
status TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',summary TEXT NOT NULL DEFAULT '',
report INTEGER NOT NULL DEFAULT 0,created INTEGER NOT NULL,
submission TEXT NOT NULL DEFAULT '',detection TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS access_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,created INTEGER NOT NULL,user TEXT NOT NULL,
 ip TEXT NOT NULL,method TEXT NOT NULL,path TEXT NOT NULL,status INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS job_resolutions (
job_id TEXT PRIMARY KEY REFERENCES jobs(id),note TEXT NOT NULL,checked_at INTEGER NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS one_active ON jobs((1))
WHERE status IN ('uploading','submitting','detecting','waiting');`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateJobOwners(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}

const columns = "id,name,hash,key,status,message,summary,report,created,submission,detection,owner,client_ip"

func scanJob(row interface{ Scan(...any) error }) (j Job, err error) {
	err = row.Scan(&j.ID, &j.Name, &j.Hash, &j.Key, &j.Status, &j.Message, &j.Summary, &j.ReportAvailable, &j.CreatedAt, &j.SubmissionID, &j.DetectionID, &j.Owner, &j.ClientIP)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return
}
func (s *Store) get(id string) (Job, error) {
	return scanJob(s.db.QueryRow("SELECT "+columns+" FROM jobs WHERE id=?", id))
}
func (s *Store) create(j Job) (Job, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()
	old, err := scanJob(tx.QueryRow("SELECT "+columns+" FROM jobs WHERE key=?", j.Key))
	if err == nil {
		if old.Hash != j.Hash || old.Owner != j.Owner {
			return Job{}, false, errConflict
		}
		return old, false, nil
	}
	if !errors.Is(err, errNotFound) {
		return Job{}, false, err
	}
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM jobs WHERE status IN ('queued','uploading','submitting','detecting','waiting')").Scan(&count); err != nil {
		return Job{}, false, err
	}
	if count >= 20 {
		return Job{}, false, errQueueFull
	}
	j.Status = "queued"
	j.CreatedAt = time.Now().Unix()
	_, err = tx.Exec("INSERT INTO jobs(id,name,hash,key,status,created,owner,client_ip) VALUES(?,?,?,?,?,?,?,?)", j.ID, j.Name, j.Hash, j.Key, j.Status, j.CreatedAt, j.Owner, j.ClientIP)
	if err != nil {
		return Job{}, false, err
	}
	err = tx.Commit()
	return j, true, err
}
func (s *Store) claim(ctx context.Context) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, `UPDATE jobs SET status='uploading'
WHERE id=(SELECT id FROM jobs WHERE status='queued' ORDER BY rowid LIMIT 1)
AND NOT EXISTS(SELECT 1 FROM jobs WHERE status IN ('uploading','submitting','detecting','waiting','uncertain'))
RETURNING `+columns))
}
func (s *Store) update(id, status, message string) error {
	_, err := s.db.Exec("UPDATE jobs SET status=?,message=? WHERE id=?", status, message, id)
	return err
}
func (s *Store) finish(id string, result Result, report bool) error {
	_, err := s.db.Exec("UPDATE jobs SET status='succeeded',message='检测完成',summary=?,report=?,submission=?,detection=? WHERE id=?", result.Summary, report, result.SubmissionID, result.DetectionID, id)
	return err
}
func (s *Store) recover() error {
	_, err := s.db.Exec("UPDATE jobs SET status='uncertain',message='服务重启，需核对平台状态，避免重复提交' WHERE status IN ('uploading','submitting','detecting','waiting')")
	return err
}

func (s *Store) paused() (bool, error) {
	var paused bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM jobs WHERE status='uncertain')").Scan(&paused)
	return paused, err
}

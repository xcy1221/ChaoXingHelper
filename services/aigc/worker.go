package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Result struct {
	Summary, SubmissionID, DetectionID, DocumentHash string
	Report                                           []byte
}
type Runner interface {
	Ready() bool
	Run(context.Context, Job, string, func(string) error) (Result, error)
}
type UnconfiguredRunner struct{}

func (UnconfiguredRunner) Ready() bool { return false }
func (UnconfiguredRunner) Run(context.Context, Job, string, func(string) error) (Result, error) {
	return Result{}, errors.New("真实学习通检测流程尚未接入")
}
func runWorker(ctx context.Context, s *Store, dir string, runner Runner) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if !runNext(ctx, s, dir, runner) {
			return
		}
	}
}
func runNext(ctx context.Context, s *Store, dir string, runner Runner) bool {
	if !runner.Ready() {
		return true
	}
	if platform, ok := runner.(*ChaoXingRunner); ok && platform.onDemand {
		var pending bool
		if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM jobs WHERE status='queued')").Scan(&pending); err != nil || !pending {
			return true
		}
		release, err := browserLease(dir, "worker")
		if err != nil {
			return true
		}
		defer release()
	}
	j, err := s.claim(ctx)
	if err != nil {
		return true
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	advance := func(stage string) error {
		if stage != "submitting" && stage != "detecting" && stage != "waiting" {
			return errors.New("无效阶段")
		}
		return s.update(j.ID, stage, "")
	}
	result, err := runner.Run(runCtx, j, filepath.Join(dir, "uploads", j.ID+".docx"), advance)
	cancel()
	if err != nil {
		s.update(j.ID, "uncertain", "检测连接中断，需核对平台状态后处理")
		return true
	}
	if result.SubmissionID == "" || result.DetectionID == "" || result.Summary == "" || result.DocumentHash != j.Hash {
		s.update(j.ID, "uncertain", "无法确认报告与本次提交的对应关系")
		return true
	}
	report := len(result.Report) > 0
	if report {
		if _, _, err = reportFormat(result.Report); err != nil {
			s.update(j.ID, "uncertain", "报告格式或大小不符，需人工核对")
			return true
		}
		if err = os.WriteFile(filepath.Join(dir, "reports", j.ID+".report"), result.Report, 0600); err != nil {
			s.update(j.ID, "uncertain", "报告保存失败，需人工核对")
			return true
		}
	}
	return s.finish(j.ID, result, report) == nil
}

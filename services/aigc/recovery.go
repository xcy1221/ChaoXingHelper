package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// 只读取已触发的检测，核对本次附件与批次；不重交或重复检测。
func collectPlatformResult(dir string, r *ChaoXingRunner, id string, output io.Writer) error {
	if r.config.EnableAutomation || !jobID.MatchString(id) {
		return errors.New("需停用网页执行器并指定有效任务")
	}
	a, err := newApp(dir, r)
	if err != nil {
		return err
	}
	defer a.store.db.Close()
	j, err := a.store.get(id)
	if err != nil || j.Status != "uncertain" {
		return errors.New("任务不是待核对状态")
	}
	data, err := os.ReadFile(filepath.Join(dir, "uploads", id+".docx"))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != j.Hash {
		return errors.New("原始文档摘要不符")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root, closeRoot, err := r.session(ctx)
	defer closeRoot()
	if err != nil {
		return errors.New("浏览器控制连接不可用")
	}
	review, closeReview, err := platformTab(root, r.config.ReviewURL)
	defer closeReview()
	if err != nil {
		return errors.New("批阅页不可用")
	}
	if err = r.openReview(review); err != nil {
		return errors.New("查重浮窗不可用")
	}
	var saved PlatformSnapshot
	var row DetectionRow
	err = until(ctx, 3*time.Second, func() (bool, error) {
		var refreshed bool
		if err := platformCall(review, "refresh", r.config, &refreshed); err != nil {
			return false, err
		}
		if err := platformCall(review, "review", r.config, &saved); err != nil {
			return false, err
		}
		if len(saved.Attachments) != 1 || saved.Attachments[0].Name != id+".docx" || saved.Attachments[0].ObjectID == "" {
			return false, errors.New("当前附件不属于此任务")
		}
		if !saved.ResultVisible || len(saved.Rows) != 1 {
			return false, nil
		}
		if completeDetection(saved.Rows[0], r.config, nil, j.CreatedAt*1000) {
			row = saved.Rows[0]
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return errors.New("本次检测结果尚未确认；保留待核对状态")
	}
	report, err := browserReport(review, row.ReportURL)
	if err != nil {
		return err
	}
	result := Result{Summary: "疑似 AIGC 占比 " + row.AIGC + "；相似度 " + row.Similarity, SubmissionID: r.config.AnswerID + ":" + saved.Attachments[0].ObjectID, DetectionID: row.ID, DocumentHash: j.Hash, Report: report}
	if err = os.WriteFile(filepath.Join(dir, "reports", id+".report"), report, 0600); err != nil {
		return err
	}
	update, err := a.store.db.Exec("UPDATE jobs SET status='succeeded',message='已核对并保存本次检测结果',summary=?,report=1,submission=?,detection=? WHERE id=? AND status='uncertain' AND NOT EXISTS(SELECT 1 FROM jobs WHERE status IN ('uploading','submitting','detecting','waiting'))", result.Summary, result.SubmissionID, result.DetectionID, id)
	if err != nil {
		return err
	}
	count, err := update.RowsAffected()
	if err != nil || count != 1 {
		return errors.New("队列状态发生变化，未解除暂停")
	}
	j, err = a.store.get(id)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(j)
}
func verifyQueuedPlatform(dir string, platform *ChaoXingRunner, ids []string, output io.Writer) error {
	if platform.config.EnableAutomation || len(ids) < 1 || len(ids) > 2 {
		return errors.New("需停用网页执行器并指定一至两个排队任务")
	}
	a, err := newApp(dir, platform)
	if err != nil {
		return err
	}
	defer a.store.db.Close()
	var pending int
	if err = a.store.db.QueryRow("SELECT count(*) FROM jobs WHERE status NOT IN ('succeeded','failed')").Scan(&pending); err != nil || pending != len(ids) {
		return errors.New("队列范围不符或尚有待核对任务")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !jobID.MatchString(id) || seen[id] {
			return errors.New("排队任务标识无效")
		}
		seen[id] = true
		j, err := a.store.get(id)
		if err != nil || j.Status != "queued" {
			return errors.New("只能执行尚未开始的指定任务")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()
	probeCtx, probeCancel := context.WithTimeout(ctx, 90*time.Second)
	_, err = platform.probe(probeCtx)
	probeCancel()
	if err != nil {
		return err
	}
	config := platform.config
	config.EnableAutomation = true
	runner := &ChaoXingRunner{config: config, dir: dir}
	runner.ready.Store(true)
	return runVerificationJobs(ctx, a, runner, ids, output)
}

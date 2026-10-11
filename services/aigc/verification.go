package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// 管理员实测复用真实队列和执行器，不监听端口、不写入启用开关。
func verifyPlatform(dir string, platform *ChaoXingRunner, files []string, output io.Writer) error {
	if platform.config.EnableAutomation || len(files) < 1 || len(files) > 2 {
		return errors.New("实测需停用网页执行器，并指定一至两份测试 Word")
	}
	a, err := newApp(dir, platform)
	if err != nil {
		return errors.New("实测存储不可用")
	}
	defer a.store.db.Close()
	var pending int
	if err = a.store.db.QueryRow("SELECT count(*) FROM jobs WHERE status NOT IN ('succeeded','failed')").Scan(&pending); err != nil || pending != 0 {
		return errors.New("实测前需处理现有队列")
	}
	documents := make([][]byte, len(files))
	for i, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil || int64(len(data)) > maxUpload || !validateDocx(data) {
			return errors.New("测试文件不是有效 Word")
		}
		documents[i] = data
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()
	probeCtx, cancelProbe := context.WithTimeout(ctx, 90*time.Second)
	_, err = platform.probe(probeCtx)
	cancelProbe()
	if err != nil {
		return err
	}
	config := platform.config
	config.EnableAutomation = true
	runner := &ChaoXingRunner{config: config, dir: dir}
	runner.ready.Store(true)
	ids := make([]string, len(files))
	for i, data := range documents {
		random := make([]byte, 16)
		if _, err = rand.Read(random); err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		key := sha256.Sum256(append([]byte("private-platform-test:"), random...))
		j := Job{ID: hex.EncodeToString(random), Name: filepath.Base(files[i]), Hash: hex.EncodeToString(hash[:]), Key: hex.EncodeToString(key[:])}
		if err = os.WriteFile(filepath.Join(dir, "uploads", j.ID+".docx"), data, 0600); err != nil {
			return err
		}
		if _, _, err = a.store.create(j); err != nil {
			return err
		}
		ids[i] = j.ID
	}
	return runVerificationJobs(ctx, a, runner, ids, output)
}

func runVerificationJobs(parent context.Context, a *app, runner *ChaoXingRunner, ids []string, output io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() { defer close(done); runWorker(ctx, a.store, a.dir, verificationRunner{runner, a.dir}) }()
	defer func() { cancel(); <-done }()
	encoder := json.NewEncoder(output)
	previous := map[string]string{}
	for {
		complete := true
		for _, id := range ids {
			j, readErr := a.store.get(id)
			if readErr != nil {
				return readErr
			}
			if previous[id] != j.Status {
				if err := encoder.Encode(j); err != nil {
					return err
				}
				previous[id] = j.Status
			}
			if j.Status == "uncertain" || j.Status == "failed" {
				return errors.New("实测停止，请检查私有错误记录并核对平台")
			}
			complete = complete && j.Status == "succeeded"
		}
		if complete {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("实测等待超时，需核对平台")
		case <-time.After(time.Second):
		}
	}
}

type verificationRunner struct {
	*ChaoXingRunner
	dir string
}

func (r verificationRunner) Run(ctx context.Context, j Job, file string, advance func(string) error) (Result, error) {
	result, err := r.ChaoXingRunner.Run(ctx, j, file, advance)
	if err != nil {
		os.WriteFile(filepath.Join(r.dir, "account", j.ID+"-test-error.txt"), []byte(err.Error()), 0600)
	}
	return result, err
}

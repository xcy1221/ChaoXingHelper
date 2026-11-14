package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

//go:embed platform/login.js
var loginAdapter string

type loginCredentials struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

var phoneNumber = regexp.MustCompile(`^1[0-9]{10}$`)
var errLoginCredentials = errors.New("私有登录文件未配置、权限过宽或格式无效")
var errLoginRequired = errors.New("学习通自动登录未完成，请检查私有登录文件或人工验证")

func loadLoginCredentials(file string) (loginCredentials, error) {
	var c loginCredentials
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return c, errLoginCredentials
	}
	data, err := os.ReadFile(file)
	if err != nil || len(data) > 4096 {
		return c, errLoginCredentials
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&c) != nil || d.Decode(&extra) != io.EOF || !phoneNumber.MatchString(c.Phone) ||
		c.Password == "" || len(c.Password) > 256 || strings.ContainsAny(c.Password, "\x00\r\n") {
		return loginCredentials{}, errLoginCredentials
	}
	return c, nil
}

func isLoginURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "passport2.chaoxing.com" &&
		u.User == nil && u.Path == "/login" && u.Fragment == ""
}

func postLoginURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Host {
	case "i.chaoxing.com", "i.mooc.chaoxing.com", "mooc1.chaoxing.com", "mooc1-api.chaoxing.com", "mooc2-ans.chaoxing.com":
		return true
	}
	return false
}

// 每次核验/任务最多重登一次；仅在上传前调用，提交和检测不重试。
func (r *ChaoXingRunner) accountTab(root context.Context, address string, attempted *bool) (context.Context, context.CancelFunc, error) {
	ctx, cancel, err := platformTab(root, address)
	if err != nil {
		return ctx, cancel, errors.New("平台页面无法访问")
	}
	var current string
	if chromedp.Run(ctx, chromedp.Location(&current)) != nil {
		return ctx, cancel, errLoginRequired
	}
	if !isLoginURL(current) {
		return ctx, cancel, nil
	}
	if *attempted {
		return ctx, cancel, errLoginRequired
	}
	*attempted = true
	c, err := loadLoginCredentials(filepath.Join(r.dir, "account", "login.json"))
	if err != nil {
		return ctx, cancel, err
	}
	loginCtx, closeLogin := context.WithTimeout(ctx, 35*time.Second)
	defer closeLogin()
	expression := "(" + strings.TrimSpace(loginAdapter) + ").submit(" + string(mustJSON(c)) + ")"
	if chromedp.Run(loginCtx, chromedp.Evaluate(expression, nil)) != nil {
		return ctx, cancel, errLoginRequired
	}
	err = until(loginCtx, 500*time.Millisecond, func() (bool, error) {
		if chromedp.Run(loginCtx, chromedp.Location(&current)) != nil {
			return false, nil // 页面跳转时上下文可能暂时不可用。
		}
		if postLoginURL(current) {
			return true, nil
		}
		if !isLoginURL(current) {
			return false, nil
		}
		var blocked bool
		err := chromedp.Run(loginCtx, chromedp.Evaluate("("+strings.TrimSpace(loginAdapter)+").blocked()", &blocked))
		if err == nil && blocked {
			return false, errLoginRequired
		}
		return false, nil
	})
	if err != nil || chromedp.Run(ctx, chromedp.Navigate(address), chromedp.WaitReady("body", chromedp.ByQuery), chromedp.Location(&current)) != nil || isLoginURL(current) {
		return ctx, cancel, errLoginRequired
	}
	return ctx, cancel, nil
}

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrivateLoginCredentials(t *testing.T) {
	file := filepath.Join(t.TempDir(), "login.json")
	valid := `{"phone":"13800000000","password":" test-password "}`
	if err := os.WriteFile(file, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadLoginCredentials(file)
	if err != nil || c.Phone != "13800000000" || c.Password != " test-password " {
		t.Fatal("valid credentials rejected or password changed")
	}
	for _, data := range []string{
		`{`, valid + `{}`, valid + ` true`,
		`{"phone":"13800000000","password":"test-password","extra":true}`,
		`{"phone":"","password":"test-password"}`,
		`{"phone":"13800000000","password":""}`,
		`{"phone":"13800000000","password":"test-password\n"}`,
		strings.Repeat("x", 4097),
	} {
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := loadLoginCredentials(file)
		if err == nil || strings.Contains(err.Error(), "test-password") || strings.Contains(err.Error(), file) {
			t.Fatal("bad credentials accepted or secret/path exposed")
		}
	}
	if _, err := loadLoginCredentials(file + "-missing"); err == nil {
		t.Fatal("missing credentials accepted")
	}
	if _, err := loadLoginCredentials(filepath.Dir(file)); err == nil {
		t.Fatal("directory accepted")
	}
	if runtime.GOOS != "windows" {
		os.WriteFile(file, []byte(valid), 0600)
		os.Chmod(file, 0644)
		if _, err := loadLoginCredentials(file); err == nil {
			t.Fatal("world-readable password accepted")
		}
		os.Chmod(file, 0600)
		link := file + "-link"
		if err := os.Symlink(file, link); err != nil {
			t.Fatal(err)
		}
		if _, err := loadLoginCredentials(link); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}

func TestLoginOriginIsExact(t *testing.T) {
	if !isLoginURL("https://passport2.chaoxing.com/login?refer=fixture") {
		t.Fatal("official login rejected")
	}
	for _, raw := range []string{
		"http://passport2.chaoxing.com/login", "https://passport2.chaoxing.com.attacker.example/login",
		"https://attacker.example/login", "https://passport2.chaoxing.com:8443/login",
		"https://user@passport2.chaoxing.com/login", "https://passport2.chaoxing.com/other",
		"https://passport2.chaoxing.com/login#other",
	} {
		if isLoginURL(raw) {
			t.Fatal("unsafe login destination accepted")
		}
	}
	if !postLoginURL("https://i.chaoxing.com/base?ws=1") || postLoginURL("https://passport2.chaoxing.com/login") ||
		postLoginURL("https://i.chaoxing.com.attacker.example/") || postLoginURL("https://user@i.chaoxing.com/") {
		t.Fatal("invalid post-login redirect handling")
	}
}

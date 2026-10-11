package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
)

func reportFormat(data []byte) (string, string, error) {
	invalid := errors.New("报告格式无效")
	if len(data) == 0 || len(data) > 32<<20 {
		return "", "", invalid
	}
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return ".pdf", "application/pdf", nil
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) == 0 || len(z.File) > 16 {
		return "", "", invalid
	}
	var total uint64
	for _, f := range z.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if f.FileInfo().IsDir() || path.Base(name) != name || !strings.EqualFold(path.Ext(name), ".pdf") ||
			f.UncompressedSize64 > 64<<20-total {
			return "", "", invalid
		}
		total += f.UncompressedSize64
		r, err := f.Open()
		if err != nil {
			return "", "", invalid
		}
		var header [5]byte
		_, headErr := io.ReadFull(r, header[:])
		_, readErr := io.Copy(io.Discard, io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if headErr != nil || readErr != nil || string(header[:]) != "%PDF-" {
			return "", "", invalid
		}
	}
	return ".zip", "application/zip", nil
}

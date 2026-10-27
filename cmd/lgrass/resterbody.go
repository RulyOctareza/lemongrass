// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/faizalv/lemongrass/internal/restergate"
)

const (
	defaultBodyFileType = "application/json"
	formURLEncodedType  = "application/x-www-form-urlencoded"
	octetStreamType     = "application/octet-stream"
	filePartTypeMarker  = ";type="
)

var extensionTypes = map[string]string{
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xls":  "application/vnd.ms-excel",
	".csv":  "text/csv",
	".json": "application/json",
	".pdf":  "application/pdf",
	".zip":  "application/zip",
	".txt":  "text/plain",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

type formField struct {
	name  string
	value string
}

type filePart struct {
	field       string
	path        string
	contentType string
}

// parseFormField splits a --form value of the form key=value.
func parseFormField(raw string) (formField, error) {
	name, value, ok := strings.Cut(raw, "=")
	if !ok || name == "" {
		return formField{}, fmt.Errorf("--form needs key=value, got %q", raw)
	}
	return formField{name: name, value: value}, nil
}

// parseFilePart splits a --file value of the form field=path or field=path;type=mime.
func parseFilePart(raw string) (filePart, error) {
	field, rest, ok := strings.Cut(raw, "=")
	if !ok || field == "" || rest == "" {
		return filePart{}, fmt.Errorf("--file needs field=path, got %q", raw)
	}
	part := filePart{field: field, path: rest}
	if i := strings.LastIndex(rest, filePartTypeMarker); i >= 0 {
		part.path = rest[:i]
		part.contentType = rest[i+len(filePartTypeMarker):]
		if err := restergate.CheckContentType(part.contentType); err != nil {
			return filePart{}, fmt.Errorf("--file %s has an invalid type %q", field, part.contentType)
		}
	}
	if part.path == "" {
		return filePart{}, fmt.Errorf("--file needs field=path, got %q", raw)
	}
	return part, nil
}

func fileTypeFor(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if t, ok := extensionTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return octetStreamType
}

func requestLimitError(what string, size int64) error {
	return fmt.Errorf("%s is %d bytes, over the %d MiB request limit", what, size, restergate.MaxRequestBodyBytes>>20)
}

// readRequestFile reads path when it is a regular file within the request size limit.
func readRequestFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > restergate.MaxRequestBodyBytes {
		return nil, requestLimitError(path, info.Size())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return b, nil
}

// buildResterBody turns the body flags into the bytes to send and their content type. The
// content type is empty for an inline JSON body and set for every body read from a file or
// built from form input.
func buildResterBody(cmd resterCommand) ([]byte, string, error) {
	switch {
	case cmd.bodyFile != "":
		b, err := readRequestFile(cmd.bodyFile)
		if err != nil {
			return nil, "", err
		}
		contentType := cmd.contentType
		if contentType == "" {
			contentType = defaultBodyFileType
		}
		return b, contentType, nil
	case len(cmd.files) > 0:
		return buildMultipartBody(cmd.forms, cmd.files)
	case len(cmd.forms) > 0:
		values := url.Values{}
		for _, f := range cmd.forms {
			values.Add(f.name, f.value)
		}
		b := []byte(values.Encode())
		if len(b) > restergate.MaxRequestBodyBytes {
			return nil, "", requestLimitError("the form body", int64(len(b)))
		}
		return b, formURLEncodedType, nil
	default:
		return []byte(cmd.body), "", nil
	}
}

func buildMultipartBody(forms []formField, files []filePart) ([]byte, string, error) {
	var total int64
	for _, f := range files {
		info, err := os.Stat(f.path)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read %s: %w", f.path, err)
		}
		total += info.Size()
	}
	if total > restergate.MaxRequestBodyBytes {
		return nil, "", requestLimitError("the files together", total)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range forms {
		if err := w.WriteField(f.name, f.value); err != nil {
			return nil, "", err
		}
	}
	for _, f := range files {
		data, err := readRequestFile(f.path)
		if err != nil {
			return nil, "", err
		}
		contentType := f.contentType
		if contentType == "" {
			contentType = fileTypeFor(f.path)
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
			"name":     f.field,
			"filename": filepath.Base(f.path),
		}))
		header.Set("Content-Type", contentType)
		part, err := w.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	if buf.Len() > restergate.MaxRequestBodyBytes {
		return nil, "", requestLimitError("the request body", int64(buf.Len()))
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

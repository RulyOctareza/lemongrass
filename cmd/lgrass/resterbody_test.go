// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/restergate"
)

const xlsxMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func spreadsheetBytes() []byte {
	b := []byte("PK\x03\x04")
	for i := 0; i < 256; i++ {
		b = append(b, byte(i))
	}
	return append(b, bytes.Repeat([]byte{0, 0xff, 0x0d, 0x0a}, 32)...)
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type readPart struct {
	name, filename, contentType string
	data                        []byte
}

func readMultipart(t *testing.T, body []byte, contentType string) []readPart {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type %q: %v", contentType, err)
	}
	r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var parts []readPart
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			return parts
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		data, _ := io.ReadAll(p)
		parts = append(parts, readPart{name: p.FormName(), filename: p.FileName(), contentType: p.Header.Get("Content-Type"), data: data})
	}
}

func TestBuildResterBodyInlineHasNoContentType(t *testing.T) {
	body, contentType, err := buildResterBody(resterCommand{body: `{"a":1}`})
	if err != nil || string(body) != `{"a":1}` || contentType != "" {
		t.Errorf("got %q, %q, %v", body, contentType, err)
	}
	body, contentType, err = buildResterBody(resterCommand{})
	if err != nil || len(body) != 0 || contentType != "" {
		t.Errorf("no body: got %q, %q, %v", body, contentType, err)
	}
}

func TestBuildResterBodyFileDefaultsToJSONAndKeepsBytes(t *testing.T) {
	path := writeTemp(t, "payload.json", []byte(`{"rows":[1,2]}`))
	body, contentType, err := buildResterBody(resterCommand{bodyFile: path})
	if err != nil || string(body) != `{"rows":[1,2]}` || contentType != "application/json" {
		t.Errorf("got %q, %q, %v", body, contentType, err)
	}

	want := spreadsheetBytes()
	path = writeTemp(t, "book.xlsx", want)
	body, contentType, err = buildResterBody(resterCommand{bodyFile: path, contentType: xlsxMIME})
	if err != nil || !bytes.Equal(body, want) || contentType != xlsxMIME {
		t.Errorf("raw file: identical = %v, type %q, err %v", bytes.Equal(body, want), contentType, err)
	}
}

func TestBuildResterBodyMultipartCarriesFileByteForByte(t *testing.T) {
	want := spreadsheetBytes()
	xlsx := writeTemp(t, "Q3 report.xlsx", want)
	csv := writeTemp(t, "extra.csv", []byte("a,b\n1,2\n"))
	blob := writeTemp(t, "blob.unknownext", []byte("raw"))

	body, contentType, err := buildResterBody(resterCommand{
		forms: []formField{{name: "sheet", value: "Summary"}, {name: "mode", value: "append"}},
		files: []filePart{
			{field: "upload", path: xlsx},
			{field: "extra", path: csv, contentType: "text/plain"},
			{field: "other", path: blob},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := readMultipart(t, body, contentType)
	if len(parts) != 5 {
		t.Fatalf("parts = %d, want 2 fields then 3 files", len(parts))
	}
	if parts[0].name != "sheet" || string(parts[0].data) != "Summary" || parts[1].name != "mode" || string(parts[1].data) != "append" {
		t.Errorf("form fields = %+v, %+v, want them first and in order", parts[0], parts[1])
	}
	if p := parts[2]; p.name != "upload" || p.filename != "Q3 report.xlsx" || p.contentType != xlsxMIME || !bytes.Equal(p.data, want) {
		t.Errorf("xlsx part = name %q file %q type %q identical %v", p.name, p.filename, p.contentType, bytes.Equal(p.data, want))
	}
	if p := parts[3]; p.filename != "extra.csv" || p.contentType != "text/plain" {
		t.Errorf("override part = file %q type %q, want the ;type= override", p.filename, p.contentType)
	}
	if p := parts[4]; p.contentType != "application/octet-stream" {
		t.Errorf("unknown extension type = %q", p.contentType)
	}
}

func TestBuildResterBodyMultipartQuotesAwkwardFileNames(t *testing.T) {
	path := writeTemp(t, `we"ird name.csv`, []byte("x"))
	body, contentType, err := buildResterBody(resterCommand{files: []filePart{{field: `up"load`, path: path}}})
	if err != nil {
		t.Fatal(err)
	}
	parts := readMultipart(t, body, contentType)
	if len(parts) != 1 || parts[0].name != `up"load` || parts[0].filename != `we"ird name.csv` {
		t.Errorf("parts = %+v, want the quoted names to round trip", parts)
	}
}

func TestBuildResterBodyFormOnlyIsURLEncoded(t *testing.T) {
	body, contentType, err := buildResterBody(resterCommand{forms: []formField{{name: "a", value: "1 2"}, {name: "a", value: "x&y"}, {name: "b", value: ""}}})
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("type %q, err %v", contentType, err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil || strings.Join(values["a"], "|") != "1 2|x&y" || len(values["b"]) != 1 {
		t.Errorf("values = %v, %v", values, err)
	}
}

func TestBuildResterBodyRefusesUnreadableAndOversizeFiles(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := buildResterBody(resterCommand{bodyFile: filepath.Join(dir, "missing.xlsx")}); err == nil || !strings.Contains(err.Error(), "missing.xlsx") {
		t.Errorf("missing file err = %v, want one naming the path", err)
	}
	if _, _, err := buildResterBody(resterCommand{bodyFile: dir}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("directory err = %v", err)
	}

	big := filepath.Join(dir, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(restergate.MaxRequestBodyBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for name, cmd := range map[string]resterCommand{
		"body file": {bodyFile: big},
		"multipart": {files: []filePart{{field: "f", path: big}}},
	} {
		if _, _, err := buildResterBody(cmd); err == nil || !strings.Contains(err.Error(), "25 MiB") {
			t.Errorf("%s: err = %v, want one naming the 25 MiB limit", name, err)
		}
	}
}

func TestParseFilePartKeepsEqualsAndSemicolonsInPath(t *testing.T) {
	part, err := parseFilePart("upload=/tmp/a=b;c.xlsx")
	if err != nil || part.field != "upload" || part.path != "/tmp/a=b;c.xlsx" || part.contentType != "" {
		t.Errorf("part = %+v, %v", part, err)
	}
}

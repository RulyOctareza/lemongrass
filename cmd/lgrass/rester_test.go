// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestParseResterArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    resterCommand
		wantErr string
	}{
		{name: "get", args: []string{"abc", "get", "/orders/42"}, want: resterCommand{shortID: "abc", method: "GET", path: "/orders/42"}},
		{name: "verb case", args: []string{"abc", "PATCH", "/orders/42"}, want: resterCommand{shortID: "abc", method: "PATCH", path: "/orders/42"}},
		{name: "post with body and user", args: []string{"abc", "post", "/orders", "--user", "alice", "--body", `{"a":1}`}, want: resterCommand{shortID: "abc", method: "POST", path: "/orders", user: "alice", body: `{"a":1}`}},
		{name: "flags before path", args: []string{"abc", "post", "--user", "alice", "/orders"}, want: resterCommand{shortID: "abc", method: "POST", path: "/orders", user: "alice"}},
		{name: "equals form", args: []string{"abc", "post", "/orders", "--user=alice", `--body={"a":1}`}, want: resterCommand{shortID: "abc", method: "POST", path: "/orders", user: "alice", body: `{"a":1}`}},
		{name: "full url path", args: []string{"abc", "get", "https://api.example.com/orders?x=1"}, want: resterCommand{shortID: "abc", method: "GET", path: "https://api.example.com/orders?x=1"}},
		{name: "info", args: []string{"abc", "info"}, want: resterCommand{shortID: "abc", info: true}},
		{name: "users", args: []string{"abc", "users"}, want: resterCommand{shortID: "abc", users: true}},
		{name: "flush all", args: []string{"abc", "flush"}, want: resterCommand{shortID: "abc", flush: true}},
		{name: "flush one user", args: []string{"abc", "flush", "--user", "alice"}, want: resterCommand{shortID: "abc", flush: true, user: "alice"}},
		{name: "flush equals form", args: []string{"abc", "FLUSH", "--user=alice"}, want: resterCommand{shortID: "abc", flush: true, user: "alice"}},
		{name: "flush with path", args: []string{"abc", "flush", "/x"}, wantErr: "flush takes only --user"},
		{name: "flush with body", args: []string{"abc", "flush", "--body", "{}"}, wantErr: "flush takes only --user"},
		{name: "body file", args: []string{"abc", "post", "/x", "--body-file", "a.json"}, want: resterCommand{shortID: "abc", method: "POST", path: "/x", bodyFile: "a.json"}},
		{name: "body file with type", args: []string{"abc", "put", "/x", "--body-file", "a.csv", "--content-type", "text/csv"}, want: resterCommand{shortID: "abc", method: "PUT", path: "/x", bodyFile: "a.csv", contentType: "text/csv"}},
		{name: "multipart", args: []string{"abc", "post", "/import", "--form", "sheet=1", "--file", "upload=/tmp/a.xlsx", "--file", "extra=b.csv;type=text/csv"}, want: resterCommand{shortID: "abc", method: "POST", path: "/import", forms: []formField{{name: "sheet", value: "1"}}, files: []filePart{{field: "upload", path: "/tmp/a.xlsx"}, {field: "extra", path: "b.csv", contentType: "text/csv"}}}},
		{name: "form only", args: []string{"abc", "post", "/x", "--form", "a=1", "--form", "b="}, want: resterCommand{shortID: "abc", method: "POST", path: "/x", forms: []formField{{name: "a", value: "1"}, {name: "b", value: ""}}}},
		{name: "body and body file", args: []string{"abc", "post", "/x", "--body", "{}", "--body-file", "a.json"}, wantErr: "use only one kind of body"},
		{name: "body file and file", args: []string{"abc", "post", "/x", "--body-file", "a.json", "--file", "f=a.xlsx"}, wantErr: "use only one kind of body"},
		{name: "body and form", args: []string{"abc", "post", "/x", "--body", "{}", "--form", "a=1"}, wantErr: "use only one kind of body"},
		{name: "content type alone", args: []string{"abc", "post", "/x", "--content-type", "text/csv"}, wantErr: "--content-type only goes with --body-file"},
		{name: "content type with file", args: []string{"abc", "post", "/x", "--file", "f=a.xlsx", "--content-type", "text/csv"}, wantErr: "--content-type only goes with --body-file"},
		{name: "bad content type", args: []string{"abc", "post", "/x", "--body-file", "a", "--content-type", "nonsense"}, wantErr: "not a valid media type"},
		{name: "form without equals", args: []string{"abc", "post", "/x", "--form", "novalue"}, wantErr: "--form needs key=value"},
		{name: "file without path", args: []string{"abc", "post", "/x", "--file", "field="}, wantErr: "--file needs field=path"},
		{name: "file bad type", args: []string{"abc", "post", "/x", "--file", "f=a.bin;type=nonsense"}, wantErr: "invalid type"},
		{name: "info with body file", args: []string{"abc", "info", "--body-file", "a"}, wantErr: "info takes no other arguments"},
		{name: "flush with form", args: []string{"abc", "flush", "--form", "a=1"}, wantErr: "flush takes only --user"},
		{name: "users extra argument", args: []string{"abc", "users", "--user", "x"}, wantErr: "users takes no other arguments"},
		{name: "info extra argument", args: []string{"abc", "info", "/x"}, wantErr: "info takes no other arguments"},
		{name: "missing command", args: []string{"abc"}, wantErr: "expected a channel id and a command"},
		{name: "unknown command", args: []string{"abc", "fetch", "/x"}, wantErr: `unknown command "fetch"`},
		{name: "old method flag", args: []string{"abc", "--user", "alice", "--method", "GET", "--path", "/x"}, wantErr: `unknown command "--user"`},
		{name: "old path flag after verb", args: []string{"abc", "get", "--path", "/x"}, wantErr: "unknown flag --path"},
		{name: "missing path", args: []string{"abc", "get"}, wantErr: "get needs exactly one path"},
		{name: "two paths", args: []string{"abc", "get", "/a", "/b"}, wantErr: "get needs exactly one path"},
		{name: "flag without value", args: []string{"abc", "get", "/a", "--user"}, wantErr: "--user needs a value"},
		{name: "headers", args: []string{"abc", "post", "/x", "--header", "anthropic-version: 2023-06-01", "--header=X-Empty:", "--header", "X-Url:  https://a.test:8080/ "}, want: resterCommand{shortID: "abc", method: "POST", path: "/x", headers: []vault.Header{{Name: "anthropic-version", Value: "2023-06-01"}, {Name: "X-Empty", Value: ""}, {Name: "X-Url", Value: "https://a.test:8080/"}}}},
		{name: "header with out", args: []string{"abc", "get", "/x", "--out", "f", "--header", "Accept: text/csv"}, want: resterCommand{shortID: "abc", method: "GET", path: "/x", out: "f", headers: []vault.Header{{Name: "Accept", Value: "text/csv"}}}},
		{name: "header without colon", args: []string{"abc", "get", "/x", "--header", "Accept"}, wantErr: "--header needs '<name>: <value>'"},
		{name: "header bad name", args: []string{"abc", "get", "/x", "--header", "X Bad: 1"}, wantErr: "--header: header name \"X Bad\""},
		{name: "header empty name", args: []string{"abc", "get", "/x", "--header", ": 1"}, wantErr: "--header: header name is empty"},
		{name: "info with header", args: []string{"abc", "info", "--header", "A: 1"}, wantErr: "info takes no other arguments"},
		{name: "users with header", args: []string{"abc", "users", "--header", "A: 1"}, wantErr: "users takes no other arguments"},
		{name: "flush with header", args: []string{"abc", "flush", "--header", "A: 1"}, wantErr: "flush takes only --user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResterArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package flutter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VM talks to a Dart VM service over its plain HTTP form, where the path names the method and
// the query carries its parameters, so no websocket client is needed.
type VM struct {
	base   string
	client *http.Client
}

// DialVM takes the ws URI flutter run reports, such as ws://127.0.0.1:1234/abc=/ws.
func DialVM(wsURI string) (*VM, error) {
	u, err := url.Parse(wsURI)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("flutter: %q is not a VM service address", wsURI)
	}
	u.Scheme = strings.Replace(u.Scheme, "ws", "http", 1)
	u.Path = strings.TrimSuffix(u.Path, "ws")
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return &VM{base: u.String(), client: &http.Client{Timeout: 20 * time.Second}}, nil
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Details string `json:"details"`
	} `json:"data"`
}

// Call runs one method and decodes its result into out.
func (v *VM) Call(method string, params url.Values, out any) error {
	target := v.base + method
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	resp, err := v.client.Get(target)
	if err != nil {
		return fmt.Errorf("flutter: the VM service did not answer, is the app still running? %w", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("flutter: reading the VM service answer to %s: %w", method, err)
	}
	if envelope.Error != nil {
		msg := envelope.Error.Message
		if envelope.Error.Data.Details != "" {
			msg += ": " + envelope.Error.Data.Details
		}
		return fmt.Errorf("flutter: %s: %s", method, msg)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}

// Isolate is one Dart isolate of the app.
type Isolate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Isolates lists the app's isolates, the main one first.
func (v *VM) Isolates() ([]Isolate, error) {
	var vm struct {
		Isolates []Isolate `json:"isolates"`
	}
	if err := v.Call("getVM", nil, &vm); err != nil {
		return nil, err
	}
	if len(vm.Isolates) == 0 {
		return nil, errors.New("flutter: the app has no isolates yet")
	}
	for i, iso := range vm.Isolates {
		if iso.Name == "main" && i > 0 {
			vm.Isolates[0], vm.Isolates[i] = vm.Isolates[i], vm.Isolates[0]
		}
	}
	return vm.Isolates, nil
}

// EnableHTTPProfiling turns on dart:io's HTTP profile in every isolate, so lgrass flutter http
// sees calls from then on. The run supervisor calls it once the app starts and after a restart,
// since a hot restart starts fresh isolates.
func EnableHTTPProfiling(wsURI string) {
	vm, err := DialVM(wsURI)
	if err != nil {
		fmt.Println("lgrass: " + err.Error())
		return
	}
	var last error
	// The dart:io extensions register a moment after the isolate starts.
	for range 20 {
		isolates, err := vm.Isolates()
		if err == nil {
			enabled := 0
			for _, iso := range isolates {
				params := url.Values{"isolateId": {iso.ID}, "enabled": {"true"}}
				if err = vm.Call("ext.dart.io.httpEnableTimelineLogging", params, nil); err == nil {
					enabled++
				}
			}
			if enabled > 0 {
				fmt.Printf("lgrass: HTTP profiling enabled in %d isolate(s)\n", enabled)
				return
			}
		}
		last = err
		time.Sleep(time.Second)
	}
	fmt.Printf("lgrass: could not enable HTTP profiling: %v\n", last)
}

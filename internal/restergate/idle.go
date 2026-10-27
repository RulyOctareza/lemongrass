// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// idleTimer cancels a request when nothing has happened for d: no response headers yet, or no body bytes since the last read.
type idleTimer struct {
	ctx    context.Context
	cancel context.CancelFunc
	timer  *time.Timer
	d      time.Duration
	fired  atomic.Bool
}

func newIdleTimer(d time.Duration) *idleTimer {
	ctx, cancel := context.WithCancel(context.Background())
	t := &idleTimer{ctx: ctx, cancel: cancel, d: d}
	t.timer = time.AfterFunc(d, func() {
		t.fired.Store(true)
		cancel()
	})
	return t
}

func (t *idleTimer) stop() {
	t.timer.Stop()
	t.cancel()
}

// explain names the idle timeout when it, not the transport, ended the request.
func (t *idleTimer) explain(err error) error {
	if t.fired.Load() {
		return fmt.Errorf("nothing received for %s: %w", t.d, err)
	}
	return err
}

func (t *idleTimer) wrap(body io.ReadCloser) io.ReadCloser {
	t.timer.Reset(t.d)
	return &idleBody{body: body, idle: t}
}

type idleBody struct {
	body io.ReadCloser
	idle *idleTimer
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, b.idle.explain(err)
	}
	b.idle.timer.Reset(b.idle.d)
	return n, err
}

func (b *idleBody) Close() error {
	b.idle.stop()
	return b.body.Close()
}

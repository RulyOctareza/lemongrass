// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/faizalv/lemongrass/cmd/lgrass/version"
	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/node"
	"github.com/faizalv/lemongrass/internal/nodesvc"
)

const nodeSyncInterval = 15 * time.Second

func cmdNode(args []string) {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(os.Stderr, "usage: lgrassd node run")
		os.Exit(1)
	}
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sockPath := nodesvc.SocketPath()
	os.Remove(sockPath) // a stale socket left behind by a previous, uncleanly-stopped run

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer l.Close()
	if err := os.Chmod(sockPath, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	svc := nodesvc.NewService(node.NewStore(node.ConfigPath()), node.NewLibrary(), nodesvc.DefaultProvider(version.Version))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		svc.Run(ctx, nodeSyncInterval)
		close(done)
	}()
	go func() {
		<-ctx.Done()
		l.Close()
	}()

	fmt.Printf("lgrassd node: listening on %s\n", sockPath)
	if err := svc.Serve(l); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	<-done
}

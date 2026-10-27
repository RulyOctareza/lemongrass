// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package tunnel opens the listener the node serves on, either through ngrok or on a loopback port.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"golang.ngrok.com/ngrok/v2"
)

// Provider opens a listener and the URL consumers reach it at; closing the listener closes the tunnel.
type Provider interface {
	Listen(ctx context.Context) (net.Listener, string, error)
}

const DefaultLocalPort = 7457

// Local listens on a loopback port and exposes nothing beyond this machine.
type Local struct {
	Port int
}

func (l Local) Listen(ctx context.Context) (net.Listener, string, error) {
	port := l.Port
	if port == 0 {
		port = DefaultLocalPort
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, "", fmt.Errorf("tunnel: %w", err)
	}
	return ln, "http://" + ln.Addr().String(), nil
}

// Ngrok serves straight off an ngrok endpoint through the embedded agent, so no local port opens.
type Ngrok struct {
	Authtoken string
	// Domain is an optional reserved domain that keeps the URL stable across runs.
	Domain     string
	ClientName string
	Version    string
}

func (n Ngrok) Listen(ctx context.Context) (net.Listener, string, error) {
	if n.Authtoken == "" {
		return nil, "", errors.New("tunnel: no ngrok authtoken, set one in the Node view or NGROK_AUTHTOKEN")
	}
	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(n.Authtoken), ngrok.WithClientInfo(n.ClientName, n.Version))
	if err != nil {
		return nil, "", fmt.Errorf("tunnel: %w", err)
	}
	var opts []ngrok.EndpointOption
	if n.Domain != "" {
		domain := n.Domain
		if !strings.Contains(domain, "://") {
			domain = "https://" + domain
		}
		opts = append(opts, ngrok.WithURL(domain))
	}
	ln, err := agent.Listen(ctx, opts...)
	if err != nil {
		agent.Disconnect()
		return nil, "", fmt.Errorf("tunnel: %w", err)
	}
	return &ngrokListener{EndpointListener: ln, agent: agent}, ln.URL().String(), nil
}

type ngrokListener struct {
	ngrok.EndpointListener
	agent ngrok.Agent
}

func (l *ngrokListener) Close() error {
	err := l.EndpointListener.Close()
	l.agent.Disconnect()
	return err
}

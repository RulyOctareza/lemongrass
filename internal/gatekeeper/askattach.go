// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// attach_ask is an app op that holds its connection open: after the reply, which carries the replay, the daemon writes output frames and one end frame, and the app writes input and resize frames. Both directions are JSON lines; data is base64 through encoding/json's []byte.
const opAttachAsk = "attach_ask"

// Frame types on an attach connection.
const (
	askFrameOutput = "output"
	askFrameEnd    = "end"
	askFrameInput  = "input"
	askFrameResize = "resize"
)

// askFrameWriteTimeout bounds one frame write, so a viewer that stops reading is dropped instead of holding the daemon's goroutine.
const askFrameWriteTimeout = 10 * time.Second

type attachAskReply struct {
	Replay []byte `json:"replay"`
}

// AskFrame is one message on an attach connection. Reason is set on an end frame: ended, replaced, behind or detached.
type AskFrame struct {
	Type   string `json:"type"`
	Data   []byte `json:"data,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	Cols   int    `json:"cols,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// attachAsk makes a new viewer of a running command's terminal.
func (b *Backend) attachAsk(id string) (*askTerm, []byte, *askViewer, error) {
	b.runs.mu.Lock()
	var term *askTerm
	if run := b.runs.runs[id]; run != nil {
		term = run.term
	}
	b.runs.mu.Unlock()
	if term == nil {
		return nil, nil, nil, errors.New("gatekeeper: no command is running for that ask")
	}
	replay, v, err := term.attach()
	if err != nil {
		return nil, nil, nil, err
	}
	return term, replay, v, nil
}

// serveAttachAsk runs one attach connection. dec is the decoder that read the request, since the app may have written frames behind it already.
func serveAttachAsk(svc *Backend, adminLimiter *vault.FailureLimiter, conn net.Conn, dec *json.Decoder, req request) {
	enc := json.NewEncoder(conn)
	var p askIDPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		enc.Encode(errResponse(err))
		return
	}
	var term *askTerm
	var v *askViewer
	resp := adminOp(adminLimiter, func() (response, error) {
		if err := svc.verifyRoot(p.RootSecret); err != nil {
			return response{}, err
		}
		t, replay, view, err := svc.attachAsk(p.ID)
		if err != nil {
			return response{}, err
		}
		term, v = t, view
		return payloadResponse(attachAskReply{Replay: replay}), nil
	})
	// The command's own timeout bounds the connection, so it has no deadline of its own.
	conn.SetDeadline(time.Time{})
	conn.SetWriteDeadline(time.Now().Add(askFrameWriteTimeout))
	if err := enc.Encode(resp); err != nil || !resp.OK {
		if v != nil {
			term.detach(v)
		}
		return
	}

	go func() {
		defer term.detach(v)
		for {
			var f AskFrame
			if err := dec.Decode(&f); err != nil {
				return
			}
			// A refused write means the view was cut off, and its end frame is on its way.
			switch f.Type {
			case askFrameInput:
				_ = term.input(v, f.Data)
			case askFrameResize:
				_ = term.resize(v, f.Rows, f.Cols)
			}
		}
	}()

	for chunk := range v.out {
		conn.SetWriteDeadline(time.Now().Add(askFrameWriteTimeout))
		if err := enc.Encode(AskFrame{Type: askFrameOutput, Data: chunk}); err != nil {
			term.detach(v)
			for range v.out {
			}
			return
		}
	}
	conn.SetWriteDeadline(time.Now().Add(askFrameWriteTimeout))
	enc.Encode(AskFrame{Type: askFrameEnd, Reason: v.reason})
}

// AskAttachment is an open attach connection to a running command's terminal.
type AskAttachment struct {
	// Replay is the output so far.
	Replay []byte
	conn   net.Conn
	dec    *json.Decoder
	enc    *json.Encoder
}

// AttachAsk opens a view of a running command ask's terminal.
func (c *Client) AttachAsk(rootSecret, id string) (*AskAttachment, error) {
	conn, err := net.DialTimeout("unix", c.SocketPath, c.timeout())
	if err != nil {
		return nil, fmt.Errorf("gatekeeper: dialing %s: %w", c.SocketPath, err)
	}
	conn.SetDeadline(time.Now().Add(c.timeout()))
	payload, err := json.Marshal(askIDPayload{RootSecret: rootSecret, ID: id})
	if err != nil {
		conn.Close()
		return nil, err
	}
	enc := json.NewEncoder(conn)
	if err := enc.Encode(request{Op: opAttachAsk, Payload: payload}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("gatekeeper: sending request: %w", err)
	}
	dec := json.NewDecoder(conn)
	var resp response
	if err := dec.Decode(&resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("gatekeeper: reading response: %w", err)
	}
	if !resp.OK {
		conn.Close()
		return nil, errors.New(resp.Error)
	}
	var reply attachAskReply
	if err := json.Unmarshal(resp.Payload, &reply); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return &AskAttachment{Replay: reply.Replay, conn: conn, dec: dec, enc: enc}, nil
}

// Next reads the next frame; an end frame is the last.
func (a *AskAttachment) Next() (AskFrame, error) {
	var f AskFrame
	err := a.dec.Decode(&f)
	return f, err
}

func (a *AskAttachment) Input(data []byte) error {
	return a.enc.Encode(AskFrame{Type: askFrameInput, Data: data})
}

func (a *AskAttachment) Resize(rows, cols int) error {
	return a.enc.Encode(AskFrame{Type: askFrameResize, Rows: rows, Cols: cols})
}

func (a *AskAttachment) Close() error {
	return a.conn.Close()
}

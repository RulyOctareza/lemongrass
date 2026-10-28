// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

const (
	// maxFrame bounds one data frame, so a peer cannot make the reader allocate an arbitrary amount.
	maxFrame = 1 << 20
	// frameChunk is how much of a body one data frame carries.
	frameChunk = 64 << 10
	// abortFrame in place of a length says the sender failed midway, and a short message follows.
	abortFrame = 0xFFFFFFFF
	// maxAbortMessage bounds the error text an abort frame carries.
	maxAbortMessage = 1024
)

// RemoteError is a failure a daemon reported in its response, as opposed to one that happened while talking to it.
type RemoteError string

func (e RemoteError) Error() string { return string(e) }

// StreamMeta describes a 2xx response whose body follows the header line as frames.
type StreamMeta struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	URL     string              `json:"url"`
	Name    string              `json:"name"`
	// Length is the declared body size, or -1 when the response did not declare one.
	Length int64 `json:"length"`
}

// DownloadHeader is the one JSON line a download answers with. Exactly one of Inline and Stream is set: Inline carries a response that was not 2xx, Stream announces a body that follows as frames.
type DownloadHeader struct {
	Inline *restergate.HTTPResult `json:"inline,omitempty"`
	Stream *StreamMeta            `json:"stream,omitempty"`
}

// DownloadResult is a download as a caller sees it. A streamed result has Body open and the caller must close it.
type DownloadResult struct {
	Inline *restergate.HTTPResult
	Stream *StreamMeta
	Body   io.ReadCloser
}

// WriteDownloadError answers a download request with a failure.
func WriteDownloadError(conn net.Conn, err error) {
	json.NewEncoder(conn).Encode(errResponse(err))
}

// WriteDownload answers a download request with res: the header line, then for a streamed result the body as frames, ending with an end frame or an abort frame. Every write must complete within idle. redact, when set, rewrites an error before it is sent.
func WriteDownload(conn net.Conn, res DownloadResult, idle time.Duration, redact func(error) error) error {
	if res.Body != nil {
		defer res.Body.Close()
	}
	conn.SetDeadline(time.Now().Add(idle))
	if err := json.NewEncoder(conn).Encode(payloadResponse(DownloadHeader{Inline: res.Inline, Stream: res.Stream})); err != nil {
		return err
	}
	if res.Stream == nil {
		return nil
	}

	frame := make([]byte, 4+frameChunk)
	for {
		n, readErr := res.Body.Read(frame[4:])
		if n > 0 {
			binary.BigEndian.PutUint32(frame[:4], uint32(n))
			conn.SetWriteDeadline(time.Now().Add(idle))
			if _, err := conn.Write(frame[:4+n]); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			conn.SetWriteDeadline(time.Now().Add(idle))
			_, err := conn.Write([]byte{0, 0, 0, 0})
			return err
		}
		if readErr != nil {
			if redact != nil {
				readErr = redact(readErr)
			}
			conn.SetWriteDeadline(time.Now().Add(idle))
			writeAbort(conn, readErr.Error())
			return readErr
		}
	}
}

func writeAbort(conn net.Conn, message string) {
	if len(message) > maxAbortMessage {
		message = message[:maxAbortMessage]
	}
	out := make([]byte, 6+len(message))
	binary.BigEndian.PutUint32(out[:4], abortFrame)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(message)))
	copy(out[6:], message)
	conn.Write(out)
}

// ReadDownload reads a download's header line from conn, which must be at the start of the response. A streamed result owns conn and closes it with its body. Each read of the body must complete within idle.
func ReadDownload(conn net.Conn, idle time.Duration) (DownloadResult, error) {
	br := bufio.NewReader(conn)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return DownloadResult{}, fmt.Errorf("reading response: %w", err)
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return DownloadResult{}, fmt.Errorf("reading response: %w", err)
	}
	if !resp.OK {
		return DownloadResult{}, RemoteError(resp.Error)
	}
	var header DownloadHeader
	if err := json.Unmarshal(resp.Payload, &header); err != nil {
		return DownloadResult{}, fmt.Errorf("reading response: %w", err)
	}
	if header.Stream == nil {
		return DownloadResult{Inline: header.Inline}, nil
	}
	conn.SetDeadline(time.Time{})
	return DownloadResult{Stream: header.Stream, Body: &streamReader{conn: conn, r: br, idle: idle}}, nil
}

// streamReader decodes frames into the body's bytes and reports an abort frame, or a connection that ended without an end frame, as an error.
type streamReader struct {
	conn net.Conn
	r    *bufio.Reader
	idle time.Duration
	left int
	err  error
}

func (s *streamReader) Read(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	for s.left == 0 {
		s.conn.SetReadDeadline(time.Now().Add(s.idle))
		var head [4]byte
		if _, err := io.ReadFull(s.r, head[:]); err != nil {
			return 0, s.fail(fmt.Errorf("the download stopped before its end marker: %w", err))
		}
		switch n := binary.BigEndian.Uint32(head[:]); {
		case n == 0:
			s.err = io.EOF
			return 0, io.EOF
		case n == abortFrame:
			return 0, s.fail(s.readAbort())
		case n > maxFrame:
			return 0, s.fail(fmt.Errorf("download frame of %d bytes is over the %d byte limit", n, maxFrame))
		default:
			s.left = int(n)
		}
	}
	s.conn.SetReadDeadline(time.Now().Add(s.idle))
	n, err := s.r.Read(p[:min(len(p), s.left)])
	s.left -= n
	if err != nil && n == 0 {
		return 0, s.fail(fmt.Errorf("the download stopped inside a frame: %w", err))
	}
	return n, nil
}

func (s *streamReader) readAbort() error {
	var size [2]byte
	if _, err := io.ReadFull(s.r, size[:]); err != nil {
		return fmt.Errorf("the download was aborted: %w", err)
	}
	msg := make([]byte, min(int(binary.BigEndian.Uint16(size[:])), maxAbortMessage))
	if _, err := io.ReadFull(s.r, msg); err != nil {
		return fmt.Errorf("the download was aborted: %w", err)
	}
	return errors.New(string(msg))
}

func (s *streamReader) fail(err error) error {
	s.err = err
	return err
}

func (s *streamReader) Close() error {
	return s.conn.Close()
}

func serveDownload(svc *Backend, conn net.Conn, req request) {
	var p requestHTTPPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		WriteDownloadError(conn, err)
		return
	}
	dl, err := svc.HTTP.DownloadHTTP(p.ID, p.User, p.Method, p.Path, p.Body, p.ContentType, p.Headers)
	if err != nil {
		WriteDownloadError(conn, err)
		return
	}
	WriteDownload(conn, downloadResult(dl), restergate.RequestTimeout, nil)
}

func downloadResult(dl restergate.Download) DownloadResult {
	if dl.Inline != nil {
		return DownloadResult{Inline: dl.Inline}
	}
	return DownloadResult{
		Stream: &StreamMeta{Status: dl.Status, Headers: dl.Headers, URL: dl.URL, Name: dl.Name, Length: dl.Length},
		Body:   dl.Body,
	}
}

// RequestHTTPDownload asks the vault daemon for a download and returns its header, and for a 2xx response the open body.
func (c *Client) RequestHTTPDownload(id vault.ChannelID, user, method, path string, body []byte, contentType string, headers []vault.Header) (DownloadResult, error) {
	timeout := max(c.Timeout, restergate.RequestTimeout)
	conn, err := net.DialTimeout("unix", c.SocketPath, timeout)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("gatekeeper: dialing %s: %w", c.SocketPath, err)
	}
	conn.SetDeadline(time.Now().Add(timeout))

	payload, err := json.Marshal(requestHTTPPayload{ID: id, User: user, Method: method, Path: path, Body: body, ContentType: contentType, Headers: headers})
	if err == nil {
		err = json.NewEncoder(conn).Encode(request{Op: opRequestHTTPDownload, Payload: payload})
	}
	if err != nil {
		conn.Close()
		return DownloadResult{}, fmt.Errorf("gatekeeper: sending request: %w", err)
	}

	res, err := ReadDownload(conn, restergate.RequestTimeout)
	if err != nil || res.Body == nil {
		conn.Close()
	}
	var remote RemoteError
	if err != nil && !errors.As(err, &remote) {
		err = fmt.Errorf("gatekeeper: %w", err)
	}
	return res, err
}

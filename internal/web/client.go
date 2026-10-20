// Package web carries a model's web command from the agent to the browser engine inside the lemongrass app, over the app socket.
package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
)

const opWeb = "web"

// Timeout bounds one web command at every hop. An action can wait for its target (5 s), then for the page to settle (10 s), after a login that runs several steps, so it is well above the 10 second deadline of the other agent ops.
const Timeout = 60 * time.Second

const dialTimeout = time.Second

// Request is the JSON line the app's engine reads. ChannelID is the real channel id, which only the agent and the app ever hold.
type Request struct {
	Op        string            `json:"op"`
	TabID     string            `json:"tab_id"`
	ChannelID string            `json:"channel_id"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Flags     map[string]string `json:"flags"`
}

type reply struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text,omitempty"`
	Data  []byte `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// Result is what a web command returns. Text is what the model reads, and Data holds a screenshot's PNG bytes, empty for every other command.
type Result struct {
	Text string `json:"text"`
	Data []byte `json:"data,omitempty"`
}

// SocketPath is the socket the lemongrass app listens on, the same one `lgrass workgroup` uses.
func SocketPath() string {
	return filepath.Join(config.Dir(), "app.sock")
}

// Call sends one web command to the app at socketPath and waits up to Timeout for its reply.
func Call(socketPath string, req Request) (Result, error) {
	req.Op = opWeb
	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err != nil {
		return Result{}, fmt.Errorf("web: the lemongrass app is not reachable at %s, is it running: %w", socketPath, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(Timeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Result{}, fmt.Errorf("web: sending to the app: %w", err)
	}
	var resp reply
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Result{}, fmt.Errorf("web: reading the app's answer: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			return Result{}, errors.New("web: the app refused the command")
		}
		return Result{}, errors.New("web: " + resp.Error)
	}
	return Result{Text: resp.Text, Data: resp.Data}, nil
}

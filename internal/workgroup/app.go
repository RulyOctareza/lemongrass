package workgroup

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
)

const (
	opPropose = "propose"
	opSpawn   = "spawn"
	opNudge   = "nudge"
	opClaim   = "claim"

	dialTimeout    = time.Second
	proposeTimeout = 10 * time.Minute
	spawnTimeout   = 30 * time.Second
)

type SpawnMember struct {
	TabID  string `json:"tab_id"`
	Label  string `json:"label"`
	Vendor string `json:"vendor"`
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt"`
	// Every skill the tab must load, the thinker skill first.
	Skills []string `json:"skills"`
}

type Request struct {
	Op          string `json:"op"`
	ProjectPath string `json:"project_path,omitempty"`
	LeaderTabID string `json:"leader_tab_id,omitempty"`
	LeaderLabel string `json:"leader_label,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
	// GroupID is set when the members join a live group rather than start a new one, and ExistingMembers then names who is already in it.
	GroupID         int64         `json:"group_id,omitempty"`
	ExistingMembers []string      `json:"existing_members,omitempty"`
	Members         []SpawnMember `json:"members,omitempty"`
	Token           string        `json:"token,omitempty"`
	TabID           string        `json:"tab_id,omitempty"`
}

type Response struct {
	OK       bool   `json:"ok"`
	Approved bool   `json:"approved,omitempty"`
	Token    string `json:"token,omitempty"`
	Error    string `json:"error,omitempty"`
	Typed    bool   `json:"typed,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Withdrawn is true when the request ended without the human answering, such as the app window closing.
	Withdrawn bool `json:"withdrawn,omitempty"`
	// Join is what a claimed join token was minted for.
	Join *JoinGrant `json:"join,omitempty"`
}

// A join the human approved in the app: an open tab becoming a collaborator of a live group.
type JoinGrant struct {
	ProjectPath string `json:"project_path"`
	GroupID     int64  `json:"group_id"`
	TabID       string `json:"tab_id"`
	Label       string `json:"label"`
}

// The socket the lemongrass app listens on for group requests.
func AppSocketPath() string {
	return filepath.Join(config.Dir(), "app.sock")
}

// The human's answer to a proposal. Token is set only on approval. Reason is the optional text the human gave when declining, and Withdrawn means no human answered.
type Answer struct {
	Approved  bool
	Token     string
	Reason    string
	Withdrawn bool
}

// Blocks until the human answers in the app. Nothing has been spawned when it returns, and an approval carries the one-time token that spawns exactly what was shown.
func Propose(socketPath string, req Request) (Answer, error) {
	req.Op = opPropose
	resp, err := call(socketPath, req, proposeTimeout)
	if err != nil {
		return Answer{}, err
	}
	return Answer{Approved: resp.Approved, Token: resp.Token, Reason: resp.Reason, Withdrawn: resp.Withdrawn}, nil
}

// Asks the app to open the approved members' tabs next to the leader's.
func Spawn(socketPath, token string) error {
	_, err := call(socketPath, Request{Op: opSpawn, Token: token}, spawnTimeout)
	return err
}

// Asks the app to type the tab's pending nudge into its terminal. The request carries only the tab id, since the app composes the line itself from the ledger. Typed is false with a reason when the human is typing or nothing is pending.
func Nudge(socketPath, tabID string) (typed bool, reason string, err error) {
	resp, err := call(socketPath, Request{Op: opNudge, TabID: tabID}, spawnTimeout)
	if err != nil {
		return false, "", err
	}
	return resp.Typed, resp.Reason, nil
}

// Redeems a join token the app minted when the human approved, once. Only the app can mint one, so a model running `lgrass workgroup join` itself gets nothing.
func Claim(socketPath, token string) (JoinGrant, error) {
	resp, err := call(socketPath, Request{Op: opClaim, Token: token}, spawnTimeout)
	if err != nil {
		return JoinGrant{}, err
	}
	if resp.Join == nil {
		return JoinGrant{}, errors.New("workgroup: the app returned no join for the token")
	}
	return *resp.Join, nil
}

func call(socketPath string, req Request, deadline time.Duration) (Response, error) {
	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err != nil {
		return Response{}, fmt.Errorf("workgroup: the lemongrass app is not reachable at %s, is it running: %w", socketPath, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(deadline))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("workgroup: sending to the app: %w", err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("workgroup: reading the app's answer: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			return Response{}, errors.New("workgroup: the app refused the request")
		}
		return Response{}, errors.New("workgroup: " + resp.Error)
	}
	return resp, nil
}

// A random version 4 UUID, the same shape the app mints for its own tabs.
func NewTabID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// The text a thinker starts with: its role, the skills it must load, and its assignment. It holds no ids that are only known after the group exists, so the human approves the exact final text.
func ComposePrompt(label, groupName, leaderLabel string, skills []string, assignment string) string {
	return composePrompt(label, groupName, leaderLabel, skills, assignment, "")
}

// ComposeAddedPrompt is ComposePrompt for a thinker added to a live group, who must catch up on a thread that started without it.
func ComposeAddedPrompt(label, groupName, leaderLabel string, skills []string, assignment string) string {
	return composePrompt(label, groupName, leaderLabel, skills, assignment,
		" The group was already working before you joined, so read the whole group thread first with `lgrass workgroup thread --all`, paging back with `--before` until its first message.")
}

func composePrompt(label, groupName, leaderLabel string, skills []string, assignment, joined string) string {
	text := fmt.Sprintf("You are the thinker \"%s\" in the lemongrass workgroup \"%s\", led by the leader \"%s\". "+
		"Before any other tool call, load these skills: %s. Until you have, every tool except `lgrass` commands is denied. "+
		"Then run `lgrass workgroup thread` to see your group and the group thread.%s "+
		"Before you change anything, post your plan to the group thread and wait for the leader's go.",
		label, groupName, leaderLabel, strings.Join(skills, ", "), joined)
	return text + "\n\nYour assignment:\n\n" + assignment
}

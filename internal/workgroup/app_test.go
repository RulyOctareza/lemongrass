package workgroup

import (
	"encoding/json"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func fakeApp(t *testing.T, answer func(Request) Response) (string, chan Request) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	seen := make(chan Request, 4)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			var req Request
			json.NewDecoder(conn).Decode(&req)
			seen <- req
			json.NewEncoder(conn).Encode(answer(req))
			conn.Close()
		}
	}()
	return path, seen
}

func TestProposeReturnsATokenAndSpawnPresentsOnlyIt(t *testing.T) {
	path, seen := fakeApp(t, func(r Request) Response {
		return Response{OK: true, Approved: r.Op == opPropose, Token: "tok-1"}
	})
	req := Request{ProjectPath: "/p", LeaderTabID: "leader-tab", LeaderLabel: "lead", GroupName: "g", Members: []SpawnMember{{TabID: "t1", Label: "a", Vendor: "claude", Prompt: "go"}}}

	answer, err := Propose(path, req)
	token := answer.Token
	if err != nil || !answer.Approved || token != "tok-1" {
		t.Fatalf("Propose = %+v, %v, want an approval with a token", answer, err)
	}
	if got := <-seen; got.Op != "propose" || got.LeaderTabID != "leader-tab" || len(got.Members) != 1 || got.Members[0].Prompt != "go" {
		t.Errorf("propose request = %+v", got)
	}
	if err := Spawn(path, token); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := <-seen; got.Op != "spawn" || got.Token != "tok-1" || len(got.Members) != 0 || got.LeaderTabID != "" {
		t.Errorf("spawn request = %+v, want only the op and token", got)
	}
}

func TestProposeDeclinedIsNotAnError(t *testing.T) {
	path, _ := fakeApp(t, func(Request) Response { return Response{OK: true, Approved: false} })
	answer, err := Propose(path, Request{})
	if err != nil || answer != (Answer{}) {
		t.Errorf("Propose = %+v, %v, want a plain decline", answer, err)
	}
}

func TestProposeCarriesTheDeclineReasonAndWithdrawal(t *testing.T) {
	path, _ := fakeApp(t, func(Request) Response { return Response{OK: true, Reason: "use haiku"} })
	answer, err := Propose(path, Request{})
	if err != nil || answer.Approved || answer.Reason != "use haiku" || answer.Withdrawn || answer.Token != "" {
		t.Errorf("Propose = %+v, %v, want a decline with its reason", answer, err)
	}
	path, _ = fakeApp(t, func(Request) Response { return Response{OK: true, Withdrawn: true} })
	answer, err = Propose(path, Request{})
	if err != nil || answer.Approved || !answer.Withdrawn {
		t.Errorf("Propose = %+v, %v, want a withdrawal", answer, err)
	}
}

func TestAppErrorsAndAnUnreachableAppCarryThePackagePrefix(t *testing.T) {
	path, _ := fakeApp(t, func(Request) Response { return Response{Error: "leader tab not found"} })
	err := Spawn(path, "tok")
	if err == nil || err.Error() != "workgroup: leader tab not found" {
		t.Errorf("Spawn error = %v", err)
	}
	if _, err := Propose(filepath.Join(t.TempDir(), "none.sock"), Request{}); err == nil || !strings.HasPrefix(err.Error(), "workgroup: the lemongrass app is not reachable") {
		t.Errorf("unreachable error = %v", err)
	}
}

func TestNewTabIDIsAVersion4UUID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := NewTabID()
		if !re.MatchString(id) || seen[id] {
			t.Fatalf("tab id %q is malformed or repeated", id)
		}
		seen[id] = true
	}
}

func TestComposePromptNamesTheRoleThreadAndAssignment(t *testing.T) {
	prompt := ComposePrompt("reviewer", "schema-review", "lead", []string{"lgrass-howtobe-thinker", "lgrass-connector"}, "Review the diff.")
	for _, want := range []string{`thinker "reviewer"`, `workgroup "schema-review"`, `leader "lead"`, "lgrass-howtobe-thinker, lgrass-connector", "lgrass workgroup thread", "post your plan to the group thread and wait for the leader's go", "Review the diff."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "lgrass listen") {
		t.Error("the thinker prompt asks for a listener")
	}
}

func TestComposeAddedPromptTellsTheThinkerToCatchUp(t *testing.T) {
	added := ComposeAddedPrompt("docs", "schema-review", "lead", []string{"lgrass-howtobe-thinker"}, "Write docs.")
	plain := ComposePrompt("docs", "schema-review", "lead", []string{"lgrass-howtobe-thinker"}, "Write docs.")
	for _, want := range []string{"already working before you joined", "lgrass workgroup thread --all", "wait for the leader's go", "Write docs."} {
		if !strings.Contains(added, want) {
			t.Errorf("added prompt missing %q", want)
		}
	}
	if strings.Contains(plain, "before you joined") {
		t.Error("a founding thinker was told it joined late")
	}
}

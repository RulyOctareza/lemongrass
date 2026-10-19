package hook

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/vault"
)

func startVault(t *testing.T) *gatekeeper.Client {
	t.Helper()
	svc, err := gatekeeper.NewBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go gatekeeper.Serve(svc, l, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	t.Cleanup(func() { l.Close() })
	return &gatekeeper.Client{SocketPath: sock}
}

func TestHookPolicyReadsTheVaultsPolicy(t *testing.T) {
	client := startVault(t)
	if got := hookPolicyFrom(client.SocketPath, ""); len(got.Binaries) != 0 {
		t.Errorf("policy before the vault has one = %+v", got)
	}
	if err := client.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	if err := client.PutPolicy("pw", "", guard.Policy{Binaries: []string{"make"}}); err != nil {
		t.Fatal(err)
	}
	policy := hookPolicyFrom(client.SocketPath, "")
	if len(policy.Binaries) != 1 || policy.Binaries[0] != "make" {
		t.Fatalf("policy = %+v", policy)
	}
	verdict := guard.Decide(guard.Input{Tool: "Bash", Command: "make build", Cwd: t.TempDir(), Home: t.TempDir()}, policy)
	if verdict == nil || verdict.RuleID != "custom-binary" {
		t.Errorf("verdict = %+v, want custom-binary", verdict)
	}
}

func TestHookPolicyCombinesGlobalWithTheLaunchProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	proj, err := project.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	client := startVault(t)
	if err := client.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	if err := client.PutPolicy("pw", "", guard.Policy{Binaries: []string{"make"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.PutPolicy("pw", proj.ID, guard.Policy{Binaries: []string{"cargo"}, Rules: map[string]string{"rm-plain": "allow"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(claudeProjectDirEnv, root)
	launch := launchProject()
	if launch.ID != proj.ID {
		t.Fatalf("launchProject = %+v, want %s", launch, proj.ID)
	}
	if got := hookPolicyFrom(client.SocketPath, launch.ID); len(got.Binaries) != 2 || got.Binaries[0] != "make" || got.Binaries[1] != "cargo" || got.Rules["rm-plain"] != "allow" {
		t.Errorf("launch project policy = %+v", got)
	}
	if got := hookPolicyFrom(client.SocketPath, ""); len(got.Binaries) != 1 || got.Binaries[0] != "make" || len(got.Rules) != 0 {
		t.Errorf("no launch project = %+v, want global over the defaults", got)
	}
}

func TestHookPolicyFallsBackToDefaultsWhenTheVaultIsUnreachable(t *testing.T) {
	got := hookPolicyFrom(filepath.Join(t.TempDir(), "missing.sock"), "")
	if len(got.Binaries)+len(got.Paths)+len(got.Rules) != 0 {
		t.Errorf("policy = %+v, want empty", got)
	}
}

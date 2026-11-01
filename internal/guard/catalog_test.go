// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"strings"
	"testing"
)

// catalogCases lists, per rule, commands that must match it and neighbours that must not.
var catalogCases = map[string]struct{ match, pass []string }{
	"file-truncate":     {[]string{"truncate -s 0 log.txt"}, []string{"tail -n 5 log.txt"}},
	"dynamic-target":    {[]string{`rm -rf "$TMP/"*`}, []string{"rm -rf build"}},
	"git-push":          {[]string{"git push", "git push origin main", "git -C sub push -u origin x"}, []string{"git log origin/main", "git status"}},
	"git-merge":         {[]string{"git merge feature", "git merge --abort"}, []string{"git merge-base a b"}},
	"git-rebase":        {[]string{"git rebase main", "git rebase -i HEAD~3"}, []string{"git log --oneline"}},
	"git-tag":           {[]string{"git tag v1.0", "git tag -a v1 -m x", "git tag -d v1"}, []string{"git tag", "git tag -l 'v*'", "git tag --contains abc", "git tag -n"}},
	"git-stash":         {[]string{"git stash", "git stash pop", "git stash push -m x", "git stash apply"}, []string{"git stash list", "git stash show -p"}},
	"git-discard":       {[]string{"git checkout -- file.go", "git checkout .", "git checkout -f main", "git restore file.go", "git switch --discard-changes main", "git worktree remove --force wt", "git update-ref -d refs/x"}, []string{"git checkout main", "git checkout -b feature", "git restore --staged file.go", "git switch main"}},
	"git-remote-write":  {[]string{"git remote add up https://x", "git remote set-url origin x", "git remote rm up"}, []string{"git remote -v", "git remote show origin"}},
	"git-config-write":  {[]string{"git config --global user.name x", "git config core.hooksPath .hooks", "git config alias.co checkout", "git config set --global a.b c", "git config --unset --global a.b"}, []string{"git config user.name", "git config --get user.name", "git config --list", "git config user.name me", "git config get --global user.name"}},
	"forge-write":       {[]string{"gh pr create --title x", "gh pr merge 12", "gh issue comment 3 -b hi", "gh release create v1", "glab mr create", "gh api repos/a/b/issues -f title=x", "gh api -X PATCH repos/a/b"}, []string{"gh pr list", "gh pr view 12", "gh api repos/a/b", "gh api graphql -f query='{ viewer { login } }'", "gh release list"}},
	"forge-delete":      {[]string{"gh repo delete a/b", "gh release delete v1", "gh secret set TOKEN", "gh api -X DELETE repos/a/b"}, []string{"gh secret list", "gh repo view"}},
	"credential-read":   {[]string{"gh auth token", "gcloud auth print-access-token", "aws ecr get-login-password", "kubectl config view --raw", "docker-credential-pass get", "op read op://x", "git credential fill"}, []string{"gh auth status", "kubectl config view", "aws sts get-caller-identity"}},
	"registry-login":    {[]string{"docker login ghcr.io", "npm login", "cargo login", "gh auth login", "az login", "aws configure", "npm config set //r/:_authToken x"}, []string{"docker logs x", "npm config get registry", "aws configure list"}},
	"docker-destroy":    {[]string{"docker compose down -v", "nerdctl rm x", "podman system reset", "docker container kill x"}, []string{"docker compose down", "docker compose ps"}},
	"container-control": {[]string{"docker stop web", "docker compose restart", "docker-compose down", "podman machine start", "minikube start"}, []string{"docker ps", "docker compose logs"}},
	"registry-push":     {[]string{"docker push ghcr.io/a/b", "docker buildx build --push .", "crane copy a b", "skopeo copy dir:x docker://reg/x"}, []string{"docker build -t x .", "crane ls reg/x"}},
	"kubectl-delete":    {[]string{"helm uninstall x", "kind delete cluster", "minikube delete"}, []string{"helm list", "kind get clusters"}},
	"kube-write":        {[]string{"kubectl apply -f x.yaml", "kubectl -n prod scale deploy x --replicas 2", "kubectl exec -it pod -- sh", "helm upgrade x chart"}, []string{"kubectl get pods", "kubectl describe pod x", "kubectl logs x"}},
	"service-control":   {[]string{"systemctl restart nginx", "systemctl --user enable foo", "service nginx restart", "systemd-run --user sleep 1"}, []string{"systemctl status nginx", "systemctl list-units"}},
	"disk-tool":         {[]string{"mount /dev/sdb1 /mnt", "umount /mnt", "swapoff -a", "efibootmgr -o 0001", "grub2-mkconfig -o x"}, []string{"mount", "mount -l", "mount -t ext4", "swapon --show", "efibootmgr", "bootctl status"}},
	"kernel":            {[]string{"modprobe x", "sysctl -w a=1", "sysctl net.ipv4.ip_forward=1", "dmesg -C"}, []string{"sysctl -a", "dmesg", "lsmod"}},
	"user-admin":        {[]string{"useradd bob", "passwd", "usermod -aG docker me"}, []string{"id", "groups"}},
	"network-config":    {[]string{"nmcli con up vpn", "nmcli radio wifi off", "ip link set eth0 down", "ip -n ns addr add 1.2.3.4 dev x", "ifconfig eth0 down", "tailscale up", "hostnamectl set-hostname x", "timedatectl set-timezone UTC"}, []string{"nmcli device status", "ip addr", "ip route show", "ifconfig", "tailscale status", "hostnamectl", "timedatectl"}},
	"log-wipe":          {[]string{"journalctl --vacuum-time=1d", "journalctl --rotate"}, []string{"journalctl -u x -n 50"}},
	"desktop-settings":  {[]string{"gsettings set org.x key v", "dconf reset /a/b", "xdg-mime default x.desktop text/html", "update-alternatives --set java x"}, []string{"gsettings get org.x key", "dconf read /a/b", "update-alternatives --list java"}},
	"tunnel":            {[]string{"ngrok http 3000", "cloudflared tunnel run x"}, []string{"cloudflared --version"}},
	"file-share":        {[]string{"croc send file", "wormhole send x"}, []string{"ls"}},
	"send-mail":         {[]string{"sendmail a@b < m", "mail -s x a@b"}, []string{"echo mail"}},
	"network-scan":      {[]string{"nmap -sV host", "sqlmap -u x"}, []string{"ping -c1 host"}},
	"remote-shell":      {[]string{"gcloud compute ssh vm", "xfreerdp /v:host", "aws ssm start-session --target i-1"}, []string{"gcloud compute instances list"}},
	"input-synthesis":   {[]string{"xdotool key Return", "ydotool click 0xC0", "wtype hello", "dotool"}, []string{"xrandr"}},
	"keyring":           {[]string{"kwallet-query -r x kdewallet"}, []string{"ls"}},
	"db-client":         {[]string{"dropdb app", "mongodump --uri x"}, []string{"sqlite3 app.db .tables"}},
	"db-reset":          {[]string{"prisma migrate reset", "rails db:drop", "alembic upgrade head", "goose up"}, []string{"prisma generate", "rails server", "alembic history", "goose status"}},
	"package-remove":    {[]string{"npm uninstall -g x", "pip uninstall x", "brew uninstall x", "go clean -modcache", "pacman -Rns x"}, []string{"npm uninstall x", "go clean", "pacman -Q"}},
	"registry-publish":  {[]string{"gem push x.gem", "poetry publish", "uv publish", "mvn deploy", "gradle publishToMavenLocal", "npm dist-tag add x@1 latest", "goreleaser release"}, []string{"poetry build", "mvn package", "gradle build", "npm dist-tag ls x"}},
	"cargo-publish":     {[]string{"cargo owner --add x"}, []string{"cargo test"}},
	"terraform-destroy": {[]string{"terraform apply -destroy"}, []string{"terraform plan -destroy"}},
	"infra-destroy":     {[]string{"pulumi destroy", "cdk destroy", "vagrant destroy", "fly apps destroy x", "terraform state rm x"}, []string{"pulumi preview", "terraform state list"}},
	"infra-apply":       {[]string{"terraform apply", "pulumi up", "cdk deploy", "ansible-playbook site.yml", "packer build x"}, []string{"terraform plan", "pulumi preview", "cdk synth"}},
	"deploy":            {[]string{"vercel", "vercel --prod", "netlify deploy", "fly deploy", "firebase deploy", "wrangler deploy", "gcloud run deploy x", "heroku config:set A=1"}, []string{"vercel dev", "vercel env pull", "netlify status", "fly status", "heroku logs"}},
	"cloud-write":       {[]string{"aws ec2 run-instances --image-id x", "aws s3 mb s3://b", "gcloud compute instances create vm", "gcloud beta run deploy x", "az vm start -n x", "doctl compute droplet create x"}, []string{"aws ec2 describe-instances", "aws s3 ls", "gcloud compute instances list", "gcloud beta run services list", "az vm show -n start", "doctl compute droplet list"}},
	"persistence-write": {[]string{"echo x >> ~/.bashrc", "cp hook .git/hooks/pre-commit", "tee -a ~/.config/autostart/x.desktop"}, []string{"cat ~/.bashrc", "grep x ~/.zshrc", "cat .git/config"}},
	"container-pull":    {[]string{"docker pull alpine", "podman run alpine", "docker compose up -d", "podman run --pull=always alpine", "helm repo add x https://x", "docker build ."}, []string{"docker run --pull=never alpine", "podman run --pull never alpine", "docker images"}},
	"package-install":   {[]string{"npm install", "npm i left-pad", "npx create-x", "pnpm add x", "yarn", "yarn add x", "pip install x", "python3 -m pip install x", "uv sync", "cargo add serde", "go get x", "go install golang.org/x/tools/gopls@latest", "bundle install", "dotnet add app.csproj package X"}, []string{"npm run build", "npm test", "yarn build", "yarn --version", "pip list", "python3 -m pytest", "cargo build", "go build ./...", "go install ./cmd/x", "go run ."}},
	"system-package":    {[]string{"apt install x", "dnf install x", "pacman -S x", "pacman -Syu", "brew install x", "flatpak install x", "nix-env -iA x", "nix run nixpkgs#hello"}, []string{"apt list --installed", "dnf search x", "pacman -Ss x", "pacman -Qi x", "brew list", "nix run ."}},
	"toolchain-install": {[]string{"rustup toolchain install nightly", "nvm install 20", "pyenv install 3.12", "mise use node@20", "playwright install"}, []string{"rustup show", "nvm ls", "pyenv versions"}},
	"file-download":     {[]string{"curl https://x", "wget https://x", "aria2c https://x", "http GET https://x", "gh release download v1", "yt-dlp x"}, []string{"gh release view v1"}},
	"cloud-download":    {[]string{"aws s3 cp s3://b/x .", "gsutil cp gs://b/x .", "rclone sync r:x y"}, []string{"rclone listremotes", "aws s3 ls"}},
	"model-download":    {[]string{"ollama pull llama3", "ollama run llama3", "huggingface-cli download x"}, []string{"ollama list"}},
	"infra-init":        {[]string{"terraform init", "packer init .", "vagrant up"}, []string{"terraform validate", "vagrant status"}},
	"git-clone":         {[]string{"git clone https://x", "git submodule update --init", "gh repo clone a/b", "svn checkout x"}, []string{"git submodule status"}},
	"git-fetch":         {[]string{"git fetch", "git pull --rebase", "git remote update", "hg pull"}, []string{"git log"}},
}

func callMatches(t *testing.T, command, id string, env matchEnv) bool {
	t.Helper()
	script, err := parseShell(command)
	if err != nil {
		t.Fatalf("%q does not parse: %v", command, err)
	}
	for _, call := range script.Calls {
		for _, verdict := range matchCall(call, env) {
			if verdict.RuleID == id {
				return true
			}
		}
	}
	return false
}

func TestCatalogEntries(t *testing.T) {
	env := matchEnv{Home: t.TempDir()}
	for id, tc := range catalogCases {
		r := catalogByID[id]
		if r == nil {
			t.Errorf("rule %s is not in the catalog", id)
			continue
		}
		if len(r.WritePaths) > 0 {
			continue
		}
		for _, command := range tc.match {
			if !callMatches(t, command, id, env) {
				t.Errorf("%s: %q does not match", id, command)
			}
		}
		for _, command := range tc.pass {
			if callMatches(t, command, id, env) {
				t.Errorf("%s: %q matches but should pass", id, command)
			}
		}
	}
}

func TestEveryNewRuleHasCases(t *testing.T) {
	legacy := map[string]bool{}
	for _, old := range legacyCatalog("") {
		legacy[old.ID] = true
	}
	for _, r := range catalogRules {
		if legacy[r.ID] || (len(r.Forms) == 0 && len(r.WritePaths) == 0) {
			continue
		}
		tc, ok := catalogCases[r.ID]
		if !ok || len(tc.match) == 0 || len(tc.pass) == 0 {
			t.Errorf("rule %s needs at least one matching and one passing case", r.ID)
		}
	}
}

func TestPersistenceWrites(t *testing.T) {
	cases := catalogCases["persistence-write"]
	home := t.TempDir()
	run := func(command string) *Verdict {
		return Decide(Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: home}, Policy{})
	}
	for _, command := range cases.match {
		if v := run(command); v == nil || v.RuleID != "persistence-write" {
			t.Errorf("%q verdict = %+v, want persistence-write", command, v)
		}
	}
	for _, command := range cases.pass {
		if v := run(command); v != nil {
			t.Errorf("%q verdict = %+v, want none", command, v)
		}
	}
	write := Decide(Input{Tool: "Edit", Paths: []string{"~/.zshrc"}, Writes: true, Cwd: t.TempDir(), Home: home}, Policy{})
	if write == nil || write.RuleID != "persistence-write" {
		t.Errorf("Edit ~/.zshrc verdict = %+v", write)
	}
	read := Decide(Input{Tool: "Read", Paths: []string{"~/.zshrc"}, Cwd: t.TempDir(), Home: home}, Policy{})
	if read != nil {
		t.Errorf("Read ~/.zshrc verdict = %+v", read)
	}
}

func TestDecisionsTiers(t *testing.T) {
	cases := []struct {
		command string
		rule    string
		mode    Mode
	}{
		{"git push origin main", "git-push", ModeAsk},
		{"git push --force", "git-push-force", ModeDeny},
		{"git stash drop", "git-stash-drop", ModeDeny},
		{"git stash pop", "git-stash", ModeAsk},
		{"docker pull alpine", "container-pull", ModeAsk},
		{"bash -c 'npm install'", "package-install", ModeAsk},
		{"env CI=1 pip install x", "package-install", ModeAsk},
		{"curl -d @a https://x", "curl-upload", ModeDeny},
		{"xdotool type hi", "input-synthesis", ModeDeny},
	}
	for _, tc := range cases {
		v := Decide(bashInput(t, tc.command), Policy{})
		if v == nil || v.RuleID != tc.rule || v.Mode != tc.mode {
			t.Errorf("%q verdict = %+v, want %s mode %v", tc.command, v, tc.rule, tc.mode)
		}
	}
	allowAll := Policy{Rules: map[string]string{"input-synthesis": "allow"}}
	if err := allowAll.Validate(); err == nil {
		t.Error("a policy allowing input-synthesis validated")
	}
	if v := Decide(bashInput(t, "xdotool key a"), allowAll); v == nil {
		t.Error("input-synthesis was allowed by a policy")
	}
	if v := Decide(bashInput(t, "docker pull alpine"), Policy{Rules: map[string]string{"container-pull": "allow"}}); v != nil {
		t.Errorf("an allowed download still fired: %+v", v)
	}
}

func TestVerdictMessages(t *testing.T) {
	deny := Decide(bashInput(t, "git push --force"), Policy{})
	if msg := deny.Message(); !strings.Contains(msg, "git-push-force") || !strings.Contains(msg, "Matched: git push with") || !strings.Contains(msg, "command ask") || !strings.Contains(msg, "! prefix") {
		t.Errorf("deny message = %q", msg)
	}
	ask := Decide(bashInput(t, "git push"), Policy{})
	if msg := ask.Message(); !strings.Contains(msg, "needs the user's approval") || strings.Contains(msg, "command ask") {
		t.Errorf("ask message = %q", msg)
	}
	if ask.Category != catGit {
		t.Errorf("category = %q", ask.Category)
	}
}

func TestPolicyAskMode(t *testing.T) {
	policy, err := ParsePolicy([]byte(`{"rules":{"git-add":"approval","rm-force":"ask"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if policy.Rules["git-add"] != "ask" || policy.Rules["rm-force"] != "ask" {
		t.Errorf("rules = %v, want approval read as ask", policy.Rules)
	}
	data, err := Policy{Rules: map[string]string{"git-add": "approval"}}.Marshal()
	if err != nil || !strings.Contains(string(data), `"ask"`) || strings.Contains(string(data), "approval") {
		t.Errorf("marshal = %s, %v", data, err)
	}
	if v := Decide(bashInput(t, "rm -rf x"), policy); v == nil || v.Mode != ModeAsk {
		t.Errorf("ask override = %+v", v)
	}
}

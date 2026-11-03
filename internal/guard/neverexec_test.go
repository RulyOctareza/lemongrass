// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "testing"

func TestNeverExecute(t *testing.T) {
	const home = "/home/tester"
	const cwd = "/home/tester/work/app"
	cases := []struct {
		name    string
		command string
		refused bool
	}{
		{"rm -rf root", "rm -rf /", true},
		{"rm root glob", "rm -rf /*", true},
		{"rm no preserve root", "rm -rf --no-preserve-root /tmp/x", true},
		{"rm home", "rm -rf ~", true},
		{"rm home var", "rm -rf $HOME", true},
		{"rm home slash", "rm -r /home/tester/", true},
		{"rm home glob", "rm -rf ~/*", true},
		{"rm ssh", "rm ~/.ssh/id_ed25519", true},
		{"rm system dir", "rm -rf /usr", true},
		{"rm system dir slash", "rm -rf /usr/", true},
		{"chmod -R system dir slash", "chmod -R 777 /etc/", true},
		{"rm dynamic recursive", `rm -rf "$BUILD"/`, true},
		{"rm dynamic plain", `rm "$FILE"`, false},
		{"rm project dir", "rm -rf build", false},
		{"rm tmp", "rm -rf /tmp/cache", false},
		{"rm under home", "rm -rf ~/work/app/node_modules", false},
		{"sudo rm root", "sudo rm -rf /", true},
		{"sudo with flag", "sudo -E rm -rf /", true},
		{"bash -c rm root", `bash -c "rm -rf /"`, true},
		{"chained", "cd /tmp && rm -rf /", true},
		{"mv home", "mv ~ /tmp/old", true},
		{"mv file", "mv a.txt b.txt", false},
		{"shred ssh", "shred -u ~/.ssh/id_rsa", true},
		{"shred file", "shred secret.txt", false},
		{"dd to disk", "dd if=/dev/zero of=/dev/sda bs=1M", true},
		{"dd to file", "dd if=/dev/zero of=disk.img bs=1M count=10", false},
		{"redirect to disk", "echo x > /dev/nvme0n1", true},
		{"redirect into ssh", "echo key >> ~/.ssh/authorized_keys", true},
		{"mkfs", "mkfs.ext4 /dev/sdb1", true},
		{"wipefs", "wipefs -a /dev/sdb", true},
		{"chmod -R root", "chmod -R 777 /", true},
		{"chown -R home", "chown -R nobody ~", true},
		{"chmod -R project", "chmod -R u+w build", false},
		{"kill everything", "kill -9 -1", true},
		{"kill a pid", "kill -9 1234", false},
		{"fork bomb", ":(){ :|:& };:", true},
		{"docker privileged", "docker run --privileged alpine sh", true},
		{"docker root mount", "docker run -v /:/host alpine", true},
		{"docker root mount joined", "docker run -v/:/host alpine", true},
		{"docker socket mount", "docker run -v /var/run/docker.sock:/var/run/docker.sock alpine", true},
		{"docker mount flag", "docker run --mount type=bind,source=/,target=/host alpine", true},
		{"docker pid host", "docker run --pid=host alpine", true},
		{"docker cap sys admin", "docker run --cap-add SYS_ADMIN alpine", true},
		{"podman privileged", "podman container run --privileged alpine", true},
		{"docker plain run", "docker run --rm -v ./data:/data alpine ls", false},
		{"docker pull", "docker pull postgres:16", false},
		{"git push", "git push origin main", false},
		{"npm install", "npm install", false},
		{"unparseable", "echo 'unterminated", true},
		{"dynamic name", "$CMD --all", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, refused := NeverExecute(tc.command, cwd, home)
			if refused != tc.refused {
				t.Fatalf("NeverExecute(%q) = %q, %v, want refused %v", tc.command, reason, refused, tc.refused)
			}
			if refused && reason == "" {
				t.Fatalf("NeverExecute(%q) refused with no reason", tc.command)
			}
		})
	}
}

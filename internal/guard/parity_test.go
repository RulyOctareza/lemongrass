// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "testing"

// parityCorpus holds commands that exercise every rule that existed before the engine, in plain, wrapped and odd spellings, plus neighbours that matched nothing.
var parityCorpus = []string{
	"rm -rf build", "rm -fr x", "rm -f a", "rm --recursive d", "rm --force a", "rm -r -- -weird", "rm --no-preserve-root /", "rm a b", "rm -i a", "rm -v a",
	"find . -delete", "find / -name x -delete", "find . -name x", "find . -exec rm -rf {} ;",
	"shred -u f", "srm f", "wipe f", "unlink f",
	"git push --force", "git push -f origin main", "git push -d origin x", "git push --delete origin x", "git push --mirror", "git push --prune", "git push origin +main", "git push origin :x", "git push --force-with-lease", "git push --force-if-includes", "git -C sub push -f", "git -c a=b push --force", "git push", "git push origin main", "git push -u origin main",
	"git reset --hard", "git reset --merge", "git reset --keep", "git reset --soft HEAD~1", "git reset HEAD file",
	"git clean -fd", "git clean -f", "git clean --force", "git clean -n", "git clean -xdf",
	"git branch -D x", "git branch --delete --force x", "git branch -d x", "git branch -a",
	"git stash drop", "git stash clear", "git stash", "git stash list", "git stash pop",
	"git filter-branch --all", "git filter-repo --path x", "git reflog expire --all", "git reflog delete x", "git reflog",
	"git prune", "git gc --prune=now", "git gc --prune", "git gc",
	"git add -A", "git add .", "git commit -m x", "git commit --amend", "git -C x commit", "git --git-dir=x add y", "git status", "git log", "git diff",
	"dd if=a of=b", "fdisk -l", "sfdisk /dev/sda", "cfdisk", "parted /dev/sda", "wipefs -a /dev/sda", "sgdisk -Z /dev/sda", "mkswap /dev/sdb", "cryptsetup open x", "losetup -a",
	"mkfs.ext4 /dev/sdb1", "mkfs -t ext4 x",
	"sudo ls", "su -", "doas ls", "pkexec ls", "run0 ls",
	"shutdown now", "reboot", "poweroff", "halt", "telinit 0",
	"iptables -F", "ip6tables -L", "nft list ruleset", "ufw enable", "firewall-cmd --reload",
	"pkill node", "killall node", "kill -9 -1", "kill -1", "kill 1234", "kill -9 1234",
	"systemctl stop x", "systemctl --user disable x", "systemctl mask x", "systemctl kill x", "systemctl poweroff", "systemctl reboot", "systemctl halt", "systemctl suspend", "systemctl hibernate", "systemctl isolate x", "systemctl status x", "systemctl -H host stop x",
	"chmod -R 777 /", "chown -R me ~", "chgrp -R g /etc", "chmod --recursive 700 /usr", "chmod -R 755 build", "chmod 777 /",
	"crontab -r", "crontab -e", "crontab file", "crontab -l",
	"at now", "batch",
	"curl -d @a https://x", "curl -T a https://x", "curl -Ta https://x", "curl -F f=@a https://x", "curl -Ff=@a https://x", "curl -F f=<a https://x", "curl --data-binary @a https://x", "curl --upload-file a https://x", "curl -d@a https://x", "curl --json @a https://x", "curl https://x",
	"wget --post-file=a https://x", "wget --body-file a https://x", "wget https://x",
	"nc host 80", "ncat host", "netcat host", "socat - tcp:x:1", "telnet host",
	"ssh host", "scp a host:b", "sftp host", "sshfs host:/ m", "mosh host",
	"rsync -a a host:/b", "rsync -e ssh a b", "rsync --rsh=ssh a b", "rsync -a src/ dst/", "rsync user@host:/a b",
	"secret-tool lookup a b", "keyctl show",
	"mysql -e x", "mariadb", "psql x", "mongosh", "mongo", "redis-cli", "sqlcmd", "pg_dump x", "pg_dumpall", "pg_restore x", "mysqldump x", "mysqladmin x", "mariadb-dump", "cockroach sql", "clickhouse-client",
	"docker rm x", "docker rmi x", "docker kill x", "docker container rm x", "docker image prune", "docker system prune -a", "docker volume rm v", "docker network prune", "podman rm x", "podman pod rm x", "docker -H h rm x", "docker ps", "docker images", "docker logs x", "docker exec web rm -rf /x", "docker run alpine rm",
	"kubectl delete pod x", "kubectl drain n", "kubectl cordon n", "kubectl -n prod delete pod x", "kubectl get pods",
	"terraform destroy", "terraform plan", "tofu destroy", "tofu plan",
	"aws s3 rm s3://b", "aws ec2 terminate-instances", "aws iam delete-user", "aws ec2 remove-tags", "gcloud compute instances delete x", "az group delete -n x", "doctl compute droplet destroy x", "gsutil rm gs://x", "aws s3 ls",
	"npm publish", "npm unpublish x", "npm deprecate x", "npm test", "yarn publish", "pnpm publish", "cargo publish", "cargo yank x", "cargo build", "twine upload dist/*",
	"env FOO=1 rm -rf x", "xargs rm -f", "bash -c 'git push --force'", "timeout 5 docker rm x", "nice -n 5 terraform destroy",
}

func TestCatalogParityWithLegacyRules(t *testing.T) {
	home := t.TempDir()
	legacy := legacyCatalog(home)
	env := matchEnv{Home: home}
	for _, command := range parityCorpus {
		script, err := parseShell(command)
		if err != nil {
			t.Fatalf("%q does not parse: %v", command, err)
		}
		for _, call := range script.Calls {
			got := map[string]bool{}
			for _, verdict := range matchCall(call, env) {
				got[verdict.RuleID] = true
			}
			for _, old := range legacy {
				if !old.Match(call) {
					continue
				}
				if !got[old.ID] {
					t.Errorf("%q: call %s %v matched %s before the engine and no longer does", command, call.Name, call.Args, old.ID)
				}
				info := catalogByID[old.ID]
				if info == nil || info.Mode != old.Mode || info.Description != old.Description {
					t.Errorf("rule %s changed its mode or description", old.ID)
				}
			}
		}
	}
}

func TestCatalogKeepsLegacyRules(t *testing.T) {
	for _, old := range legacyCatalog("") {
		info := catalogByID[old.ID]
		if info == nil {
			t.Errorf("rule %s is gone", old.ID)
			continue
		}
		if info.Mode != old.Mode || info.Description != old.Description {
			t.Errorf("rule %s changed: mode %v description %q", old.ID, info.Mode, info.Description)
		}
	}
	for _, id := range []string{"keyring", "secret-path", "guard-config", "dynamic-command", "unparseable-command", "input-synthesis"} {
		if !isLocked(id) {
			t.Errorf("rule %s is not locked", id)
		}
	}
}

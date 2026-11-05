// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

var systemRules = []rule{
	{ID: "disk-tool", Category: catSystem, Description: "a raw disk or filesystem tool", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"dd", "fdisk", "sfdisk", "cfdisk", "parted", "wipefs", "sgdisk", "mkswap", "cryptsetup", "losetup"}},
		{Tools: []string{"grub-install", "grub2-install", "update-grub", "grub-mkconfig", "grub2-mkconfig"}},
		{Tools: []string{"mount", "umount"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			return matched(len(positionals(call)) > 0 || hasShortFlag(call.Args, "a") || hasLongFlag(call.Args, "--all"))
		}},
		{Tools: []string{"swapon", "swapoff"}, Unless: []string{"--show", "-s", "--summary"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			return matched(len(call.Args) > 0)
		}},
		{Tools: []string{"efibootmgr"}, Flags: []string{"-c", "-b", "-B", "-o", "-n", "-N", "-a", "-A", "-O", "-t", "-T", "--create", "--delete-bootnum", "--bootorder", "--bootnext", "--active", "--inactive", "--timeout"}},
		{Tools: []string{"bootctl"}, Subs: []string{"install", "update", "remove", "set-default", "set-oneshot", "set-timeout", "random-seed", "unlink", "cleanup"}},
		{Tools: []string{"diskpart", "mountvol", "bcdboot"}},
		{Tools: []string{"cipher"}, Match: winOptionForm("w")},
		{Tools: []string{"vssadmin", "wbadmin"}, Match: firstWordIn("delete", "resize")},
		{Tools: []string{"bcdedit"}, Match: winOptionForm("set", "delete", "deletevalue", "create", "import", "default", "bootsequence", "displayorder")},
		{Tools: []string{"clear-disk", "initialize-disk", "remove-partition", "clear-recyclebin"}},
	}},
	{ID: "mkfs", Category: catSystem, Description: "creating a filesystem", Mode: ModeDeny, Forms: []form{{ToolPrefixes: []string{"mkfs"}}, {Tools: []string{"format", "format-volume"}}}},
	{ID: "privilege", Category: catSystem, Description: "privilege escalation", Mode: ModeDeny, Forms: []form{{Tools: []string{"sudo", "su", "doas", "pkexec", "run0", "runas", "gsudo"}}}},
	{ID: "power", Category: catSystem, Description: "shutting down or rebooting the machine", Mode: ModeDeny, Forms: []form{{Tools: []string{"shutdown", "reboot", "poweroff", "halt", "telinit", "stop-computer", "restart-computer", "logoff"}}}},
	{ID: "firewall", Category: catSystem, Description: "changing the firewall", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"iptables", "ip6tables", "nft", "ufw", "firewall-cmd"}},
		{Tools: []string{"netsh"}, Match: netshWrite("advfirewall", "firewall")},
		{Tools: []string{"new-netfirewallrule", "set-netfirewallrule", "remove-netfirewallrule", "set-netfirewallprofile", "disable-netfirewallrule"}},
	}},
	{ID: "kill-by-name", Category: catSystem, Description: "killing processes by name", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"pkill", "killall"}},
		{Tools: []string{"taskkill"}, Match: winOptionForm("im")},
		{Tools: []string{"stop-process", "kill"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			return matched(psParameter(call.Args, "name", 2) != "" || psParameter(call.Args, "processname", 2) != "")
		}},
	}},
	{ID: "kill-all", Category: catSystem, Description: "killing every process", Mode: ModeDeny, Forms: []form{{Tools: []string{"kill"}, Args: []string{"-1"}}}},
	{ID: "systemctl-stop", Category: catSystem, Description: "stopping, disabling or masking a service", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"systemctl"}, Subs: []string{"stop", "disable", "mask", "kill", "poweroff", "reboot", "halt", "suspend", "hibernate", "isolate"}},
		{Tools: []string{"sc"}, Match: firstWordIn("stop", "delete", "config", "failure", "pause")},
		{Tools: []string{"net"}, Match: firstWordIn("stop", "pause")},
		{Tools: []string{"stop-service", "remove-service", "suspend-service", "set-service"}},
	}},
	{ID: "recursive-perms", Category: catSystem, Description: "recursive chmod or chown on a broad path", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"chmod", "chown", "chgrp"}, Flags: []string{"-R", "--recursive"}, Match: func(call shellCall, env matchEnv) (string, bool) {
			return matched(broadRecursiveTarget(call.Args, env.Home))
		}},
	}},
	{ID: "crontab", Category: catSystem, Description: "changing the crontab", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"crontab"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			return matched(!(len(call.Args) == 1 && call.Args[0] == "-l"))
		}},
	}},
	{ID: "at-jobs", Category: catSystem, Description: "scheduling jobs with at", Mode: ModeDeny, Forms: []form{{Tools: []string{"at", "batch"}}}},
	// The keeper is itself a scheduled task on Windows, so changing scheduled tasks is denied like the crontab.
	{ID: "scheduled-task", Category: catSystem, Description: "creating, changing or running a Windows scheduled task", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"schtasks"}, Match: winOptionForm("create", "delete", "change", "run", "end")},
		{Tools: []string{"register-scheduledtask", "unregister-scheduledtask", "set-scheduledtask", "start-scheduledtask", "stop-scheduledtask", "disable-scheduledtask"}},
	}},
	{ID: "registry-write", Category: catSystem, Description: "changing the Windows registry", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"reg"}, Match: firstWordIn("add", "delete", "import", "restore", "load", "unload", "copy")},
		{Tools: []string{"regedit", "regini"}},
		{Tools: []string{"set-itemproperty", "new-itemproperty", "remove-itemproperty", "rename-itemproperty"}},
	}},
	{ID: "security-settings", Category: catSystem, Description: "changing Windows security settings or Defender", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"set-executionpolicy", "set-mppreference", "add-mppreference", "remove-mppreference", "unblock-file", "takeown"}},
		{Tools: []string{"icacls"}, Match: winOptionForm("grant", "deny", "setowner", "reset", "remove", "restore", "setintegritylevel")},
	}},
	{ID: "raw-device-write", Category: catSystem, Description: "writing to a raw disk device", Mode: ModeDeny},

	{ID: "service-control", Category: catSystem, Description: "starting, restarting, enabling or reconfiguring a service", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"systemctl"}, Subs: []string{"start", "restart", "reload", "try-restart", "reload-or-restart", "try-reload-or-restart", "enable", "reenable", "daemon-reload", "daemon-reexec", "edit", "set-property", "link", "preset", "preset-all", "revert", "unmask", "set-environment", "unset-environment", "import-environment"}},
		{Tools: []string{"service"}, Subs: []string{"* start", "* stop", "* restart", "* reload", "* force-reload", "* condrestart", "* try-restart"}},
		{Tools: []string{"launchctl"}, Subs: []string{"load", "unload", "bootstrap", "bootout", "kickstart", "stop", "start", "enable", "disable", "remove", "submit", "kill"}},
		{Tools: []string{"systemd-run"}},
		{Tools: []string{"sc"}, Match: firstWordIn("start", "create", "continue", "sdset", "privs", "sidtype", "triggerinfo")},
		{Tools: []string{"net"}, Match: func(call shellCall, env matchEnv) (string, bool) {
			if _, ok := firstWordIn("start", "continue")(call, env); !ok {
				return "", false
			}
			return matched(len(positionals(call)) >= 2)
		}},
		{Tools: []string{"start-service", "restart-service", "new-service"}},
		{Tools: []string{"loginctl"}, Subs: []string{"terminate-session", "terminate-user", "terminate-seat", "kill-session", "kill-user", "enable-linger", "disable-linger", "lock-sessions", "poweroff", "reboot"}},
	}},
	{ID: "kernel", Category: catSystem, Description: "loading kernel modules or changing kernel settings", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"modprobe", "insmod", "rmmod", "kexec", "depmod"}},
		{Tools: []string{"sysctl"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			if hasShortFlag(call.Args, "wp") || hasLongFlag(call.Args, "--write", "--load", "--system") {
				return "", true
			}
			for _, arg := range call.Args {
				if !strings.HasPrefix(arg, "-") && strings.Contains(arg, "=") {
					return "Matched: sysctl setting " + arg + ".", true
				}
			}
			return "", false
		}},
		{Tools: []string{"dmesg"}, Flags: []string{"-C", "-c", "--clear", "--read-clear"}},
	}},
	{ID: "user-admin", Category: catSystem, Description: "managing users, groups or passwords", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"net"}, Match: func(call shellCall, env matchEnv) (string, bool) {
			if _, ok := firstWordIn("user", "localgroup", "group", "accounts")(call, env); !ok {
				return "", false
			}
			return matched(len(positionals(call)) >= 3)
		}},
		{Tools: []string{"new-localuser", "remove-localuser", "set-localuser", "add-localgroupmember", "remove-localgroupmember", "enable-localuser"}},
		{Tools: []string{"useradd", "userdel", "usermod", "adduser", "deluser", "groupadd", "groupdel", "groupmod", "passwd", "chpasswd", "chsh", "chfn", "chage", "visudo", "gpasswd", "newusers", "vipw", "vigr"}},
	}},
	{ID: "network-config", Category: catSystem, Description: "changing network, VPN, hostname or clock settings", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"nmcli"}, Match: nmcliWrite},
		{Tools: []string{"ip"}, Match: ipWrite},
		{Tools: []string{"ifconfig"}, Match: func(call shellCall, _ matchEnv) (string, bool) { return matched(len(positionals(call)) >= 2) }},
		{Tools: []string{"route"}, Subs: []string{"add", "del", "delete", "change", "flush"}},
		{Tools: []string{"tailscale"}, Subs: []string{"up", "down", "logout", "login", "set", "serve", "funnel", "switch", "cert"}},
		{Tools: []string{"wg"}, Subs: []string{"set", "setconf", "addconf", "syncconf"}},
		{Tools: []string{"wg-quick", "openvpn", "nmtui"}},
		{Tools: []string{"resolvectl"}, Subs: []string{"dns * *", "domain * *", "default-route * *", "llmnr * *", "mdns * *", "dnssec * *", "dnsovertls * *", "nta * *", "revert", "flush-caches", "reset-statistics", "reset-server-features"}},
		{Tools: []string{"hostnamectl"}, Subs: []string{"set-hostname", "set-icon-name", "set-chassis", "set-deployment", "set-location", "hostname *", "icon-name *", "chassis *", "deployment *", "location *"}},
		{Tools: []string{"timedatectl"}, Subs: []string{"set-time", "set-timezone", "set-local-rtc", "set-ntp"}},
		{Tools: []string{"netsh"}, Match: netshWrite()},
		{Tools: []string{"set-netipaddress", "new-netipaddress", "remove-netipaddress", "set-dnsclientserveraddress", "new-netroute", "remove-netroute", "disable-netadapter", "enable-netadapter", "rename-computer", "set-date", "set-timezone"}},
	}},
	{ID: "log-wipe", Category: catSystem, Description: "deleting or rotating the system journal", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"journalctl"}, Flags: []string{"--vacuum-size", "--vacuum-time", "--vacuum-files", "--rotate", "--flush", "--relinquish-var"}},
		{Tools: []string{"wevtutil"}, Match: firstWordIn("cl", "clear-log", "sl", "set-log", "um", "uninstall-manifest")},
		{Tools: []string{"clear-eventlog", "remove-eventlog", "limit-eventlog"}},
	}},
	{ID: "desktop-settings", Category: catSystem, Description: "changing desktop settings or default applications", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"gsettings"}, Subs: []string{"set", "reset", "reset-recursively"}},
		{Tools: []string{"dconf"}, Subs: []string{"write", "reset", "load", "update"}},
		{Tools: []string{"xdg-settings"}, Subs: []string{"set"}},
		{Tools: []string{"xdg-mime"}, Subs: []string{"default", "install", "uninstall"}},
		{Tools: []string{"update-alternatives", "alternatives"}, Flags: []string{"--set", "--install", "--remove", "--remove-all", "--config", "--auto", "--all"}},
	}},
	{ID: "persistence-write", Category: catSystem, Description: "writing a shell startup file, autostart entry, git hook or tool credentials file", Mode: ModeAsk, WritePaths: []string{
		"~/.bashrc", "~/.bash_profile", "~/.bash_login", "~/.bash_logout", "~/.profile", "~/.zshrc", "~/.zprofile", "~/.zshenv", "~/.zlogin",
		"~/.config/fish/config.fish", "~/.config/fish/conf.d/", "~/.config/autostart/", "~/.config/systemd/user/", "~/.config/environment.d/",
		"~/.pam_environment", "~/.xprofile", "~/.xinitrc", "~/.xsession", "~/.gitconfig", "~/.config/git/config", "~/.npmrc", "~/.yarnrc", "~/.yarnrc.yml",
		"~/.pypirc", "~/.pip/pip.conf", "~/.config/pip/pip.conf", "~/.cargo/credentials*", "~/.cargo/config*", "~/.docker/config.json",
		"~/.local/share/applications/", ".git/hooks/", ".git/config", ".husky/",
	}},
}

var nmcliObjects = []string{"connection", "device", "radio", "networking", "general"}

var nmcliWriteVerbs = map[string]bool{"up": true, "down": true, "delete": true, "del": true, "modify": true, "mod": true, "add": true, "edit": true, "reload": true, "import": true, "connect": true, "disconnect": true, "set": true, "reapply": true, "on": true, "off": true, "clone": true, "hostname": true}

// nmcliWrite matches nmcli writes; nmcli accepts any prefix of an object name, such as c or con for connection.
func nmcliWrite(call shellCall, _ matchEnv) (string, bool) {
	words := positionals(call)
	if len(words) < 2 {
		return "", false
	}
	known := false
	for _, object := range nmcliObjects {
		if strings.HasPrefix(object, words[0]) {
			known = true
		}
	}
	if !known {
		return "", false
	}
	for _, word := range words[1:] {
		if nmcliWriteVerbs[word] {
			return "Matched: nmcli " + words[0] + " " + word + ".", true
		}
	}
	return "", false
}

var ipObjects = []string{"link", "address", "addr", "route", "rule", "neighbour", "neigh", "tunnel", "tuntap", "netns", "maddress", "mroute", "nexthop", "vrf", "xfrm"}

var ipWriteVerbs = map[string]bool{"add": true, "del": true, "delete": true, "set": true, "flush": true, "change": true, "replace": true, "append": true, "prepend": true, "attach": true, "detach": true}

func ipWrite(call shellCall, _ matchEnv) (string, bool) {
	words := positionals(call)
	if len(words) < 2 || !ipWriteVerbs[words[1]] {
		return "", false
	}
	for _, object := range ipObjects {
		if strings.HasPrefix(object, words[0]) {
			return "Matched: ip " + words[0] + " " + words[1] + ".", true
		}
	}
	return "", false
}

var netshWriteVerbs = map[string]bool{"add": true, "set": true, "delete": true, "del": true, "reset": true, "import": true, "install": true, "uninstall": true, "flush": true, "connect": true, "disconnect": true, "exec": true}

// netshWrite matches a netsh command with a write verb, inside one of contexts when any are given and outside the firewall contexts otherwise, so firewall writes report under firewall.
func netshWrite(contexts ...string) func(shellCall, matchEnv) (string, bool) {
	return func(call shellCall, _ matchEnv) (string, bool) {
		words := positionals(call)
		if len(words) == 0 {
			return "", false
		}
		context := strings.ToLower(words[0])
		firewall := context == "advfirewall" || context == "firewall"
		if len(contexts) > 0 != firewall {
			return "", false
		}
		for _, word := range words[1:] {
			if netshWriteVerbs[strings.ToLower(word)] {
				return "Matched: netsh " + words[0] + " " + word + ".", true
			}
		}
		return "", false
	}
}

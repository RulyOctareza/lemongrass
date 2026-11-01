// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

var networkRules = []rule{
	{ID: "curl-upload", Category: catNetwork, Description: "curl sending a local file", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"curl"}, Match: func(call shellCall, _ matchEnv) (string, bool) { return matched(curlSendsFile(call.Args)) }},
	}},
	{ID: "wget-upload", Category: catNetwork, Description: "wget sending a local file", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"wget"}, Flags: []string{"--post-file", "--body-file"}},
	}},
	{ID: "raw-network", Category: catNetwork, Description: "opening a raw network connection", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"nc", "ncat", "netcat", "socat", "telnet"}},
	}},
	{ID: "remote-shell", Category: catNetwork, Description: "a remote shell or file copy", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"ssh", "scp", "sftp", "sshfs", "mosh"}},
		{Tools: []string{"rdesktop", "xfreerdp", "wlfreerdp", "vncviewer", "remmina", "autossh", "sshpass"}},
		{Tools: []string{"gcloud"}, Subs: []string{"compute ssh", "compute scp", "cloud-shell ssh", "cloud-shell scp"}},
		{Tools: []string{"az"}, Subs: []string{"ssh"}},
		{Tools: []string{"aws"}, Subs: []string{"ssm start-session", "ec2-instance-connect ssh"}},
	}},
	{ID: "rsync-remote", Category: catNetwork, Description: "rsync to or from a remote host", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"rsync"}, Flags: []string{"-e", "--rsh"}},
		{Tools: []string{"rsync"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			for _, arg := range call.Args {
				if remoteRsyncArg.MatchString(arg) {
					return "Matched: rsync with the remote path " + arg + ".", true
				}
			}
			return "", false
		}},
	}},
	{ID: "download-exec", Category: catNetwork, Description: "running downloaded code", Mode: ModeDeny},

	{ID: "tunnel", Category: catNetwork, Description: "exposing this machine through a tunnel", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"ngrok", "lt", "localtunnel", "bore", "frpc", "chisel", "zrok", "pinggy", "localxpose", "loclx"}},
		{Tools: []string{"cloudflared"}, Subs: []string{"tunnel", "access"}},
	}},
	{ID: "file-share", Category: catNetwork, Description: "sending files through a sharing service", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"croc", "wormhole", "magic-wormhole", "ffsend", "transfer", "onionshare-cli", "pastebinit"}},
	}},
	{ID: "send-mail", Category: catNetwork, Description: "sending email", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"mail", "mailx", "sendmail", "mutt", "neomutt", "swaks", "msmtp", "ssmtp", "s-nail"}},
	}},
	{ID: "network-scan", Category: catNetwork, Description: "scanning or attacking network hosts", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"nmap", "masscan", "zmap", "rustscan", "nikto", "sqlmap", "hydra", "medusa", "gobuster", "ffuf", "wfuzz", "nuclei", "dirb", "dirbuster", "feroxbuster", "wpscan", "arp-scan", "hping3", "responder", "bettercap", "ettercap", "aircrack-ng", "metasploit", "msfconsole", "msfvenom"}},
	}},
}

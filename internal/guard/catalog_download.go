// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// downloadRules default to ask: a download brings unreviewed third-party code or data onto the machine. Fetches a build tool makes as a side effect are not seen.
var downloadRules = []rule{
	{ID: "container-pull", Category: catDownload, Description: "pulling or building container images", Mode: ModeAsk, Forms: []form{
		{Tools: containerEngines, Subs: []string{"pull", "image pull", "build", "image build", "buildx build", "builder build", "buildx bake", "compose pull", "compose up", "compose build", "compose run", "compose create"}},
		// run and create pull a missing image, and the guard cannot know whether it is present.
		{Tools: containerEngines, Subs: []string{"run", "create", "container run", "container create"}, Unless: []string{"--pull=never", "--pull never"}},
		{Tools: composeTools, Subs: []string{"pull", "up", "build", "run", "create"}},
		{Tools: []string{"buildah"}, Subs: []string{"pull", "from", "bud", "build"}},
		{Tools: []string{"skopeo"}, Subs: []string{"copy", "sync"}},
		{Tools: []string{"crane"}, Subs: []string{"pull", "export"}},
		{Tools: []string{"ctr"}, Subs: []string{"image pull", "images pull", "i pull", "content fetch"}},
		{Tools: []string{"crictl"}, Subs: []string{"pull"}},
		{Tools: []string{"oras"}, Subs: []string{"pull"}},
		{Tools: []string{"helm"}, Subs: []string{"pull", "fetch", "install", "i", "upgrade", "dependency update", "dependency build", "dep update", "dep build", "repo add", "repo update"}},
	}},
	// package-restore allows, by default, fetching what a committed manifest or lockfile already declares: a human chose those dependencies when committing them.
	{ID: "package-restore", Category: catDownload, Description: "restoring the dependencies a project already declares", Mode: ModeAllow, Forms: []form{
		{Match: func(call shellCall, env matchEnv) (string, bool) {
			if !restoreCall(call, env) {
				return "", false
			}
			return "Matched: " + call.Name + " restoring declared dependencies.", true
		}},
	}},
	{ID: "package-install", Category: catDownload, Description: "installing or fetching language packages", Mode: ModeAsk, Except: restoreCall, Forms: []form{
		{Tools: []string{"npm"}, Subs: []string{"install", "i", "in", "ins", "inst", "insta", "instal", "isnt", "isnta", "isntal", "isntall", "add", "ci", "clean-install", "install-clean", "isntall-clean", "update", "up", "upgrade", "udpate", "exec", "x", "install-test", "it", "install-ci-test", "cit", "init", "create"}},
		{Tools: []string{"npx", "pnpx", "bunx", "uvx", "pipx"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			if call.Name == "pipx" {
				return matched(subMatch([]string{"install", "run", "inject", "upgrade", "upgrade-all", "reinstall", "reinstall-all"}, positionals(call)) != "")
			}
			return "", true
		}},
		{Tools: []string{"pnpm"}, Subs: []string{"install", "i", "add", "update", "up", "upgrade", "dlx", "exec", "create", "fetch", "install-test", "it"}},
		{Tools: []string{"yarn"}, Subs: []string{"install", "add", "up", "upgrade", "upgrade-interactive", "dlx", "create", "global add", "import"}},
		{Tools: []string{"yarn"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			if len(positionals(call)) > 0 || containsArg(call.Args, "--version", "-v", "--help", "-h") {
				return "", false
			}
			return "Matched: yarn with no command, which installs.", true
		}},
		{Tools: []string{"bun"}, Subs: []string{"install", "i", "add", "a", "update", "x", "create", "c"}},
		{Tools: []string{"deno"}, Subs: []string{"install", "add", "cache", "outdated", "upgrade"}},
		{Tools: []string{"corepack"}, Subs: []string{"enable", "prepare", "install", "up", "use"}},
		{Tools: []string{"pip", "pip3"}, Subs: []string{"install", "download", "wheel"}},
		{ToolPrefixes: []string{"python", "pypy"}, Match: pythonModuleInstall},
		{Tools: []string{"uv"}, Subs: []string{"add", "sync", "lock", "pip install", "pip sync", "pip compile", "tool install", "tool run", "tool upgrade", "python install"}},
		{Tools: []string{"poetry"}, Subs: []string{"install", "add", "update", "lock", "sync", "self update", "self add", "self install"}},
		{Tools: []string{"pdm"}, Subs: []string{"install", "add", "update", "sync", "lock", "self update"}},
		{Tools: []string{"pipenv"}, Subs: []string{"install", "sync", "update", "lock", "upgrade"}},
		{Tools: []string{"hatch"}, Subs: []string{"env create", "dep sync", "python install"}},
		{Tools: []string{"conda", "mamba", "micromamba"}, Subs: []string{"install", "create", "update", "upgrade", "env create", "env update"}},
		{Tools: []string{"pixi"}, Subs: []string{"install", "add", "update", "global install", "exec"}},
		{Tools: []string{"cargo"}, Subs: []string{"install", "add", "fetch", "update", "binstall", "generate-lockfile", "vendor"}},
		{Tools: []string{"cargo-binstall"}},
		{Tools: []string{"go"}, Subs: []string{"get", "install", "mod download", "mod tidy", "mod vendor", "run"}, Match: goFetches},
		{Tools: []string{"gem"}, Subs: []string{"install", "update", "fetch", "i"}},
		{Tools: []string{"bundle", "bundler"}, Subs: []string{"install", "update", "add", "lock", "cache", "outdated"}},
		{Tools: []string{"composer"}, Subs: []string{"install", "require", "update", "create-project", "global require", "upgrade", "u", "i", "reinstall"}},
		{Tools: []string{"dotnet"}, Subs: []string{"add package", "add * package", "restore", "tool install", "tool update", "tool restore", "new install", "workload install", "workload update", "workload restore"}},
		{Tools: []string{"nuget"}, Subs: []string{"install", "restore", "update"}},
		{Tools: []string{"cpan", "cpanm", "cpm", "carton"}},
		{Tools: []string{"luarocks"}, Subs: []string{"install", "download", "build"}},
		{Tools: []string{"opam"}, Subs: []string{"install", "update", "upgrade", "init", "switch create", "pin", "repository add"}},
		{Tools: []string{"cabal"}, Subs: []string{"install", "update", "get", "fetch"}},
		{Tools: []string{"stack"}, Subs: []string{"install", "setup", "update", "upgrade", "unpack"}},
		{Tools: []string{"ghcup"}, Subs: []string{"install", "upgrade", "set"}},
		{Tools: []string{"mix"}, Subs: []string{"deps.get", "deps.update", "archive.install", "escript.install", "local.hex", "local.rebar", "hex.install"}},
		{Tools: []string{"rebar3"}, Subs: []string{"get-deps", "upgrade", "update"}},
		{Tools: []string{"dart", "flutter"}, Subs: []string{"pub get", "pub upgrade", "pub add", "pub downgrade", "pub global activate", "pub cache add", "pub cache repair", "precache", "upgrade", "create"}},
		{Tools: []string{"swift"}, Subs: []string{"package resolve", "package update", "package add-dependency"}},
		{Tools: []string{"pod"}, Subs: []string{"install", "update", "repo add", "repo update", "setup"}},
		{Tools: []string{"carthage"}, Subs: []string{"update", "bootstrap", "checkout"}},
		{Tools: []string{"vcpkg"}, Subs: []string{"install", "update", "upgrade", "add"}},
		{Tools: []string{"conan"}, Subs: []string{"install", "download", "create"}},
		{Tools: []string{"mvn", "mvnw"}, Args: []string{"dependency:resolve", "dependency:get", "dependency:go-offline", "dependency:copy-dependencies", "archetype:generate"}},
		{Tools: []string{"gradle", "gradlew"}, Args: []string{"dependencies", "--refresh-dependencies", "wrapper"}},
		{Tools: []string{"sbt"}, Args: []string{"update", "updateClassifiers"}},
		{Tools: []string{"lein"}, Subs: []string{"deps"}},
		{Tools: []string{"clojure", "clj"}, Flags: []string{"-P"}},
		{Tools: []string{"raco"}, Subs: []string{"pkg install", "pkg update"}},
		{Tools: []string{"nimble"}, Subs: []string{"install", "refresh"}},
		{Tools: []string{"zig"}, Subs: []string{"fetch"}},
		{Tools: []string{"julia"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			for _, arg := range call.Args {
				if strings.Contains(arg, "Pkg.add") || strings.Contains(arg, "Pkg.update") || strings.Contains(arg, "Pkg.instantiate") {
					return "Matched: julia running Pkg to fetch packages.", true
				}
			}
			return "", false
		}},
		{Tools: []string{"Rscript", "R"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			for _, arg := range call.Args {
				if strings.Contains(arg, "install.packages") || strings.Contains(arg, "BiocManager::install") || strings.Contains(arg, "remotes::install_") || strings.Contains(arg, "devtools::install_") {
					return "Matched: R installing packages.", true
				}
			}
			return "", false
		}},
	}},
	{ID: "system-package", Category: catDownload, Description: "installing or upgrading system packages", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"apt", "apt-get", "aptitude"}, Subs: []string{"install", "upgrade", "full-upgrade", "dist-upgrade", "download", "source", "update", "reinstall", "build-dep"}},
		{Tools: []string{"dnf", "yum", "microdnf", "dnf5"}, Subs: []string{"install", "in", "upgrade", "up", "update", "download", "group install", "groupinstall", "reinstall", "distro-sync", "makecache", "builddep", "copr enable", "system-upgrade download", "swap", "downgrade"}},
		{Tools: []string{"zypper"}, Subs: []string{"install", "in", "update", "up", "dup", "dist-upgrade", "source-install", "si", "refresh", "ref", "patch", "addrepo", "ar"}},
		{Tools: []string{"pacman", "yay", "paru"}, Match: pacmanSync},
		{Tools: []string{"apk"}, Subs: []string{"add", "upgrade", "update", "fetch", "fix"}},
		{Tools: []string{"rpm-ostree"}, Subs: []string{"install", "upgrade", "rebase", "deploy", "override replace", "apply-live"}},
		{Tools: []string{"rpm"}, Flags: []string{"-i", "-U", "-F", "--install", "--upgrade", "--freshen", "--reinstall"}},
		{Tools: []string{"dpkg"}, Flags: []string{"-i", "--install", "--unpack"}},
		{Tools: []string{"flatpak"}, Subs: []string{"install", "update", "upgrade", "remote-add"}},
		{Tools: []string{"snap"}, Subs: []string{"install", "refresh", "download"}},
		{Tools: []string{"brew"}, Subs: []string{"install", "upgrade", "fetch", "tap", "reinstall", "update", "bundle", "bundle install"}},
		{Tools: []string{"nix-env"}, Flags: []string{"-i", "-u", "--install", "--upgrade"}},
		{Tools: []string{"nix"}, Subs: []string{"profile install", "profile add", "profile upgrade", "flake update", "flake lock", "copy"}},
		{Tools: []string{"nix"}, Subs: []string{"shell", "run", "build", "develop"}, Match: nixFlakeRef},
		{Tools: []string{"nix-shell"}, Flags: []string{"-p", "--packages"}},
		{Tools: []string{"nix-channel"}, Flags: []string{"--update", "--add"}},
		{Tools: []string{"port"}, Subs: []string{"install", "upgrade", "selfupdate", "sync"}},
		{Tools: []string{"choco", "winget", "scoop"}, Subs: []string{"install", "upgrade", "update"}},
		{Tools: []string{"emerge"}},
		{Tools: []string{"xbps-install", "pkg_add", "eopkg", "slackpkg", "swupd"}},
		{Tools: []string{"pkg"}, Subs: []string{"install", "upgrade", "update", "fetch"}},
		{Tools: []string{"toolbox", "distrobox"}, Subs: []string{"create", "upgrade"}},
		{Tools: []string{"fwupdmgr"}, Subs: []string{"update", "refresh", "install", "upgrade"}},
	}},
	{ID: "toolchain-install", Category: catDownload, Description: "installing a language toolchain, runtime or browser", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"rustup"}, Subs: []string{"install", "update", "toolchain install", "toolchain add", "component add", "target add", "default", "self update", "override set"}},
		{Tools: []string{"nvm", "fnm", "volta", "pyenv", "rbenv", "nodenv", "goenv", "jenv", "tfenv", "tfswitch", "proto", "n"}, Subs: []string{"install", "i", "use"}},
		{Tools: []string{"asdf"}, Subs: []string{"install", "plugin add", "plugin-add", "plugin update", "plugin-update"}},
		{Tools: []string{"mise", "rtx"}, Subs: []string{"install", "i", "use", "u", "upgrade", "up", "plugins install", "self-update"}},
		{Tools: []string{"sdk"}, Subs: []string{"install", "i", "upgrade", "ug", "selfupdate"}},
		{Tools: []string{"playwright"}, Subs: []string{"install", "install-deps"}},
		{Tools: []string{"puppeteer"}, Subs: []string{"browsers install"}},
		{Tools: []string{"gvm", "g"}, Subs: []string{"install"}},
		{Tools: []string{"juliaup"}, Subs: []string{"add", "update", "self update"}},
		{Tools: []string{"bun"}, Subs: []string{"upgrade"}},
		{Tools: []string{"deno"}, Subs: []string{"upgrade"}},
		{Tools: []string{"flutter"}, Subs: []string{"upgrade", "precache", "channel"}},
		{Tools: []string{"sdkmanager", "avdmanager"}},
		{Tools: []string{"uv"}, Subs: []string{"self update", "python install"}},
	}},
	{ID: "file-download", Category: catDownload, Description: "downloading files from the network", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"curl", "wget", "wget2", "aria2c", "axel", "http", "https", "xh", "xhs", "httpie", "lftp", "ftp", "tftp", "yt-dlp", "youtube-dl", "gallery-dl", "gdown", "megadl", "transmission-cli", "fetch", "lwp-download", "lwp-request", "w3m", "lynx", "links", "elinks", "grpcurl", "websocat", "wscat", "hurl"}},
		{Tools: []string{"iwr", "irm", "invoke-webrequest", "invoke-restmethod", "start-bitstransfer", "bitsadmin"}},
		{Tools: []string{"certutil"}, Match: winOptionForm("urlcache", "verifyctl")},
		{Tools: []string{"gh"}, Subs: []string{"release download", "run download", "attestation download", "extension install"}},
		{Tools: []string{"glab"}, Subs: []string{"release download", "job artifact", "ci artifact"}},
		{Tools: []string{"svn"}, Subs: []string{"cat"}},
		{Tools: []string{"git"}, Subs: []string{"archive"}, Flags: []string{"--remote"}},
	}},
	{ID: "cloud-download", Category: catDownload, Description: "copying files to or from cloud storage", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"aws"}, Subs: []string{"s3 cp", "s3 sync", "s3 mv", "s3api get-object"}},
		{Tools: []string{"gsutil"}, Subs: []string{"cp", "rsync", "mv", "cat"}},
		{Tools: []string{"gcloud"}, Subs: []string{"storage cp", "storage rsync", "storage mv", "storage cat"}},
		{Tools: []string{"az"}, Subs: []string{"storage blob download", "storage blob download-batch", "storage blob upload", "storage blob upload-batch", "storage blob sync", "storage file download", "storage file download-batch", "storage copy"}},
		{Tools: []string{"azcopy"}, Subs: []string{"copy", "cp", "sync"}},
		{Tools: []string{"rclone"}, Subs: []string{"copy", "sync", "copyto", "move", "moveto", "bisync", "copyurl", "cat"}},
		{Tools: []string{"s3cmd"}, Subs: []string{"get", "sync", "put", "cp", "mv"}},
		{Tools: []string{"mc"}, Subs: []string{"cp", "mirror", "mv", "get", "put", "cat"}},
		{Tools: []string{"b2"}, Subs: []string{"download-file", "download-file-by-name", "sync", "file download", "upload-file"}},
	}},
	{ID: "model-download", Category: catDownload, Description: "downloading AI models or datasets", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"ollama"}, Subs: []string{"pull", "run"}},
		{Tools: []string{"huggingface-cli", "hf"}, Subs: []string{"download"}},
		{Tools: []string{"kaggle"}, Subs: []string{"datasets download", "competitions download", "kernels pull", "models instances versions download"}},
		{Tools: []string{"llm"}, Subs: []string{"install", "models download"}},
		{Tools: []string{"lms"}, Subs: []string{"get", "load"}},
	}},
	{ID: "infra-init", Category: catDownload, Description: "downloading infrastructure providers, modules or boxes", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"terraform", "tofu", "terragrunt"}, Subs: []string{"init", "get", "providers mirror", "providers lock"}},
		{Tools: []string{"packer"}, Subs: []string{"init", "plugins install"}},
		{Tools: []string{"pulumi"}, Subs: []string{"plugin install", "install", "new"}},
		{Tools: []string{"vagrant"}, Subs: []string{"box add", "box update", "up", "plugin install"}},
		{Tools: []string{"ansible-galaxy"}, Subs: []string{"install", "collection install", "role install"}},
	}},
	{ID: "git-clone", Category: catDownload, Description: "cloning a repository", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"clone", "submodule update", "submodule add", "submodule init", "lfs pull", "lfs fetch", "lfs clone"}},
		{Tools: []string{"gh"}, Subs: []string{"repo clone", "gist clone", "pr checkout"}},
		{Tools: []string{"gh"}, Subs: []string{"repo fork"}, Flags: []string{"--clone"}},
		{Tools: []string{"glab"}, Subs: []string{"repo clone", "repo fork", "mr checkout"}},
		{Tools: []string{"hub"}, Subs: []string{"clone"}},
		{Tools: []string{"svn"}, Subs: []string{"checkout", "co", "export"}},
		{Tools: []string{"hg", "fossil", "bzr", "darcs"}, Subs: []string{"clone", "get", "branch"}},
		{Tools: []string{"repo"}, Subs: []string{"init", "sync"}},
		{Tools: []string{"degit", "tiged", "ghq"}},
	}},
	{ID: "git-fetch", Category: catDownload, Description: "fetching or pulling from a remote repository", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"fetch", "pull", "remote update", "ls-remote"}},
		{Tools: []string{"gh"}, Subs: []string{"repo sync"}},
		{Tools: []string{"svn"}, Subs: []string{"update", "up", "switch"}},
		{Tools: []string{"hg"}, Subs: []string{"pull", "incoming", "in"}},
		{Tools: []string{"fossil"}, Subs: []string{"pull", "update", "sync"}},
	}},
}

// pythonModuleInstall matches python -m pip, ensurepip or venv installing packages.
func pythonModuleInstall(call shellCall, _ matchEnv) (string, bool) {
	for i, arg := range call.Args {
		if arg != "-m" || i+1 >= len(call.Args) {
			continue
		}
		module := call.Args[i+1]
		rest := call.Args[i+2:]
		switch module {
		case "pip", "pip3", "uv", "pipx", "poetry":
			for _, word := range rest {
				switch word {
				case "install", "download", "wheel", "add", "sync", "run":
					return "Matched: " + call.Name + " -m " + module + " " + word + ".", true
				}
			}
		case "ensurepip":
			return "Matched: " + call.Name + " -m ensurepip.", true
		case "spacy", "nltk":
			for _, word := range rest {
				if word == "download" {
					return "Matched: " + call.Name + " -m " + module + " download.", true
				}
			}
		case "playwright":
			if containsArg(rest, "install") {
				return "Matched: " + call.Name + " -m playwright install.", true
			}
		}
		return "", false
	}
	return "", false
}

// goFetches passes go run and go install of a local package; with a module path or @version they fetch.
func goFetches(call shellCall, _ matchEnv) (string, bool) {
	words := positionals(call)
	if len(words) == 0 {
		return "", false
	}
	switch words[0] {
	case "run", "install":
		for _, word := range words[1:] {
			if strings.Contains(word, "@") {
				return "Matched: go " + words[0] + " " + word + ", which fetches the module.", true
			}
		}
		if words[0] == "install" && len(words) == 1 {
			return "", false
		}
		if words[0] == "run" {
			return "", false
		}
		for _, word := range words[1:] {
			if !strings.HasPrefix(word, ".") && !strings.HasPrefix(word, "/") && strings.Contains(strings.SplitN(word, "/", 2)[0], ".") {
				return "Matched: go install " + word + ", which fetches the module.", true
			}
		}
		return "", false
	}
	return "", true
}

// pacmanSync matches a pacman sync or local install, passing the search and info forms such as -Ss and -Si.
func pacmanSync(call shellCall, _ matchEnv) (string, bool) {
	for _, arg := range call.Args {
		if arg == "--sync" || arg == "--upgrade" {
			return "", true
		}
		if strings.HasPrefix(arg, "--") || !strings.HasPrefix(arg, "-") {
			continue
		}
		letters := arg[1:]
		if strings.HasPrefix(letters, "U") {
			return "", true
		}
		if !strings.HasPrefix(letters, "S") {
			continue
		}
		if strings.ContainsAny(letters, "yuw") || !strings.ContainsAny(letters, "silgcp") {
			return "", true
		}
	}
	return "", false
}

// nixFlakeRef matches nix shell, run, build and develop naming a flake or nixpkgs attribute, which fetch.
func nixFlakeRef(call shellCall, _ matchEnv) (string, bool) {
	for _, word := range positionals(call)[1:] {
		if strings.Contains(word, "#") || strings.HasPrefix(word, "nixpkgs") || strings.Contains(word, ":") {
			return "Matched: nix " + positionals(call)[0] + " " + word + ", which fetches it.", true
		}
	}
	return "", false
}

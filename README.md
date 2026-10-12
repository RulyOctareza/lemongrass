# Lemongrass

Lemongrass runs coding agents in real terminals and gives them what a raw terminal does not: credentials they can use but never see, teammates they can message, a safety guard that holds with permission prompts off, and project rules that arrive at the start of every session.

It is an Electron app plus `lgrass`, a Go CLI that the agents call themselves. Claude Code and Codex are supported, and a pane can also open Cursor Agent or a plain shell.

## What it does

### Agents use your databases and APIs without holding the credential

`lgrass db` and `lgrass rester` send queries and HTTP calls through a vault daemon. The vault is the only process that decrypts anything. The agent holds a six-character channel id that means nothing without the vault's own mapping.

- **Databases (MySQL, MariaDB, Postgres).** The vault parses each statement and checks the tables it actually touches against the channel's scope, so a query that joins a table outside the scope is refused. DDL and multi-statement calls are refused. A write runs as a dry run first and commits in a separate call, and an `UPDATE` or `DELETE` without a `WHERE` clause is refused. Database errors are redacted before the agent sees them.
- **HTTP APIs.** A domain holds a base URL, a login endpoint and many users, each with credentials or a pasted token. The vault logs in, keeps the token in locked memory and never on disk, injects it into each request, and logs in again once when a token has gone stale. A channel grants a list of methods and excludes method and path pairs. A request path is resolved onto the domain's base URL and cannot address another host.
- **Lifetime.** Channels expire, and a revoke takes effect at the vault. Database writes are recorded in an audit table with a 7-day retention.
- **Managed from one place.** The Connector tab creates, tests and revokes connections and channels behind a single vault passphrase unlock with auto-lock.

### Agents work as a team, across vendors

A leader agent declares a workgroup of one to five thinkers, each running Claude Code or Codex, with `lgrass workgroup create`.

- **The human approves what starts.** Every thinker's complete starting text is shown before anything spawns. Approval issues a one-time token, and that token is the only path that opens the members' tabs.
- **Plan first.** A thinker posts its plan to the group thread and waits for the leader's go before it changes anything. A gate denies every tool except `lgrass` commands until the skills the thinker was configured with are loaded.
- **Real conversation.** Members share a thread with mentions, unread-only reads, and notices delivered through each agent's own hooks. Any member can address any other directly.
- **Persistent.** Membership survives a tab closing and an app restart. A sidebar panel shows each group's members and whether their tabs are online. A disbanded group's thread stays readable.

### Bypass mode with a guard that stays on

Claude Code tabs start with `--dangerously-skip-permissions`, so an agent does not stall on prompts. A `PreToolUse` hook enforces a catalog of dangerous operations instead, in the categories Git, Deletion, System, Network, Infrastructure, Database clients and Lemongrass itself. A refused call returns an approval message to the agent and is not silently dropped. A user policy edited in the Vault tab's Safety pane is sealed in the vault, and the hook reads it from there.

### Project rules arrive at the start of every session

A `SessionStart` hook delivers the project's laws summary and gates tool calls behind required skills. Sessions in the same project know about each other, and the hook warns when two tabs touch the same file.

The convention behind this is [bibliothek](https://github.com/faizalv/bibliothek): a skill plus a `biblio/{scratchpad,laws,handover,books}/` folder layout, developed in its own repository and usable without Lemongrass. Lemongrass pins it as a git submodule and puts its text into the system prompt of every Claude tab, so Claude tabs never load it as a skill. The app shows that folder as a tree and reader beside the terminal panes. The scratchpad is editable in a WYSIWYG editor that autosaves, enforced on the server side. The other folders are read-only.

### Real terminals, controlled by structured events

Panes are `xterm.js` in the renderer over `node-pty` in the main process, the stack behind VS Code's integrated terminal, not a chat UI. Control decisions such as nudges come from each agent's own hook events and never from parsing rendered terminal text. The app also provides tabs, split layouts that persist across restarts, session restore, and a Git panel with diff, commit and push.

## Status and limits

Built:

- The vault, `lgrass db` reads and writes, `lgrass rester`, and the Connector tab.
- Workgroups with Claude Code members, the thread layer, and the workgroup panel.
- The guard for Claude Code, unit tested and denying live in a tab. The sealed-policy round trip through the Safety pane has not been run live.
- The bibliothek browser and editor, and the Git panel.

In progress:

- A wire-protocol proxy so a whole app can point its own database driver at a local port. Port allocation and the MySQL handshake listener are done. Query execution and the Postgres listener are not.
- A Decisions section in the Vault tab that replaces the native dialog for workgroup approvals.
- Codex as a workgroup member. Its gate depends on a listener heartbeat that is unverified under Codex's default sandbox.

Planned:

- An audit of `lgrass rester` calls and of `lgrass db` reads.
- An investigator role for cheap, read-only workgroup members.

Limits:

- Linux is the primary platform. The macOS build exists and Windows packaging is not wired.
- An HTTP channel's scope is declared and checked against the request. The database scope is checked against what the parsed statement touches, which is a stronger guarantee.
- The guard does not inspect an interpreter one-liner such as `python -c` or `node -e`, and a path built from an unknown variable can pass.

## How it fits together

- `ui/` is the Electron app: terminal panes, project and layout management, the bibliothek tree and reader, and the Connector tab.
- `cmd/lgrass` is the Go CLI models and users call: session and thread coordination, `lgrass db`, `lgrass rester`, and `lgrass workgroup`.
- `cmd/lgrassd` is the binary only the app and the agents' hooks call, installed off `PATH`: the hooks and their gates, the vault and agent daemons, tab records and account management.
- `cmd/lgrassconf` is a small Go daemon, a systemd user service on Linux and a LaunchAgent on macOS. It registers the Claude Code and Codex hooks, installs each agent's skills and keeps every Claude account's hooks and shared folders in place.
- `internal/` holds the Go packages the three binaries share, such as the vault, the guard, the hook engine, the session store and the account manager. It is importable only from inside this repository.

The Go code is one module at the repo root, and `ui/` is the second toolchain beside it. Neither side's scripts reach across that boundary except through the `dist/` binaries. The binaries are bundled inside the Electron app at build time, and `lgrass` installs itself onto `PATH` when the app launches, so the two stay on the same version without a separate release pipeline.

## Skills

`lgrassconf` installs these for Claude Code and Codex and keeps them current. Bibliothek is not among them: Claude tabs get it in their system prompt, and a Codex tab or a Claude session outside Lemongrass uses it only if it is installed separately from its own repository.

| Skill | What it teaches an agent |
|---|---|
| `lgrass-howtobe-leader` | Lead a workgroup: declare it, brief each thinker, ask each for a plan, and stay responsible for the result. |
| `lgrass-howtobe-thinker` | Work inside a workgroup: post a plan, wait for the leader's go, and talk with the leader and the other thinkers. |
| `lgrass-howtobe-keeper` | Keep `biblio/` and the code in agreement: clean the whiteboard, dangling handovers and tasks, correct books against the code, and tidy `toc.md`. |
| `lgrass-howtodo-work` | Work a task end to end: read the related books, plan first, keep the PRD marked as work happens, wait for the user only before code, then close. |
| `lgrass-howtodo-taskclosing` | Close a finished task: verify it is done and the checks pass, compare git with the stated facts, update the books and `toc.md`, archive, and sweep stale references. |
| `lgrass-howtodo-newlaws` | Offer the user a new standing law, and write it to `biblio/laws/` only after they approve explicitly. |

## Build

```
make build-linux
```

Cross-compiles `lgrass`, `lgrassd` and `lgrassconf` for `linux/amd64` and `linux/arm64` into `dist/`, then builds and packages the Electron app for Linux. This is the one entrypoint above both toolchains. `ui/`'s own `npm run build:linux` builds only the Electron half and expects `dist/` to exist already, so use the root `make` target. `make verify` compiles every Go package without writing a binary into the source tree. Every Go target first checks out the bibliothek submodule, and `make bibliothek-update` moves its pin to bibliothek's latest commit. It builds AppImage, snap, deb and rpm, so it needs `dpkg` and `snapcraft` on the host.

```
make build-rpm
```

Same cross-compile, then builds only the rpm (needs `rpmbuild`), for Fedora and other rpm distributions. The package registers the `lgrassconf` keeper as a systemd user unit on install and removes it on uninstall, not on upgrade. The app also writes a per-user copy of `lgrass` and `lgrassconf` into `~/.local/bin` and its own user unit on launch. `lgrassconf uninstall` stops and removes that unit and leaves the binaries in place.

```
make build-mac
```

Cross-compiles `lgrass` and `lgrassconf` for `darwin/amd64` and `darwin/arm64`, then packages a macOS `.app`, `.dmg` and `.zip` via electron-builder. On first launch the app copies the matching arch binaries into `~/.local/bin` and runs `lgrassconf install` (LaunchAgent). Notarization is off by default.

Windows packaging is not wired up yet.

## Develop

```
cd ui && npm run dev
```

`make dev` from the repo root installs the `lgrassconf` keeper (systemd user unit on Linux, LaunchAgent on macOS) and builds `lgrass` into `~/.local/bin` first, so Claude Code and Codex configuration are correct before a session starts. Codex hook definitions still require review and trust through `/hooks`. `lgrass` is not required for the Electron shell itself to run. The agent CLI running inside a terminal pane invokes it, not the app directly. New shell panes prompt for which CLI to spawn (Claude Code, Codex, Cursor Agent, or a plain shell). For work on `lgrass` alone:

```
cd lgrass && go build ./... && go test ./...
```

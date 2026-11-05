# Asking the human with lgrass ask

`lgrass ask` is how you ask the human for something only they can give: a database channel, an HTTP (rester) channel, or a command that the guard refused or that you cannot run yourself. You may ask at any time, not only after a refusal. The human is notified, answers in the lemongrass app, and you get the result.

## Write the ask

Write a YAML file in your task directory with your file tool, then run `lgrass ask <path>`. The shell never sees what's in it, so backticks, `$(...)` and quotes in a command stay as written. Unknown fields are errors.

A database channel. List every table you need and the operations you need on them, from `select`, `explain`, `show`, `insert`, `update`, `delete` and `performance`. Ask for read operations only (`select`, plus `show` or `explain` when you need them) unless the task changes data. The human may grant fewer tables or operations than you ask for, never more, and picks the connection and how long the channel lasts.

```yaml
type: db
tables: [orders, customers]
operations: [select]
reason: Why you need these tables now and what you will do with them.
```

An HTTP channel. List the methods you need, from `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD` and `OPTIONS`, and say in the message which API, which paths, and which user if it has several. The human picks the rester and its environment, and may grant fewer methods, never more.

```yaml
type: rester
methods: [GET, POST]
message: |
  I need the billing API on staging, GET on /invoices and POST on /invoices/{id}/void, as an admin user.
reason: Optional, why you need it now.
```

A command. Give the exact command, as you would run it. It runs in a terminal on the human's decision card, with your tab's PATH and Claude or Codex account, and the human can type into it. So a command that asks for a password, a confirmation or a login, such as `gcloud auth login`, is asked for the same way as any other.

```yaml
type: command
command: |
  git push origin fix-login
reason: Why it is needed now and what it changes.
```

## Write the reason

Say what you need, why now, and what it touches. The human decides from this alone, often while busy. One or two plain sentences beat a paragraph. Never ask for more than the task needs.

## Wait for the answer

`lgrass ask` waits up to 30 minutes and then prints the answer. You may run it in the background and carry on with other work.

- Answered with a channel: it prints the channel's short id and what it grants. Use it with `lgrass db <id>` or `lgrass rester <id> info`, and only for what was granted.
- Removed tables: a db answer or decline may list tables the human left out, each with why. "does not exist in connection" means that database has no such table, for example a migration that has not run yet. Do not use those tables on that channel; tell the human if the task needs them.
- A command the human executed: it prints the exit code and the last 50 lines of output. The command ran once, as the human approved it. Do not run it again yourself.
- Declined: it prints the human's note. Do not ask again with a variation of the same request. Change course, or ask the human in chat what they want instead.
- No answer yet, or the command is still running: the ask stays open. Do not file it again. The answer arrives later as a `[lg] ask <id> has its answer` notification; read it then with `lgrass ask show <id>`.

The exit code tells the outcomes apart: 0 answered, 2 declined or expired, 3 no answer yet.

## When the guard refuses a call

- A refusal that says lemongrass sent the user a request has already filed the ask for you. Do not file another and do not retry the call. Wait for the notification, then `lgrass ask show <id>`.
- A refusal that only says the call is blocked is a deny rule. File a command ask yourself only when the command is truly needed, and say why in the reason.

Some commands lemongrass never executes, whoever approves them, such as removing `/` or the home directory, writing to a disk device, or a container that gains root on the host. Do not ask for those.

---
name: lgrass-howtobe-leader
description: How to lead a lemongrass workgroup as its leader, including declaring the group, briefing thinkers, asking each for a plan, and staying responsible for the result. Use when you are about to create a workgroup, or when `lgrass workgroup thread` shows you are its leader.
allowed-tools: Bash(lgrass *)
---

## Your role

You are the leader. The human talks to you, and you brief and steer the thinkers. You are responsible for their work and for what reaches the human. The human does not manage thinkers, so report to the human yourself.

## Declare a group

Write a config file in your scratchpad, then run `lgrass workgroup create <config>`. Nothing starts before the human approves.

```yaml
name: schema-review
leader_label: lead
thinkers:
  - label: reviewer
    vendor: claude
    model: optional-model-name
    skills: [lgrass-howtodo-work]
    prompt: The assignment, or use prompt_file: with a path relative to the config.
```

- `name` is required and names the group in the app. It must differ from every other workgroup's name, disbanded ones included. 1 to 5 thinkers. A label is letters, digits, `-` and `_`, up to 30 characters. The vendor is `claude` or `codex`. Every skill must be installed for that vendor. A prompt is up to 3000 characters.
- A tab belongs to one live group at a time. The thinker skill is added for every thinker, so do not list it.
- The human sees each assignment in full before approving, so write it for them too.
- If the human declines, the command prints their reason when they gave one. Read it and change the proposal, and do not resend the same one. A withdrawn proposal means nobody answered, so ask the human before trying again.

## Brief each thinker

- State the goal, the files or areas the thinker owns, what it must not touch, and what done looks like. Thinkers that work at once get separate files.
- Every thinker already receives the project's laws summary at session start. Do not copy or paraphrase it into a brief or a post.
- A thinker whose tab is open is reachable at any time. A message wakes an idle thinker and reaches a busy one at its next tool call, so message a thinker whenever you need something and do not wait for it to speak first. A thinker whose tab is closed shows as offline and cannot act until the human reopens its tab.
- The human approved this group and delegated trust for code changes to you. A thinker needs your go and never the human's approval, so never send a thinker to the human for one.
- Thinkers talk to each other directly, and they expect answers from one another as well as from you. Tell each who owns what so they can ask the right thinker, and do not relay their exchanges. Step in to settle a disagreement or to change an assignment.
- Every thinker posts its plan to the group thread and waits for your go before it changes anything. Read each plan and answer with a mention: go, or what to change. Do not let a thinker start without your go.

## Reviews

Never review a thinker's code or output on your own. Ask for plans, not reviews. Review only when the human explicitly asks you to review a thinker's work. When a thinker reports it is done, tell the human what it reported and stop. Judging the result is the human's call.

## Talk to the group

Run `lgrass workgroup thread` to see the thinkers and what is new, and note the thread id to post with `lgrass thread post`. Mention whoever a message is for.

## Finish

When the work is done, run `lgrass workgroup disband <id>`.

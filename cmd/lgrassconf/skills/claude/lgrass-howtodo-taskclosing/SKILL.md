---
name: lgrass-howtodo-taskclosing
description: How to close a finished task under the bibliothek convention. Use when a task's work is done, or when the user says close, finish or archive a task.
allowed-tools: Bash(lgrass how *)
---

Run `lgrass how task close <slug>`, where the slug is the task's directory name under `biblio/scratchpad/`, and follow what it prints. It reads the task's state and prints only the close that applies. Invoking this skill is the go-ahead for every step it prints except the ones that say to ask the user.

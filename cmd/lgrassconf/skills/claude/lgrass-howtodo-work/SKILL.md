---
name: lgrass-howtodo-work
description: Use at the start of any task, before reading source, planning or editing anything. How to work a task from reading the related books to closing it, including planning first, keeping the PRD current as work happens, and when to wait for the user.
---

## 1. Find the books

Read `biblio/books/toc.md` and open every book whose name or tags relate to the task, then the chapters whose names fit. Do this before reading source. Say which books you read. Go to the code only for what no book covers, and where code and a book disagree, the code wins and the chapter is corrected at closing.

If a PRD was already given, check it against the books and the current code before acting on it. When something in it is stale, stop and tell the user what is stale before planning, asking or building.

## 2. Plan first

Show the plan in chat and write it to the task's scratchpad directory in the same turn: `biblio/scratchpad/<task-slug>/prd.md` with Problem, Expectation, Objective and Plans. Write every Plans item as `- [ ]`. A small task gets a short PRD, not none.

Write Plans as the simplest reading of the Objective. Every item must be needed to meet an Expectation. If a similar function or an earlier feature suggests adding more, do not put it in Plans. Tell the user in one or two plain sentences with your recommendation, and add it only after they say yes. Whoever resumes the PRD reads it as the spec, so an unconfirmed item becomes an order.

For a task that already has a directory, read its `prd.md` and `activities.md` first and continue from the marks, since the marks show what is done.

## 3. What waits for the user

Wait for the user's go-ahead before changing code or running anything with side effects outside `biblio/`. Also ask before changing the core idea: the Problem, the Expectation, the Objective, a settled design decision, or dropping a Plans item. Writing under `biblio/` never waits: the PRD, notes, activity log, books and toc are the model's own responsibility, and so are adding an item the Objective requires, adding detail, marking done and logging.

## 4. Do the work, keeping the PRD current

Follow the PRD as written. If building it hits a contradiction, such as an import cycle or a missing piece, stop and tell the user in one or two plain sentences with your recommendation, instead of working around it.

In the same turn that a decision is made or a Plans item is finished:

- Flip the item to `- [x]`, only once it is done and verified.
- Write the decision into the PRD in place.
- Append a dated entry to `activities.md` for a decision, shipped work or a correction.

The marks are how anyone, including you tomorrow, knows what is done.

## 5. Out-of-scope findings

A problem outside the Objective is not worked in this task. Offer the user a new scratchpad task for it, or, when a task covers the same domain, offer to add it to that task's PRD instead. A Plans item big enough to be built and tracked on its own is offered as a split: its own task, with the parent's Plans pointing at it.

## 6. Pausing or delegating

Write a handover only when the task is paused or handed to someone else, never for a small task finished in one session. Its Status holds where to resume, what is next and what is blocked, and the PRD marks hold what is done. A handover comes with a whiteboard row and a memory pointer.

## 7. Done

When the work is done, run `lgrass-howtodo-taskclosing`.

Never run `git add`, `git commit` or any history-changing git command unless the user asks for it in that moment.

---
name: lgrass-howtobe-keeper
description: How to keep a project's biblio/ and code in agreement. Use when asked to tidy, sweep or audit the whiteboard, handovers, scratchpad tasks, books or toc, or when a document, book chapter, handover, PRD, toc line, README or code comment contradicts the code or the bibliothek convention.
---

## Act without asking

Verify the current fact against its source, then fix the stale document in the same turn. Ask only when the current state cannot be determined or the fix is a design decision. Invoking this skill is the go-ahead for every fix below.

Git is read-only here: `log`, `status`, `diff` and `show` only. Never `add`, `commit` or any command that changes the index or history.

## Source of truth

- Code wins over any document that describes it, a book chapter included.
- The bibliothek skill wins over how a project lays out its `biblio/`.
- A PRD wins over a handover for design, and a handover's Status wins for progress.

## What stays as written

Dated records of what happened: activity logs and archived tasks.

## Sweep

1. **Whiteboard and handovers.** One row per active handover and none for an archived one. A handover with no row gets a row, and a row with no handover is dropped. Renumber in place.
2. **Memory pointers.** One `type: project` pointer per active handover. A pointer whose handover is archived or missing is deleted with its `MEMORY.md` line. Memory writes need `lgrass sign i-write-memory`.
3. **Scratchpad directories.** Each one outside `archive/` is either a task with a handover, or a recon, debug or notes task. A directory that is neither, or that holds finished work, is reported.
4. **Handover shape.** A handover exists only for a paused or delegated task. It points at a `prd.md` only, and its Status holds where to resume, what is next and what is blocked.
5. **Git against PRD marks.** Compare each PRD's Plans marks with the code and with `git log` for the paths involved. An item the code shows as shipped gets marked `[x]`, and a mark the code does not support is cleared. Work that looks fully finished is reported as ready for `lgrass-howtodo-taskclosing` and is not closed or archived here.
6. **Books against code.** Check each chapter's claims against the code. A stale fact is corrected in place, with no changelog and no dated narrative. A book that is wholly obsolete gets a `warning:` on its toc line naming what to read instead.
7. **toc.md.** One line per existing book, in the bibliothek form: the folder name and its bracketed tags. Drop the line of a book that no longer exists and add the line of one that is missing. A description or a `[WIP]` marker is removed. The only allowed addition is a one-sentence `warning:`.
8. **Renames.** A book folder or chapter file whose name carries a bracketed date or tags is renamed to its plain snake_case title, its tags kept on the toc line. A rename propagates to the toc and every reference.

## Report

One line per fix. Then a separate list of what needs the user: work ready to close, directories that fit nowhere, and anything the code could not settle.

A chapter records how the system works, never how it broke. A fix that restores intended behavior gets no chapter, and its root cause stays in the archived `debug.md`. Write or extend a chapter only when the task settled a new design, mechanism or decision a future reader needs. A recon that ended in "not worth pursuing" just archives.

When a chapter is written or changed:

- It is a frozen snapshot of settled facts: no narrative, no dates, no changelog. A stale fact is corrected in place.
- Tags are one word each. A compound idea becomes several tags.
- A new book is a folder named `title_snake_case[date][tags]`, with the date written once.
- Its `biblio/books/toc.md` line is rewritten in place: the folder name only, tags equal to the union of its chapters' tags, plus an optional one-sentence `warning:` for an obsolete book or a bare `[WIP]` for an unfinished one.

Archived copies are the permanent record and are never deleted.

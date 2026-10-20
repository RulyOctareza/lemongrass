A chapter records how the system works, never how it broke. A fix that restores intended behavior gets no chapter, and its root cause stays in the archived `debug.md`. Write or extend a chapter only when the task settled a new design, mechanism or decision a future reader needs. A recon that ended in "not worth pursuing" just archives.

When a chapter is written or changed:

- It is a frozen snapshot of settled facts: no narrative, no dates, no changelog. A stale fact is corrected in place.
- A book is a folder named `title_snake_case` and a chapter a file named `title_snake_case.md`, with no date or tags in either name.
- Tags live only on the book's toc line. Each is one token, lowercase or camelCase, so a compound idea is one camelCase tag.
- Its `biblio/books/toc.md` line is rewritten in place: `title_snake_case [tag,camelTag]`, plus an optional one-sentence `warning:` for an obsolete book.

Archived copies are the permanent record and are never deleted.

package session

import "math/rand"

// Loading-screen-style reminders, surfaced periodically alongside the
// regular nudge -- only when the current project has actually adopted
// bibliothek (biblio/ exists), so they're never noise for one that hasn't.
var bibliothekTips = []string{
	"bibliothek tip: a handover is never written for finished work -- that's a book chapter.",
	"bibliothek tip: handover is temporary, permanence is a book entry.",
	"bibliothek tip: finishing a task includes writing its book chapter, same as running its tests.",
	"bibliothek tip: memory only ever points at biblio/handover/<task>.md or biblio/books/toc.md -- never a hardcoded book path.",
	"bibliothek tip: a task's dump/ folder holds work expensive to produce, such as a PoC. Test builds, logs and probes go to the system temp dir. The task directory becomes the permanent record in the archive once the task closes.",
	"bibliothek tip: check biblio/books/toc.md before researching something from scratch -- it might already be covered.",
	"bibliothek tip: a handover's Status section is the truth, its Log is history only -- don't read the Log for current state.",
	"bibliothek tip: once a handover's work is done, archive it -- biblio/handover/ should only ever hold what's still in flight.",
	"bibliothek tip: tags are one word each -- a compound idea is multiple tags, not one hyphenated tag.",
	"bibliothek tip: a plan for work that hasn't started lives in scratchpad -- a handover's Status section tracks an already-open task, not a future one.",
	"bibliothek tip: scratchpad is editable by the model too, not just the user. It is the working space for planning, debugging and every work file, not only somewhere the user drops notes.",
}

func RandomTip() string {
	return RandomTipFrom(nil)
}

// RandomTipFrom picks from the built-in tips plus whatever user-authored ones this project has configured.
func RandomTipFrom(extra []string) string {
	all := append(append([]string{}, bibliothekTips...), extra...)
	return all[rand.Intn(len(all))]
}

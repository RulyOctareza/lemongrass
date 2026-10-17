package main

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/workgroup"
)

func TestDeclinedMessageSeparatesReasonNoReasonAndWithdrawal(t *testing.T) {
	cases := []struct {
		answer workgroup.Answer
		want   string
	}{
		{workgroup.Answer{Reason: "use haiku"}, "Their reason: use haiku"},
		{workgroup.Answer{}, "declined without a reason"},
		{workgroup.Answer{Withdrawn: true}, "withdrawn before the human answered"},
	}
	for _, c := range cases {
		got := declinedMessage(c.answer)
		if !strings.HasPrefix(got, "lgrass workgroup: ") || !strings.Contains(got, c.want) {
			t.Errorf("declinedMessage(%+v) = %q, want %q", c.answer, got, c.want)
		}
	}
}

func TestJoinMessageNamesEveryNewThinker(t *testing.T) {
	one := joinMessage([]workgroup.SpawnMember{{Label: "docs", Vendor: "codex"}})
	if one != "lgrass: docs (codex) joins the group as a new thinker, approved by the human." {
		t.Errorf("one = %q", one)
	}
	two := joinMessage([]workgroup.SpawnMember{{Label: "docs", Vendor: "codex"}, {Label: "perf", Vendor: "claude"}})
	if !strings.Contains(two, "docs (codex), perf (claude) join the group") || strings.Contains(two, "!>>") {
		t.Errorf("two = %q", two)
	}
}

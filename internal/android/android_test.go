// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package android

import (
	"errors"
	"strings"
	"testing"
)

const dump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?><hierarchy rotation="0">` +
	`<node text="" resource-id="android:id/content" class="android.widget.FrameLayout" content-desc="" clickable="false" bounds="[0,0][1080,2340]">` +
	`<node text="" resource-id="" class="android.view.View" content-desc="Skip" clickable="true" bounds="[865,126][1008,228]" />` +
	`<node text="" resource-id="" class="android.view.View" content-desc="Welcome back" clickable="false" bounds="[84,1545][1008,1650]" />` +
	`<node text="" resource-id="" class="android.view.View" content-desc="Continue" clickable="true" bounds="[84,1866][1008,2022]">` +
	`<node text="" resource-id="" class="android.view.View" content-desc="Continue" clickable="false" bounds="[400,1900][700,1990]" /></node>` +
	`<node text="" resource-id="login_email" class="android.widget.EditText" content-desc="" clickable="true" bounds="[84,600][1008,700]" />` +
	`<node text="" resource-id="" class="android.view.View" content-desc="Total&#10;Rp 10.000" clickable="false" bounds="[84,800][1008,900]" />` +
	`<node text="" resource-id="" class="android.view.View" content-desc="" clickable="false" bounds="[0,0][0,0]" />` +
	`</node></hierarchy>UI hierchary dumped to: /dev/tty`

func parsed(t *testing.T) []Node {
	t.Helper()
	nodes, err := ParseHierarchy([]byte(dump))
	if err != nil {
		t.Fatalf("ParseHierarchy: %v", err)
	}
	return nodes
}

func TestParseDevices(t *testing.T) {
	out := "List of devices attached\nABC123 device usb:1-1 product:x model:Pixel_8 device:y transport_id:1\nXYZ unauthorized usb:1-2 transport_id:2\n\n"
	got := ParseDevices(out)
	if len(got) != 2 || got[0].Model != "Pixel-8" || got[1].State != "unauthorized" || got[1].Model != "XYZ" {
		t.Fatalf("ParseDevices = %+v", got)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{"tap": "tap", "540": "540", "a b": "'a b'", "it's": `'it'\''s'`, "%s": "%s"}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseHierarchyIgnoresTrailingText(t *testing.T) {
	nodes := parsed(t)
	if len(nodes) != 8 {
		t.Fatalf("got %d nodes, want 8", len(nodes))
	}
	if nodes[1].Desc != "Skip" || !nodes[1].Clickable || nodes[1].Bounds != [4]int{865, 126, 1008, 228} {
		t.Fatalf("node = %+v", nodes[1])
	}
}

func TestParseHierarchyReportsAFailedDump(t *testing.T) {
	_, err := ParseHierarchy([]byte("ERROR: could not get idle state.\n"))
	if err == nil || !strings.Contains(err.Error(), "could not get idle state") {
		t.Fatalf("err = %v", err)
	}
}

func TestVisibleDropsEmptyAndZeroSizedNodes(t *testing.T) {
	for _, n := range Visible(parsed(t)) {
		if n.Bounds[2] == 0 || n.ID == "android:id/content" {
			t.Fatalf("kept %+v", n)
		}
	}
}

func TestFindByTextPrefersTheClickableMatch(t *testing.T) {
	n, err := FindByText(parsed(t), "Continue")
	if err != nil {
		t.Fatalf("FindByText: %v", err)
	}
	if x, y := n.Center(); x != 546 || y != 1944 {
		t.Fatalf("center = %d,%d", x, y)
	}
}

func TestFindByTextFallsBackToCaseAndSubstring(t *testing.T) {
	if n, err := FindByText(parsed(t), "skip"); err != nil || n.Desc != "Skip" {
		t.Fatalf("case: %+v %v", n, err)
	}
	if n, err := FindByText(parsed(t), "welcome"); err != nil || n.Desc != "Welcome back" {
		t.Fatalf("substring: %+v %v", n, err)
	}
	if n, err := FindByText(parsed(t), "Rp 10.000"); err != nil || !strings.HasPrefix(n.Desc, "Total") {
		t.Fatalf("line: %+v %v", n, err)
	}
}

func TestFindByTextMissing(t *testing.T) {
	_, err := FindByText(parsed(t), "Checkout")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindByID(t *testing.T) {
	n, err := FindByID(parsed(t), "login_email")
	if err != nil || n.Class != "android.widget.EditText" {
		t.Fatalf("FindByID: %+v %v", n, err)
	}
}

func TestFormat(t *testing.T) {
	got := Format(parsed(t)[1])
	if got != `(936,177) "Skip" tap [865,126 1008,228]` {
		t.Fatalf("Format = %s", got)
	}
}

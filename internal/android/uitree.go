package android

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Node is one element of the accessibility tree. Flutter puts a widget's semantics label in
// Desc, and a Semantics identifier in ID.
type Node struct {
	Text      string
	Desc      string
	ID        string
	Class     string
	Clickable bool
	Bounds    [4]int
}

// Label is what a person reads on the node.
func (n Node) Label() string {
	if n.Text != "" {
		return n.Text
	}
	return n.Desc
}

// Center is where a tap on the node lands.
func (n Node) Center() (int, int) {
	return (n.Bounds[0] + n.Bounds[2]) / 2, (n.Bounds[1] + n.Bounds[3]) / 2
}

type xmlNode struct {
	Text      string    `xml:"text,attr"`
	Desc      string    `xml:"content-desc,attr"`
	ID        string    `xml:"resource-id,attr"`
	Class     string    `xml:"class,attr"`
	Clickable string    `xml:"clickable,attr"`
	Bounds    string    `xml:"bounds,attr"`
	Children  []xmlNode `xml:"node"`
}

var boundsPattern = regexp.MustCompile(`^\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]$`)

// ParseHierarchy reads uiautomator's XML in document order. Text after the closing tag, such as
// the "dumped to" line uiautomator prints to the same stream, is ignored.
func ParseHierarchy(data []byte) ([]Node, error) {
	start := bytes.Index(data, []byte("<hierarchy"))
	end := bytes.LastIndex(data, []byte("</hierarchy>"))
	if start < 0 || end < 0 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = "empty output"
		}
		return nil, fmt.Errorf("android: no UI hierarchy in the dump: %s", msg)
	}
	var root struct {
		Nodes []xmlNode `xml:"node"`
	}
	if err := xml.Unmarshal(data[start:end+len("</hierarchy>")], &root); err != nil {
		return nil, fmt.Errorf("android: reading the UI hierarchy: %w", err)
	}
	var out []Node
	var walk func([]xmlNode)
	walk = func(nodes []xmlNode) {
		for _, x := range nodes {
			n := Node{Text: x.Text, Desc: x.Desc, ID: x.ID, Class: x.Class, Clickable: x.Clickable == "true"}
			if m := boundsPattern.FindStringSubmatch(x.Bounds); m != nil {
				for i := range 4 {
					n.Bounds[i], _ = strconv.Atoi(m[i+1])
				}
			}
			out = append(out, n)
			walk(x.Children)
		}
	}
	walk(root.Nodes)
	return out, nil
}

// Visible keeps the nodes worth showing a reader: those with a label or id, or that react to a tap.
func Visible(nodes []Node) []Node {
	var out []Node
	for _, n := range nodes {
		if n.Bounds[2] <= n.Bounds[0] || n.Bounds[3] <= n.Bounds[1] {
			continue
		}
		if n.Text != "" || n.Desc != "" || (n.ID != "" && !strings.HasPrefix(n.ID, "android:")) || n.Clickable {
			out = append(out, n)
		}
	}
	return out
}

// ErrNotFound is returned when no node matches.
var ErrNotFound = errors.New("android: no element matches")

// FindByID matches a resource id exactly, with or without a package prefix.
func FindByID(nodes []Node, id string) (Node, error) {
	var matches []Node
	for _, n := range nodes {
		if n.ID == id || strings.HasSuffix(n.ID, ":id/"+id) {
			matches = append(matches, n)
		}
	}
	return pick(matches, "id "+strconv.Quote(id))
}

// FindByText matches the label exactly first, then ignoring case, then as a substring, and
// takes the first tier with any match. A label Flutter joins over several lines still matches
// one of its lines.
func FindByText(nodes []Node, text string) (Node, error) {
	tiers := []func(string) bool{
		func(label string) bool { return label == text },
		func(label string) bool { return strings.EqualFold(label, text) },
		func(label string) bool { return strings.Contains(strings.ToLower(label), strings.ToLower(text)) },
	}
	for _, match := range tiers {
		var matches []Node
		for _, n := range nodes {
			if match(n.Text) || match(n.Desc) || anyLine(n.Desc, match) {
				matches = append(matches, n)
			}
		}
		if len(matches) > 0 {
			return pick(matches, "text "+strconv.Quote(text))
		}
	}
	return Node{}, fmt.Errorf("%w text %q", ErrNotFound, text)
}

func anyLine(s string, match func(string) bool) bool {
	if !strings.Contains(s, "\n") {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		if match(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

// pick prefers a clickable match, since a label is often drawn inside the button it names.
func pick(matches []Node, what string) (Node, error) {
	if len(matches) == 0 {
		return Node{}, fmt.Errorf("%w %s", ErrNotFound, what)
	}
	var clickable []Node
	for _, n := range matches {
		if n.Clickable {
			clickable = append(clickable, n)
		}
	}
	if len(clickable) == 1 {
		return clickable[0], nil
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return Node{}, fmt.Errorf("android: %d elements match %s, use a longer text or tap by coordinates from lgrass device tree", len(matches), what)
}

// Format prints a node as one line of lgrass device tree.
func Format(n Node) string {
	var b strings.Builder
	x, y := n.Center()
	fmt.Fprintf(&b, "(%d,%d)", x, y)
	if label := n.Label(); label != "" {
		fmt.Fprintf(&b, " %q", strings.ReplaceAll(label, "\n", " / "))
	}
	if n.ID != "" {
		fmt.Fprintf(&b, " id=%s", n.ID)
	}
	if n.Clickable {
		b.WriteString(" tap")
	}
	fmt.Fprintf(&b, " [%d,%d %d,%d]", n.Bounds[0], n.Bounds[1], n.Bounds[2], n.Bounds[3])
	return b.String()
}

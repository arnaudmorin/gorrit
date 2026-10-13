package ui

import "testing"

func TestParseBugRefs(t *testing.T) {
	msg := "Fix it\n\nCloses-Bug: #2170284\nRelated-Bug: 2141355\nrelated-bug: #2141355\nPartial-Bug: #1\nChange-Id: I123\n"
	refs := parseBugRefs(msg)
	want := []bugRef{{"Closes", "2170284"}, {"Related", "2141355"}, {"Partial", "1"}}
	if len(refs) != len(want) {
		t.Fatalf("got %v", refs)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("got %v want %v", refs, want)
		}
	}
	if got := bugURL("https://bugs.launchpad.net/bugs/{id}", "42"); got != "https://bugs.launchpad.net/bugs/42" {
		t.Fatal(got)
	}
}

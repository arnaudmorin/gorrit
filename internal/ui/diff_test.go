package ui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestBuildDiffCommandLive(t *testing.T) {
	url := os.Getenv("GORRIT_TEST_URL")
	if url == "" {
		t.Skip("GORRIT_TEST_URL not set")
	}
	client := gerrit.New(url, "", "")
	results, err := client.QueryChanges(context.Background(), "is:open project:openstack/neutron", gerrit.QueryOptions{Limit: 1})
	if err != nil || len(results) == 0 {
		t.Fatalf("query: %v", err)
	}
	c := results[0]
	id := gerrit.ChangeID(c.Project, c.Number)
	t.Logf("change %d %s", c.Number, c.Subject)

	for _, command := range []string{"git -c core.pager=cat diff --no-index {old} {new}; true", "cat", "wc -l {patch}"} {
		dir := t.TempDir()
		cmd, err := buildDiffCommand(context.Background(), client, id, command, dir)
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", command, err, out)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		t.Logf("%-55s -> %d lines, first: %s", command, len(lines), lines[0])
		if command == "cat" && !strings.Contains(string(out), "diff --git") {
			t.Fatalf("piped patch is not a diff:\n%.300s", out)
		}
	}
}

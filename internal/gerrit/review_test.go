package gerrit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetReviewReply(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.EscapedPath()
		gotAuth, _, _ = r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		w.Write([]byte(")]}'\n{\"labels\":{}}"))
	}))
	defer srv.Close()

	client := New(srv.URL, "alice", "tok")
	unresolved := false
	in := ReviewInput{Comments: map[string][]CommentInput{
		"a/b.py": {{Line: 12, InReplyTo: "abc", Message: "Done", Unresolved: &unresolved}},
	}}
	if _, err := client.SetReview(context.Background(), ChangeID("foo/bar", 42), "3", in); err != nil {
		t.Fatal(err)
	}
	if want := "POST /a/changes/foo%2Fbar~42/revisions/3/review"; gotPath != want {
		t.Fatalf("path %q, want %q", gotPath, want)
	}
	if gotAuth != "alice" {
		t.Fatalf("basic auth user %q", gotAuth)
	}
	comment := gotBody["comments"].(map[string]any)["a/b.py"].([]any)[0].(map[string]any)
	if comment["in_reply_to"] != "abc" || comment["message"] != "Done" || comment["unresolved"] != false || comment["line"] != float64(12) {
		t.Fatalf("unexpected comment payload: %v", comment)
	}
	if _, ok := comment["range"]; ok {
		t.Fatalf("nil range must be omitted: %v", comment)
	}
}

package gerrit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetStarred(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	client := New(srv.URL, "alice", "tok")
	id := ChangeID("openstack/nova", 42)
	if err := client.SetStarred(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	if err := client.SetStarred(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /a/accounts/self/starred.changes/openstack%2Fnova~42",
		"DELETE /a/accounts/self/starred.changes/openstack%2Fnova~42",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

package ui

import "testing"

const samplePatch = `From abc Mon Sep 17 00:00:00 2001
From: A <a@x>
Subject: [PATCH] x

---
 f.py | 2 +-
 1 file changed

diff --git a/f.py b/f.py
index 1..2 100644
--- a/f.py
+++ b/f.py
@@ -10,3 +10,4 @@ def f():
 keep
-old
+new
+added
 tail
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
-- 
2.43.0
`

func TestParsePatch(t *testing.T) {
	files := parsePatch([]byte(samplePatch))
	if len(files) != 2 {
		t.Fatalf("got %d files", len(files))
	}
	f := files[0]
	if f.path != "f.py" || f.status != "M" {
		t.Fatalf("file %+v", f)
	}
	want := []diffLine{
		{kind: '@', text: "@@ -10,3 +10,4 @@ def f():"},
		{kind: ' ', old: 10, new: 10, text: "keep"},
		{kind: '-', old: 11, text: "old"},
		{kind: '+', new: 11, text: "new"},
		{kind: '+', new: 12, text: "added"},
		{kind: ' ', old: 12, new: 13, text: "tail"},
	}
	for i, w := range want {
		if f.lines[i] != w {
			t.Fatalf("line %d: got %+v want %+v", i, f.lines[i], w)
		}
	}
	if files[1].status != "D" || files[1].oldPath != "gone.txt" || len(files[1].lines) != 2 {
		t.Fatalf("deleted file %+v", files[1])
	}
}

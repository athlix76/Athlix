package main

import (
	"io/fs"
	"testing"

	"athlix/backend/migrations"
)

func TestSplit(t *testing.T) {
	got := Split("-- c;\nCREATE TABLE a (x INT); INSERT INTO a VALUES ('x;y', \"q\\\"; r\");\n")
	if len(got) != 2 || got[1] != `INSERT INTO a VALUES ('x;y', "q\"; r")` {
		t.Fatalf("got %q", got)
	}
}

func TestMigrationsSplitCleanly(t *testing.T) {
	names, _ := fs.Glob(migrations.FS, "*.sql")
	if len(names) == 0 {
		t.Fatal("no migrations embedded")
	}
	for _, n := range names {
		b, _ := migrations.FS.ReadFile(n)
		if len(Split(string(b))) == 0 {
			t.Fatalf("%s produced no statements", n)
		}
	}
}

package hubdb

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestResolve_PrefersParentWithMoreHubData(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "monitor")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(sub, DefaultName)
	parent := filepath.Join(root, DefaultName)
	seedEmpty(t, local, 2)
	seedHub(t, parent, 5)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	got, hint := Resolve(DefaultName)
	want := "../" + DefaultName
	if got != want {
		t.Fatalf("Resolve() path = %q, want %q", got, want)
	}
	if hint == "" {
		t.Fatal("expected hint when switching databases")
	}
}

func seedEmpty(t *testing.T, path string, version int) {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA user_version = ` + strconv.Itoa(version)); err != nil {
		t.Fatal(err)
	}
}

func seedHub(t *testing.T, path string, version int) {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA user_version = ` + strconv.Itoa(version)); err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	for i := range id {
		id[i] = byte(i + 1)
	}
	now := int64(1_700_000_000_000)
	if _, err := db.Exec(
		`INSERT INTO hub_registry (hub_id, first_seen, last_seen, status) VALUES (?, ?, ?, 'online')`,
		id, now, now,
	); err != nil {
		t.Fatal(err)
	}
}

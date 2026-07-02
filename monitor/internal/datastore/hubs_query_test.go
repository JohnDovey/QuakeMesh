package datastore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestHubs_ReturnsRowsFromCopiedDB(t *testing.T) {
	src := filepath.Join("..", "..", "..", "quakemeshhub.db")
	if _, err := os.Stat(src); err != nil {
		t.Skip("no live quakemeshhub.db")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hub.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := New(db)
	hubs, err := s.Hubs()
	if err != nil {
		t.Fatalf("Hubs(): %v", err)
	}
	if len(hubs) == 0 {
		t.Fatal("expected hubs, got 0")
	}
	b, err := json.Marshal(hubs[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first hub json: %s", b)
	if hubs[0].HubID == "" {
		t.Fatal("hub_id empty in struct")
	}
}

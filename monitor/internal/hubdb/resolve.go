package hubdb

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

const DefaultName = "quakemeshhub.db"

type snapshot struct {
	path    string
	version int
	hubs    int
	nodes   int
	size    int64
}

// Resolve picks the best hub registry file when the default relative path
// is ambiguous (for example `cd monitor && go run .` vs hub at repo root).
// The returned hint is non-empty when a different path than configured was chosen.
func Resolve(configured string) (path string, hint string) {
	if configured != DefaultName {
		return configured, ""
	}
	candidates := []string{DefaultName, "../" + DefaultName, "../hub/" + DefaultName}
	var snaps []snapshot
	for _, candidate := range candidates {
		s, err := inspect(candidate)
		if err != nil {
			continue
		}
		snaps = append(snaps, s)
	}
	if len(snaps) == 0 {
		return configured, ""
	}
	best := snaps[0]
	for _, s := range snaps[1:] {
		if betterThan(s, best) {
			best = s
		}
	}
	if best.path != configured {
		return best.path, fmt.Sprintf(
			"using %s instead of %s (schema v%d, %d hubs, %d nodes); pass -hub-db to override",
			best.path, configured, best.version, best.hubs, best.nodes,
		)
	}
	return configured, ""
}

func betterThan(a, b snapshot) bool {
	if a.version != b.version {
		return a.version > b.version
	}
	if a.hubs != b.hubs {
		return a.hubs > b.hubs
	}
	if a.nodes != b.nodes {
		return a.nodes > b.nodes
	}
	return a.size > b.size
}

func inspect(path string) (snapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return snapshot{}, err
	}
	if info.IsDir() {
		return snapshot{}, fmt.Errorf("is directory")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return snapshot{}, err
	}
	defer db.Close()

	s := snapshot{path: path, size: info.Size()}
	_ = db.QueryRow(`PRAGMA user_version`).Scan(&s.version)
	_ = db.QueryRow(`SELECT COUNT(*) FROM hub_registry`).Scan(&s.hubs)
	_ = db.QueryRow(`SELECT COUNT(*) FROM node_registry`).Scan(&s.nodes)
	return s, nil
}

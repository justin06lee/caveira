package cli

import (
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/justin06lee/caveira/internal/fabricate"
	"github.com/justin06lee/caveira/internal/input"
	"github.com/justin06lee/caveira/internal/walk"
)

// resolveLeechPool builds the identity pool used to scatter authorship across an
// existing history: the N resolved leeches plus every original human author in
// the source (mailmap-applied, AI agents excluded by DiscoverIdentities).
// Deduplicated by lowercased email so a leech who is also an original author is
// not double-weighted. Order is deterministic: leeches first (in resolution
// order), then originals by descending real commit count.
func resolveLeechPool(srcRepo *git.Repository, cfg *input.Config, mm *fabricate.Mailmap, stdin io.Reader, out io.Writer) ([]fabricate.Identity, error) {
	leeches, err := fabricate.ResolveIdentities(srcRepo, cfg.LeechIdentities, cfg.LeechesN, mm, cfg.Pick, stdin, out)
	if err != nil {
		return nil, err
	}
	originals, err := fabricate.DiscoverIdentities(srcRepo, mm)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var pool []fabricate.Identity
	add := func(id fabricate.Identity) {
		key := strings.ToLower(strings.TrimSpace(id.Email))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(id.Name))
		}
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		pool = append(pool, id)
	}
	for _, id := range leeches {
		add(id)
	}
	for _, d := range originals {
		add(d.Identity)
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("no identities to scatter (supply --leech \"Name <email>\")")
	}
	return pool, nil
}

// scatterLeechAuthors reassigns each commit's author and committer to a random
// identity drawn uniformly from pool, mutating the DAG in place. Both author and
// committer become the same drawn identity, so each commit reads as one person's
// work. Assignment walks the DAG in topological order so a fixed --seed yields a
// fixed scatter. Returns per-identity commit counts, keyed by "Name <email>".
func scatterLeechAuthors(dag *walk.DAG, pool []fabricate.Identity, rng *rand.Rand) map[string]int {
	order, err := dag.TopologicalOrder()
	if err != nil {
		// Topological order can only fail on a cyclic DAG, which a real git
		// history never is; fall back to a stable OID sort so we still assign.
		order = sortedOIDs(dag)
	}
	counts := map[string]int{}
	for _, oid := range order {
		c := dag.Get(oid)
		if c == nil {
			continue
		}
		id := pool[rng.Intn(len(pool))]
		c.Author = walk.Person{Name: id.Name, Email: id.Email}
		c.Committer = walk.Person{Name: id.Name, Email: id.Email}
		counts[fmt.Sprintf("%s <%s>", id.Name, id.Email)]++
	}
	return counts
}

func sortedOIDs(dag *walk.DAG) []string {
	all := dag.All()
	oids := make([]string, 0, len(all))
	for _, c := range all {
		oids = append(oids, c.OID)
	}
	sort.Strings(oids)
	return oids
}

// formatLeechScatter renders the scatter for the summary / dry-run: one line per
// identity with its assigned commit count, most-assigned first.
func formatLeechScatter(counts map[string]int) string {
	type row struct {
		name  string
		count int
	}
	rows := make([]row, 0, len(counts))
	for name, n := range counts {
		rows = append(rows, row{name, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].name < rows[j].name
	})
	var b strings.Builder
	fmt.Fprintf(&b, "Authors scattered across %d identities:\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-40s %d commits\n", r.name, r.count)
	}
	return b.String()
}

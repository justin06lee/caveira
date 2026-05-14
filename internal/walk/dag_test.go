package walk

import "testing"

func TestDAG_AddAndLookup(t *testing.T) {
	d := NewDAG()
	a := &Commit{OID: "a"}
	b := &Commit{OID: "b", Parents: []string{"a"}}
	d.Add(a)
	d.Add(b)
	if got := d.Get("a"); got != a {
		t.Errorf("expected to retrieve a, got %v", got)
	}
	if got := d.Get("b"); got != b {
		t.Errorf("expected to retrieve b, got %v", got)
	}
}

func TestDAG_TopologicalOrder(t *testing.T) {
	d := NewDAG()
	d.Add(&Commit{OID: "a"})
	d.Add(&Commit{OID: "b", Parents: []string{"a"}})
	d.Add(&Commit{OID: "c", Parents: []string{"a"}})
	d.Add(&Commit{OID: "m", Parents: []string{"b", "c"}})

	order, err := d.TopologicalOrder()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	pos := map[string]int{}
	for i, oid := range order {
		pos[oid] = i
	}
	if pos["a"] >= pos["b"] || pos["a"] >= pos["c"] {
		t.Errorf("a must come before b and c, got %v", order)
	}
	if pos["b"] >= pos["m"] || pos["c"] >= pos["m"] {
		t.Errorf("merge m must come last, got %v", order)
	}
}

func TestDAG_Children(t *testing.T) {
	d := NewDAG()
	d.Add(&Commit{OID: "a"})
	d.Add(&Commit{OID: "b", Parents: []string{"a"}})
	d.Add(&Commit{OID: "c", Parents: []string{"a"}})

	kids := d.Children("a")
	if len(kids) != 2 {
		t.Fatalf("expected 2 children, got %d", len(kids))
	}
}

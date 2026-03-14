package terminal

import (
	"testing"
)

func TestRegistryNextID(t *testing.T) {
	r := NewRegistry()
	id1 := r.NextID()
	id2 := r.NextID()
	if id1 == id2 {
		t.Error("IDs should be unique")
	}
	if id1 != "t1" {
		t.Errorf("first ID should be t1, got %s", id1)
	}
	if id2 != "t2" {
		t.Errorf("second ID should be t2, got %s", id2)
	}
}

func TestRegistryAddGetRemove(t *testing.T) {
	r := NewRegistry()
	term := &Terminal{ID: "t1", Owner: "p1"}
	r.Add(term)

	if got := r.Get("t1"); got != term {
		t.Error("should find added terminal")
	}
	if got := r.Get("t99"); got != nil {
		t.Error("should not find non-existent terminal")
	}

	r.Remove("t1")
	if got := r.Get("t1"); got != nil {
		t.Error("should not find removed terminal")
	}
}

func TestRegistryListAll(t *testing.T) {
	r := NewRegistry()
	r.Add(&Terminal{ID: "t1", Owner: "p1"})
	r.Add(&Terminal{ID: "t2", Owner: "p2"})
	r.Add(&Terminal{ID: "t3", Owner: "p1"})

	all := r.List("")
	if len(all) != 3 {
		t.Errorf("expected 3 terminals, got %d", len(all))
	}
}

func TestRegistryListByOwner(t *testing.T) {
	r := NewRegistry()
	r.Add(&Terminal{ID: "t1", Owner: "p1"})
	r.Add(&Terminal{ID: "t2", Owner: "p2"})
	r.Add(&Terminal{ID: "t3", Owner: "p1"})

	p1 := r.List("p1")
	if len(p1) != 2 {
		t.Errorf("expected 2 terminals for p1, got %d", len(p1))
	}
	for _, term := range p1 {
		if term.Owner != "p1" {
			t.Errorf("terminal %s has owner %s, expected p1", term.ID, term.Owner)
		}
	}

	p2 := r.List("p2")
	if len(p2) != 1 {
		t.Errorf("expected 1 terminal for p2, got %d", len(p2))
	}
}

func TestRegistryCount(t *testing.T) {
	r := NewRegistry()
	if r.Count() != 0 {
		t.Error("empty registry should have count 0")
	}
	r.Add(&Terminal{ID: "t1"})
	if r.Count() != 1 {
		t.Error("should have count 1 after add")
	}
	r.Remove("t1")
	if r.Count() != 0 {
		t.Error("should have count 0 after remove")
	}
}

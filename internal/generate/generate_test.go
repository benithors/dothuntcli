package generate

import "testing"

func TestLabels_KIAgentur(t *testing.T) {
	t.Parallel()

	g := New(Options{
		MaxLabels:  200,
		ReplaceKI:  true,
		Reverse2:   true,
		KeepHyphen: true,
	})

	cands := g.Labels("Ki agentur")
	if len(cands) == 0 {
		t.Fatalf("expected candidates, got none")
	}

	seen := map[string]int{}
	for _, c := range cands {
		seen[c.Label] = c.Score
	}

	if score, ok := seen["ki-agentur"]; !ok || score == 0 {
		t.Fatalf("expected ki-agentur candidate, got: %v", seen)
	}
	if _, ok := seen["ai-agentur"]; !ok {
		t.Fatalf("expected ai-agentur candidate (KI<->AI replacement), got: %v", seen)
	}
	if _, ok := seen["agentur-ki"]; !ok {
		t.Fatalf("expected agentur-ki candidate (reverse), got: %v", seen)
	}
}

func TestLabels_Multiword(t *testing.T) {
	t.Parallel()

	g := New(Options{
		MaxLabels:  500,
		ReplaceKI:  true,
		Reverse2:   true,
		KeepHyphen: true,
	})

	cands := g.Labels("ki agent agentic engineering")
	if len(cands) == 0 {
		t.Fatalf("expected candidates, got none")
	}

	seen := map[string]struct{}{}
	for _, c := range cands {
		seen[c.Label] = struct{}{}
	}

	// Useful subsets should exist (not only the full 4-token concatenation).
	if _, ok := seen["agentic-engineering"]; !ok {
		t.Fatalf("expected agentic-engineering candidate; got %d candidates", len(cands))
	}
	if _, ok := seen["ki-agentic-engineering"]; !ok {
		t.Fatalf("expected ki-agentic-engineering candidate; got %d candidates", len(cands))
	}
	if _, ok := seen["agentic-eng"]; !ok {
		t.Fatalf("expected agentic-eng candidate (abbrev); got %d candidates", len(cands))
	}
}

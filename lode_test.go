package lode

import (
	"context"
	"testing"
)

type Author struct {
	ID   int
	Name string
	Handle
}

type Book struct {
	ID       int
	AuthorID int
	Title    string
	Handle
}

func TestResolve_Basic(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()

	a1 := &Author{ID: 1, Name: "Alice"}
	a2 := &Author{ID: 2, Name: "Bob"}
	eng.InitHandles([]*Author{a1, a2})

	buildCalls := 0
	build := func(ctx context.Context, models []*Author) (ResolverFunc[*Author, string], error) {
		buildCalls++
		m := make(map[*Author]string, len(models))
		for _, a := range models {
			m[a] = a.Name + "!"
		}
		return func(a *Author) string { return m[a] }, nil
	}

	const key = "author:greeting"

	got1, err := Resolve(ctx, ResolveSpec[*Author, string]{
		CacheKey: key,
		Model:    a1,
		Build:    build,
	})
	if err != nil {
		t.Fatalf("Resolve(a1) error: %v", err)
	}
	if got1 != "Alice!" {
		t.Fatalf("Resolve(a1) = %q; want %q", got1, "Alice!")
	}

	// Same cache key should reuse the built resolver for a2.
	got2, err := Resolve(ctx, ResolveSpec[*Author, string]{
		CacheKey: key,
		Model:    a2,
		Build:    build,
	})
	if err != nil {
		t.Fatalf("Resolve(a2) error: %v", err)
	}
	if got2 != "Bob!" {
		t.Fatalf("Resolve(a2) = %q; want %q", got2, "Bob!")
	}

	if buildCalls != 1 {
		t.Fatalf("build called %d times; want 1", buildCalls)
	}
}

// --- Many ---

func TestMany_Basic(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()

	a1 := &Author{ID: 1, Name: "Alice"}
	a2 := &Author{ID: 2, Name: "Bob"}
	a3 := &Author{ID: 3, Name: "NoBooks"}
	eng.InitHandles([]*Author{a1, a2, a3})

	all := []*Book{
		{ID: 1, AuthorID: 1, Title: "A1-First"},
		{ID: 2, AuthorID: 1, Title: "A1-Second"},
		{ID: 3, AuthorID: 2, Title: "A2-Only"},
	}

	fetch := func(_ context.Context, keys []int) ([]*Book, error) {
		set := map[int]struct{}{}
		for _, k := range keys {
			set[k] = struct{}{}
		}
		var out []*Book
		for _, b := range all {
			if _, ok := set[b.AuthorID]; ok {
				out = append(out, b)
			}
		}
		return out, nil
	}

	spec := RelationSpec[int, *Author, *Book]{
		CacheKey:    "author:books",
		Model:       a1,
		ModelKey:    func(a *Author) (int, bool) { return a.ID, true },
		RelationKey: func(b *Book) int { return b.AuthorID },
		Fetch:       fetch,
	}

	books1, err := Many(ctx, spec)
	if err != nil {
		t.Fatalf("Many(a1) error: %v", err)
	}
	titles1 := titles(books1)
	if want := []string{"A1-First", "A1-Second"}; !equalStrings(titles1, want) {
		t.Fatalf("Many(a1) titles %v; want %v", titles1, want)
	}

	spec.Model = a2
	books2, err := Many(ctx, spec)
	if err != nil {
		t.Fatalf("Many(a2) error: %v", err)
	}
	titles2 := titles(books2)
	if want := []string{"A2-Only"}; !equalStrings(titles2, want) {
		t.Fatalf("Many(a2) titles %v; want %v", titles2, want)
	}

	spec.Model = a3
	books3, err := Many(ctx, spec)
	if err != nil {
		t.Fatalf("Many(a3) error: %v", err)
	}
	if len(books3) != 0 {
		t.Fatalf("Many(a3) len=%d; want 0", len(books3))
	}
}

// --- One ---

func TestOne_BasicAndEmpty(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()

	a1 := &Author{ID: 1, Name: "Alice"}
	a2 := &Author{ID: 2, Name: "NoBooks"}
	eng.InitHandles([]*Author{a1, a2})

	all := []*Book{
		{ID: 1, AuthorID: 1, Title: "FirstPick"},
		{ID: 2, AuthorID: 1, Title: "SecondPick"},
	}

	fetch := func(_ context.Context, keys []int) ([]*Book, error) {
		set := map[int]struct{}{}
		for _, k := range keys {
			set[k] = struct{}{}
		}
		var out []*Book
		for _, b := range all {
			if _, ok := set[b.AuthorID]; ok {
				out = append(out, b)
			}
		}
		return out, nil
	}

	spec := RelationSpec[int, *Author, *Book]{
		CacheKey:    "author:firstBook",
		Model:       a1,
		ModelKey:    func(a *Author) (int, bool) { return a.ID, true },
		RelationKey: func(b *Book) int { return b.AuthorID },
		Fetch:       fetch,
	}

	// Has books → pick first in fetch order
	b1, err := One(ctx, spec)
	if err != nil {
		t.Fatalf("One(a1) error: %v", err)
	}
	if b1.Title != "FirstPick" {
		t.Fatalf("One(a1) = %q; want %q", b1.Title, "FirstPick")
	}

	// No books → zero value and no error
	spec.Model = a2
	b2, err := One(ctx, spec)
	if err != nil {
		t.Fatalf("One(a2) unexpected error: %v", err)
	}
	if b2 != nil { // zero value check
		t.Fatalf("One(a2) = %+v; want zero Book", b2)
	}
}

// --- tiny helpers ---

func titles(bs []*Book) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Title
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

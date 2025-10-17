package lode

import (
	"context"
	"reflect"
	"testing"
)

type Author struct {
	ID   int
	Name string
	Handle
}

type Authors []*Author    // named slice of pointers
type AuthorsVals []Author // named slice of values

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

func sameState(t *testing.T, as ...*Author) *loaderState { // same pkg to inspect state
	t.Helper()
	var first *loaderState
	for i, a := range as {
		if a == nil || a.lodeState() == nil {
			t.Fatalf("author[%d] has nil state", i)
		}
		if first == nil {
			first = a.lodeState()
		} else if a.lodeState() != first {
			t.Fatalf("author[%d] has different state", i)
		}
	}
	return first
}

func ptrsEq(a, b []*Author) bool {
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

// Extend TestInitHandles_VariousInputs with more cases.
func TestInitHandles_VariousInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() (in any, wantPtrs []*Author)
	}{
		{
			name: "single *T",
			build: func() (any, []*Author) {
				a := &Author{ID: 1}
				return a, []*Author{a}
			},
		},
		{
			name: "single **T",
			build: func() (any, []*Author) {
				a := &Author{ID: 1}
				return &a, []*Author{a}
			},
		},
		{
			name: "[]*T",
			build: func() (any, []*Author) {
				a1, a2 := &Author{ID: 1}, &Author{ID: 2}
				return []*Author{a1, a2}, []*Author{a1, a2}
			},
		},
		{
			name: "[]T",
			build: func() (any, []*Author) {
				v := []Author{{ID: 1}, {ID: 2}}
				return v, []*Author{&v[0], &v[1]}
			},
		},
		{
			name: "*[]*T",
			build: func() (any, []*Author) {
				a1, a2 := &Author{ID: 1}, &Author{ID: 2}
				ps := &[]*Author{a1, a2}
				return ps, []*Author{a1, a2}
			},
		},
		{
			name: "*[]T",
			build: func() (any, []*Author) {
				v := &[]Author{{ID: 1}, {ID: 2}}
				return v, []*Author{&(*v)[0], &(*v)[1]}
			},
		},

		// --- New coverage for named slice types ---

		{
			name: "Authors (named []*Author) → normalize to []*Author",
			build: func() (any, []*Author) {
				a1, a2 := &Author{ID: 1}, &Author{ID: 2}
				in := Authors{a1, a2}
				// want pointers unchanged
				return in, []*Author{a1, a2}
			},
		},
		{
			name: "*Authors (pointer to named []*Author) → normalize to []*Author",
			build: func() (any, []*Author) {
				a1, a2 := &Author{ID: 1}, &Author{ID: 2}
				in := &Authors{a1, a2}
				return in, []*Author{a1, a2}
			},
		},
		{
			name: "AuthorsVals (named []Author) → []*Author",
			build: func() (any, []*Author) {
				v := AuthorsVals{{ID: 1}, {ID: 2}}
				// addresses of elements in the underlying array
				return v, []*Author{&v[0], &v[1]}
			},
		},
		{
			name: "*AuthorsVals (pointer to named []Author) → []*Author",
			build: func() (any, []*Author) {
				v := &AuthorsVals{{ID: 1}, {ID: 2}}
				return v, []*Author{&(*v)[0], &(*v)[1]}
			},
		},
		{
			name: "empty Authors (named []*Author, empty) → []*Author{}",
			build: func() (any, []*Author) {
				var in Authors
				return in, nil
			},
		},
		{
			name: "empty *Authors (nil pointer allowed) → []*Author{}",
			build: func() (any, []*Author) {
				// toPtrSlice should treat a nil *slice as invalid -> InitHandles should no-op safely.
				// We still pass it to ensure no panic; want remains nil.
				var in *Authors
				return in, nil
			},
		},
		{
			name: "empty AuthorsVals (named []Author, empty) → []*Author{}",
			build: func() (any, []*Author) {
				var in AuthorsVals
				return in, nil
			},
		},
		{
			name: "empty *AuthorsVals (nil pointer allowed) → []*Author{}",
			build: func() (any, []*Author) {
				var in *AuthorsVals
				return in, nil
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := NewEngine()
			in, want := tc.build()
			e.InitHandles(in)

			// If we expected no pointers (nil/empty), ensure nothing panics and behavior is consistent.
			if len(want) == 0 {
				// If input was empty or invalid, there might be no state to inspect.
				// So just ensure that calling InitHandles didn’t attach state to some phantom Author.
				return
			}

			state := sameState(t, want...)
			ps, ok := state.models.([]*Author)
			if !ok {
				t.Fatalf("state.models not []*Author, got %T", state.models)
			}
			if !ptrsEq(ps, want) {
				t.Fatalf("state.models mismatch\n got: %#v\nwant: %#v", ps, want)
			}
		})
	}
}

func TestInitHandles_Idempotent(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	a1, a2 := &Author{ID: 1}, &Author{ID: 2}
	in := []*Author{a1, a2}

	e.InitHandles(in)
	s1 := sameState(t, a1, a2)

	// Second call should not rebind / change state.
	e.InitHandles(in)
	if a1.lodeState() != s1 || a2.lodeState() != s1 {
		t.Fatal("state changed on second InitHandles")
	}
}

func TestInitHandles_Batching(t *testing.T) {
	t.Parallel()
	e := NewEngine(WithBatchSize(2))
	a1, a2, a3, a4 := &Author{ID: 1}, &Author{ID: 2}, &Author{ID: 3}, &Author{ID: 4}
	e.InitHandles([]*Author{a1, a2, a3, a4})

	sA := sameState(t, a1, a2)
	sB := sameState(t, a3, a4)
	if sA == sB {
		t.Fatal("different batches should have distinct states")
	}
}

func TestInitHandles_Invalid_NoPanic(t *testing.T) {
	t.Parallel()
	e := NewEngine()

	cases := []any{
		nil,
		42,
		Author{ID: 1},     // single value (non-addressable via interface)
		(*Author)(nil),    // nil *T
		(*[]Author)(nil),  // nil *slice
		[]int{1, 2, 3},    // wrong elem type
		&[]int{1, 2},      // wrong *slice type
		[2]*Author{},      // array, not slice
		map[int]*Author{}, // map, not slice
		func() {},         // function
	}

	for i, in := range cases {
		func(i int, v any) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("case %d panicked: %v", i, r)
				}
			}()
			e.InitHandles(v)
		}(i, in)
	}
}

// Optional: sanity check that we’re truly storing a pointer slice type.
func TestInitHandles_ModelsAlwaysPtrSlice(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	v := []Author{{ID: 1}}
	e.InitHandles(v)
	st := (&v[0]).lodeState()
	if st == nil {
		t.Fatal("nil state")
	}
	rv := reflect.ValueOf(st.models)
	if rv.Kind() != reflect.Slice || rv.Type().Elem().Kind() != reflect.Ptr {
		t.Fatalf("models not a slice of pointers: %T", st.models)
	}
}

func TestHandleReset_ClearsCacheAndAllowsRebuild(t *testing.T) {
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

	const key = "greet"

	// 1) First round builds once and serves both models.
	got1, err := Resolve(ctx, ResolveSpec[*Author, string]{CacheKey: key, Model: a1, Build: build})
	if err != nil {
		t.Fatalf("Resolve(a1) err: %v", err)
	}
	got2, err := Resolve(ctx, ResolveSpec[*Author, string]{CacheKey: key, Model: a2, Build: build})
	if err != nil {
		t.Fatalf("Resolve(a2) err: %v", err)
	}
	if got1 != "Alice!" || got2 != "Bob!" {
		t.Fatalf("unexpected results: got1=%q got2=%q", got1, got2)
	}
	if buildCalls != 1 {
		t.Fatalf("buildCalls=%d; want 1 before reset", buildCalls)
	}

	// 2) Reset via one handle; shared state should be the same object, but empty cache.
	before := a1.lodeState()
	a1.Reset()
	after := a1.lodeState()
	if before != after {
		t.Fatalf("Reset should not replace loaderState pointer")
	}

	// 3) After reset, first Resolve should rebuild exactly once more and still work.
	got3, err := Resolve(ctx, ResolveSpec[*Author, string]{CacheKey: key, Model: a1, Build: build})
	if err != nil {
		t.Fatalf("Resolve(a1) post-reset err: %v", err)
	}
	if got3 != "Alice!" {
		t.Fatalf("post-reset Resolve(a1) = %q; want %q", got3, "Alice!")
	}
	if buildCalls != 2 {
		t.Fatalf("buildCalls=%d; want 2 after first post-reset resolve", buildCalls)
	}

	// 4) The rebuilt resolver should also serve other models without further builds.
	got4, err := Resolve(ctx, ResolveSpec[*Author, string]{CacheKey: key, Model: a2, Build: build})
	if err != nil {
		t.Fatalf("Resolve(a2) post-reset err: %v", err)
	}
	if got4 != "Bob!" {
		t.Fatalf("post-reset Resolve(a2) = %q; want %q", got4, "Bob!")
	}
	if buildCalls != 2 {
		t.Fatalf("buildCalls=%d; want still 2 (no extra build)", buildCalls)
	}
}

func TestHandleReset_UninitializedIsSafe(t *testing.T) {
	var u Author // no InitHandles
	// Should not panic:
	u.Reset()
	// Still uninitialized:
	if u.lodeState() != nil {
		t.Fatal("unexpected non-nil state after Reset on uninitialized handle")
	}
}

// Small additional test: empty named slice still yields a ptr-slice type shape when we do have a handle.
// We create a real Author to get at a state, then re-init with empty named slices and ensure no panic.
func TestInitHandles_EmptyNamedSlices_NoPanic(t *testing.T) {
	t.Parallel()
	e := NewEngine()

	// Seed a real state so sameState can inspect something if needed.
	a := &Author{ID: 1}
	e.InitHandles([]*Author{a})

	cases := []any{
		Authors{},      // empty named []*Author
		AuthorsVals{},  // empty named []Author
		&Authors{},     // pointer to empty named []*Author
		&AuthorsVals{}, // pointer to empty named []Author
	}

	for i, in := range cases {
		func(i int, v any) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("case %d panicked: %v", i, r)
				}
			}()
			e.InitHandles(v)
		}(i, in)
	}
}

func TestMany_NilModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var nilAuthor *Author
	fetchCalled := false

	spec := RelationSpec[int, *Author, *Book]{
		CacheKey: "booksByAuthor",
		Model:    nilAuthor,
		ModelKey: func(a *Author) (int, bool) {
			if a == nil {
				return 0, false
			}
			return a.ID, true
		},
		RelationKey: func(b *Book) int { return b.AuthorID },
		Fetch: func(context.Context, []int) ([]*Book, error) {
			fetchCalled = true
			return []*Book{{AuthorID: 1, Title: "should not be fetched"}}, nil
		},
	}

	got, err := Many(ctx, spec)
	if err != nil {
		t.Fatalf("Many(nil model) error = %v; want nil", err)
	}
	if got != nil {
		t.Fatalf("Many(nil model) = %#v; want nil slice", got)
	}
	if fetchCalled {
		t.Fatalf("Many(nil model) called Fetch; want not called")
	}
}

func TestOne_NilModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var nilAuthor *Author
	fetchCalled := false

	spec := RelationSpec[int, *Author, *Book]{
		CacheKey: "booksByAuthor",
		Model:    nilAuthor,
		ModelKey: func(a *Author) (int, bool) {
			if a == nil {
				return 0, false
			}
			return a.ID, true
		},
		RelationKey: func(b *Book) int { return b.AuthorID },
		Fetch: func(context.Context, []int) ([]*Book, error) {
			fetchCalled = true
			return []*Book{{AuthorID: 1, Title: "should not be fetched"}}, nil
		},
	}

	got, err := One(ctx, spec)
	if err != nil {
		t.Fatalf("One(nil model) error = %v; want nil", err)
	}
	if got != nil { // zero value for *Book is nil
		t.Fatalf("One(nil model) = %#v; want nil (*Book zero value)", got)
	}
	if fetchCalled {
		t.Fatalf("One(nil model) called Fetch; want not called")
	}
}

func TestResolve_NilModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var nilAuthor *Author
	buildCalled := false

	spec := ResolveSpec[*Author, int]{
		CacheKey: "answerForAuthor",
		Model:    nilAuthor,
		Build: func(context.Context, []*Author) (ResolverFunc[*Author, int], error) {
			buildCalled = true
			return func(*Author) int { return 42 }, nil
		},
	}

	got, err := Resolve(ctx, spec)
	if err != nil {
		t.Fatalf("Resolve(nil model) error = %v; want nil", err)
	}
	if got != 0 { // zero value for int is 0
		t.Fatalf("Resolve(nil model) = %v; want 0 (zero Result)", got)
	}
	if buildCalled {
		t.Fatalf("Resolve(nil model) called Build; want not called")
	}
}

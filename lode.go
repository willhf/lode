package lode

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

type Config struct {
	batchSize int
}

type ConfigOption func(*Config)

func WithBatchSize(batchSize int) ConfigOption {
	return func(c *Config) { c.batchSize = batchSize }
}

type Engine struct{ config Config }

func NewEngine(opts ...ConfigOption) *Engine {
	c := Config{
		batchSize: 5000,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return &Engine{config: c}
}

type Handle struct{ core *loaderState }

func (h *Handle) lodeState() *loaderState     { return h.core }
func (h *Handle) setLodeState(s *loaderState) { h.core = s }

func (h *Handle) Reset() {
	if h.core == nil {
		return
	}
	h.core.resolverEntries.Clear()
}

type hasState interface {
	lodeState() *loaderState
	setLodeState(*loaderState)
}

var _ hasState = (*Handle)(nil)

type loaderState struct {
	models          any
	engine          *Engine
	resolverEntries sync.Map
}

// InitHandles initializes the loader state for a slice of models.
// ChatGPT prefers the name "Bind".  What do you think?
func (e *Engine) InitHandles(models any) {
	if models == nil {
		return
	}
	ptrSlice, ok := toPtrSlice(models)
	if !ok || ptrSlice.Len() == 0 {
		return
	}
	e.bindPtrSlice(ptrSlice)
}

func toPtrSlice(models any) (reflect.Value, bool) {
	v := reflect.ValueOf(models)
	if !v.IsValid() {
		return reflect.Value{}, false
	}

	// Single pointer implementing hasState: make a 1-element []*T.
	if v.Kind() == reflect.Ptr {
		if _, ok := v.Interface().(hasState); ok {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			s := reflect.MakeSlice(reflect.SliceOf(v.Type()), 1, 1) // []*T
			s.Index(0).Set(v)
			return s, true
		}
	}

	// Unwrap pointers to get to a slice.
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}

	elem := v.Type().Elem()

	// Case A: slice element is already a pointer (e.g., Authors = []*Author).
	// Normalize named slice types (Authors) to the unnamed [] *Author.
	if elem.Kind() == reflect.Ptr {
		want := reflect.SliceOf(elem) // [] *Author (unnamed)
		if v.Type() == want {
			return v, true // already unnamed [] *Author
		}
		// Convert/copy into unnamed [] *Author
		out := reflect.MakeSlice(want, v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			vi := v.Index(i)
			// vi should already be *Author (or assignable) — Convert for safety.
			if vi.IsValid() && !vi.IsZero() {
				out.Index(i).Set(vi.Convert(elem))
			}
		}
		return out, true
	}

	// Case B: []T -> []*T (also ensure unnamed result type)
	ptrType := reflect.PointerTo(elem) // *T
	want := reflect.SliceOf(ptrType)   // []*T (unnamed)
	out := reflect.MakeSlice(want, v.Len(), v.Len())
	for i := 0; i < v.Len(); i++ {
		el := v.Index(i) // T
		if el.CanAddr() {
			out.Index(i).Set(el.Addr()) // *T
		}
	}
	return out, true
}

// bindPtrSlice expects a slice of pointers (e.g. []*T). It decides whether a
// (re)bind is needed, batches, and sets the shared loaderState on each element.
func (e *Engine) bindPtrSlice(ps reflect.Value) {
	// Detect whether we need to bind (nil or mixed state).
	var first *loaderState
	need := false
	for i := 0; i < ps.Len(); i++ {
		el := ps.Index(i)
		if el.Kind() != reflect.Ptr || el.IsNil() {
			continue
		}
		if hl, ok := el.Interface().(hasState); ok {
			st := hl.lodeState()
			if st == nil {
				need = true
				break
			}
			if first == nil {
				first = st
				continue
			}
			if first != st {
				need = true
				break
			}
		}
	}
	if !need {
		return
	}

	// Bind in batches; store models as []*T so Resolve's type assertion works.
	for _, br := range batchRanges(ps.Len(), e.config.batchSize) {
		sub := ps.Slice(br.StartInclusive, br.EndExclusive)
		state := &loaderState{
			models: sub.Interface(), // always []*T
			engine: e,
		}
		for i := 0; i < sub.Len(); i++ {
			el := sub.Index(i)
			if el.Kind() != reflect.Ptr || el.IsNil() {
				continue
			}
			if hl, ok := el.Interface().(hasState); ok {
				hl.setLodeState(state)
			}
		}
	}
}

type ResolverFunc[Model any, Relation any] func(Model) Relation

type BuildResolverFunc[Model any, Relation any] func(context.Context, []Model) (ResolverFunc[Model, Relation], error)

type resolverHolder struct {
	resolver any // holds Resolver[Model, Result]
	err      error
}

type resolverEntry struct {
	once  sync.Once
	ready atomic.Pointer[resolverHolder] // nil until built
}

var errNoLoader = errors.New("model not initialized with loader")

const packagePrefix = "lode"

type ResolveSpec[Model hasState, Result any] struct {
	CacheKey string
	Model    Model
	Build    BuildResolverFunc[Model, Result]
}

func applyResolver[Model any, Result any](h *resolverHolder, cacheKey string, model Model) (Result, error) {
	var zero Result
	if h == nil {
		return zero, fmt.Errorf("%s: internal error: resolver not stored", packagePrefix)
	}
	if h.err != nil {
		return zero, h.err
	}
	fn, ok := h.resolver.(ResolverFunc[Model, Result])
	if !ok {
		return zero, fmt.Errorf("%s: key %q used with incompatible result type", packagePrefix, cacheKey)
	}
	return fn(model), nil
}

func Resolve[Model hasState, Result any](ctx context.Context, spec ResolveSpec[Model, Result]) (Result, error) {
	var emptyResult Result
	if isNil(spec.Model) {
		// question: should we return an error here?
		return emptyResult, nil
	}

	loader := spec.Model.lodeState()
	if loader == nil {
		return emptyResult, errNoLoader
	}

	pmi, _ := loader.resolverEntries.LoadOrStore(spec.CacheKey, &resolverEntry{})
	pm := pmi.(*resolverEntry)

	if h := pm.ready.Load(); h != nil {
		return applyResolver[Model, Result](h, spec.CacheKey, spec.Model)
	}

	pm.once.Do(func() {
		var res any
		var err error
		if models, ok := loader.models.([]Model); !ok {
			err = fmt.Errorf("%s: models is not a slice of %T", packagePrefix, spec.Model)
		} else {
			res, err = spec.Build(ctx, models)
		}
		pm.ready.Store(&resolverHolder{resolver: res, err: err})
	})

	return applyResolver[Model, Result](pm.ready.Load(), spec.CacheKey, spec.Model)

}

type RelationSpec[JoinKey comparable, Model hasState, Relation any] struct {
	CacheKey string
	Model    Model
	// Would you prefer if this returned just a pointer to JoinKey?
	ModelKey func(Model) (key JoinKey, ok bool)
	// Unlike the model key, the relation key should be present (because only
	// joined relations should be fetched!)
	RelationKey func(Relation) JoinKey
	Fetch       func(context.Context, []JoinKey) ([]Relation, error)
}

func isNil[T any](v T) bool {
	rv := reflect.ValueOf(any(v))
	return !rv.IsValid() || (rv.Kind() == reflect.Ptr && rv.IsNil())
}

func Many[JoinKey comparable, Model hasState, Relation any](ctx context.Context, args RelationSpec[JoinKey, Model, Relation]) ([]Relation, error) {
	if isNil(args.Model) {
		return nil, nil
	}
	_, ok := args.ModelKey(args.Model)
	if !ok {
		return nil, nil
	}
	loader := args.Model.lodeState()
	if loader == nil {
		return nil, errNoLoader
	}

	queryFunc := func(ctx context.Context, models []Model) (ResolverFunc[Model, []Relation], error) {
		var modelKeySet = make(map[JoinKey]struct{})
		for _, model := range models {
			if key, ok := args.ModelKey(model); ok {
				modelKeySet[key] = struct{}{}
			}
		}
		var modelKeys = make([]JoinKey, 0, len(modelKeySet))
		for key := range modelKeySet {
			modelKeys = append(modelKeys, key)
		}

		relations, err := args.Fetch(ctx, modelKeys)
		if err != nil {
			return nil, err
		}

		// note that this setup code is not necessary in the gorm case because
		// SetupLoaders has likely already been called by the gorm callback,
		// but I left this here because I think it will be useful in other cases
		loader.engine.InitHandles(relations)

		grouped := make(map[JoinKey][]Relation)
		for _, relation := range relations {
			parentID := args.RelationKey(relation)
			grouped[parentID] = append(grouped[parentID], relation)
		}
		return func(m Model) []Relation {
			if id, ok := args.ModelKey(m); ok {
				return grouped[id]
			}
			return nil
		}, nil
	}
	result, err := Resolve(ctx, ResolveSpec[Model, []Relation]{
		CacheKey: args.CacheKey,
		Model:    args.Model,
		Build:    queryFunc,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func One[JoinKey comparable, Model hasState, Relation any](ctx context.Context, args RelationSpec[JoinKey, Model, Relation]) (Relation, error) {
	var emptyResult Relation
	relations, err := Many(ctx, args)
	if err != nil {
		return emptyResult, err
	}
	if len(relations) == 0 {
		return emptyResult, nil
	}
	return relations[0], nil
}

func FromPtr[T any](t *T) (T, bool) {
	var zero T
	if t == nil {
		return zero, false
	}
	return *t, true
}

type rangeIndex struct {
	StartInclusive int
	EndExclusive   int
}

func batchRanges(n, batchSize int) []rangeIndex {
	if n == 0 {
		return nil
	}
	nb := (n + batchSize - 1) / batchSize
	out := make([]rangeIndex, 0, nb)
	for i := 0; i < n; i += batchSize {
		j := i + batchSize
		if j > n {
			j = n
		}
		out = append(out, rangeIndex{StartInclusive: i, EndExclusive: j})
	}
	return out
}

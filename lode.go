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

type hasState interface {
	lodeState() *loaderState
	setLodeState(*loaderState)
}

var _ hasState = (*Handle)(nil)

type loaderState struct {
	sync.Mutex
	models          any
	engine          *Engine
	resolverEntries sync.Map
}

// InitHandles initializes the loader state for a slice of models.
// ChatGPT prefers the name "Bind".  What do you think?
func (e *Engine) InitHandles(models any) {
	v := reflect.ValueOf(models)
	if !v.IsValid() {
		return
	}
	// deref pointers to get to the slice
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice || v.Len() == 0 {
		return
	}

	elemType := v.Type().Elem()
	ptrElems := elemType.Kind() == reflect.Ptr

	// Do we need to (re)bind this batch?
	var (
		first *loaderState
		need  bool
	)
	for i := 0; i < v.Len(); i++ {
		el := v.Index(i)

		var x any
		if ptrElems {
			if el.IsNil() {
				continue
			}
			x = el.Interface() // *T
		} else {
			if !el.CanAddr() {
				continue
			}
			x = el.Addr().Interface() // &T
		}

		hl, ok := x.(hasState)
		if !ok {
			continue
		}
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
			need = true // mixed state: normalize to one
			break
		}
	}
	if !need {
		return
	}

	// Bind in batches; store models as a slice of *T in all cases.
	for _, br := range batchRanges(v.Len(), e.config.batchSize) {
		sub := v.Slice(br.StartInclusive, br.EndExclusive)
		state := &loaderState{
			models: nil, // set below
			engine: e,
		}

		if ptrElems {
			// models already []*T
			state.models = sub.Interface()
			for i := 0; i < sub.Len(); i++ {
				el := sub.Index(i)
				if el.IsNil() {
					continue
				}
				if hl, ok := el.Interface().(hasState); ok {
					hl.setLodeState(state)
				}
			}
			continue
		}

		// values []T → build []*T pointing at the same elements
		ptrType := reflect.PointerTo(elemType)   // *T
		ptrSliceType := reflect.SliceOf(ptrType) // []*T
		ptrSlice := reflect.MakeSlice(ptrSliceType, sub.Len(), sub.Len())

		for i := 0; i < sub.Len(); i++ {
			el := sub.Index(i)
			if !el.CanAddr() {
				continue
			}
			addr := el.Addr() // *T
			ptrSlice.Index(i).Set(addr)

			if hl, ok := addr.Interface().(hasState); ok {
				hl.setLodeState(state)
			}
		}
		state.models = ptrSlice.Interface() // always []*T for Resolve
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

func Many[JoinKey comparable, Model hasState, Relation any](ctx context.Context, args RelationSpec[JoinKey, Model, Relation]) ([]Relation, error) {
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

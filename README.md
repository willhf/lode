# Lode

**Eureka — you’ve hit gold!**

Lode is a tiny Go library for **loading related data** in batches. It makes
querying relations explicit, ergonomic, and fast — eliminating N+1 queries
without hiding what’s going on.

- **No Magic.** No codegen or schema reflection — you define relations in Go.
- **Ergonomic, No N+1.** Query once per batch while still calling per-model
  methods.
- **ORM-agnostic core.** Zero dependencies; you control the SQL/ORM in `Fetch`.
- **GORM helpers.** Optional [lodegorm](https://pkg.go.dev/github.com/willhf/lode/lodegorm) module integrates seamlessly with GORM.

## Install

```bash
go get github.com/willhf/lode@latest
```

Optional GORM adapter:

```bash
go get github.com/willhf/lode/lodegorm@latest
```

## Example

See [example/example.go](./example/example.go) for a full working sample.

## Quickstart

### 1. Embed a Handle

Embed a [Handle](https://pkg.go.dev/github.com/willhf/lode#Handle) in your models.
The handle must be initialized before use (step 3).

```go
type Author struct {
	ID   uint
	Name string

	lode.Handle
}
```

### 2. Define relation methods

Add methods to your models that describe how to fetch related data.

```go
func (a *Author) Books(ctx context.Context, db *gorm.DB) ([]*Book, error) {
	return lode.Many(ctx, lode.RelationSpec[uint, *Author, *Book]{
		CacheKey:    "books",
		Model:       a,
		ModelKey:    func(a *Author) (uint, bool) { return a.ID, true },
		RelationKey: func(b *Book) uint { return *b.AuthorID },
		Fetch: func(ctx context.Context, ids []uint) ([]*Book, error) {
			var books []*Book
			err := db.WithContext(ctx).Where("author_id IN ?", ids).Find(&books).Error
			return books, err
		},
	})
}
```

If you are using GORM, you can avoid writing a function for `Fetch` by using
[lodegorm.Fetch](https://pkg.go.dev/github.com/willhf/lode/lodegorm#Fetch) instead. You can also supply your own `Fetch` to add logging, metrics,
or custom queries.

### 3. Initialize handles

After creating or fetching models, call
[Engine.InitHandles](https://pkg.go.dev/github.com/willhf/lode#Engine.InitHandles)
to prepare them for use.

```go
engine := lode.NewEngine()

var authors []*Author = getAuthors()

engine.InitHandles(authors)
```

- [Engine](https://pkg.go.dev/github.com/willhf/lode#Engine) exists to avoid
configuring lode behavior through global variables.  You are encouraged to
 instantiate a single instance and reuse it across your project.
- It is safe to call `InitHandles` on a single model.  This is necessary if you
  have fetched a single model.  If you have fetched a slice of models though,
  you should call `InitHandles` on the slice of models because that allows
  lode to batch queries.
- Avoid sprinkling `InitHandles` calls everywhere — set up hooks to do it
  automatically.
- With GORM, use [lodegorm.RegisterCallback](https://pkg.go.dev/github.com/willhf/lode/lodegorm#RegisterCallback) to initialize handles after queries:

```go
import (
	"github.com/willhf/lode"
	"github.com/willhf/lode/lodegorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	engine := lode.NewEngine()
	lodegorm.RegisterCallback(engine, db)

	// ... rest of your program
}
```

### 4. Query without N+1

Use your relation methods just like normal methods — but under the hood, queries
are batched.

```go
var authors Authors // works with []Author, []*Author, or a named slice type
if err := db.Find(&authors).Error; err != nil {
	log.Fatal(err)
}

for _, author := range authors {
	fmt.Println(author.Name)

	// One query for *all* authors’ books. N+1 avoided.
	books, err := author.Books(ctx, db)
	if err != nil {
		log.Fatal(err)
	}

	for _, book := range books {
		fmt.Println("  ", book.Title)
	}
}
```

## Motivation

* I'm not a fan of GORM’s [preloading](https://gorm.io/docs/preload.html)
  functionality. functionality. When a model containing preload fields is passed
  as a parameter to a function, it’s unclear whether those fields have actually
  been loaded — their presence isn’t statically checked. To verify that they are,
  you must inspect all function call sites to ensure the corresponding GORM
  queries include the correct preloads.

* Additionally, if a method or function for a model requires related data and that data
  must be passed in as parameters, encapsulation is lost. Lazily loading relations as needed
  allows the caller to remain unaware of such details. However, the naive approach of
  querying relations on demand within helper functions often leads to N+1 queries.
  The strength of this library is that it avoids N+1 queries while still allowing
  per-model methods and functions to be called naturally.

* While eagerly loading all required relations (rather than doing so lazily)
  can enable query optimizations — since multiple relations can be fetched together —
  I believe this advantage is outweighed by the loss of encapsulation and
  the added complexity of managing “populated” models.

* The [ent](https://entgo.io/) library is excellent — its static checking eliminates the common GORM pitfall of discovering invalid queries only at runtime. I’ve used it with delight in the past. Nonetheless, I believe it has three main disadvantages:
  - Code generation adds an extra step that slows down builds, and the large volume of generated code can degrade editor and tooling performance.
  - Real-world projects often require raw SQL, and having to choose between fully static queries and untyped raw SQL can be frustrating.
  - Most importantly, Ent’s eager loading is less ergonomic than lazy loading, as it reintroduces the “is my relation loaded?” problem when using helper functions and methods — the same issue discussed in the preload section above.
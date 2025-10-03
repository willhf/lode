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

**Tip:** With GORM you can replace `Fetch` with
[lodegorm.Fetch](https://pkg.go.dev/github.com/willhf/lode/lodegorm#Fetch) to
avoid boilerplate. You can also supply your own `Fetch` to add logging, metrics,
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

**Best practices:**
- Reuse a single [Engine](https://pkg.go.dev/github.com/willhf/lode#Engine) across your project.
- Always call [Engine.InitHandles](https://pkg.go.dev/github.com/willhf/lode#Engine.InitHandles) on slices of models that embed [Handle](https://pkg.go.dev/github.com/willhf/lode#Handle).
- It is safe to call [Engine.InitHandles](https://pkg.go.dev/github.com/willhf/lode#Engine.InitHandles) on a single model.
- Avoid sprinkling [Engine.InitHandles](https://pkg.go.dev/github.com/willhf/lode#Engine.InitHandles) calls everywhere — set up hooks to do it
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


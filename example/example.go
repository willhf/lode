package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"strings"

	"github.com/willhf/lode"
	"github.com/willhf/lode/lodegorm"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Author struct {
	ID   uint
	Name string
	// First embed lode.Handle into your models
	lode.Handle
}

type Book struct {
	ID       uint
	AuthorID *uint
	Title    string
	lode.Handle
}

type Chapter struct {
	ID     uint
	BookID uint
	Title  string
	lode.Handle
}

type Authors []*Author

type Books []*Book

type Chapters []*Chapter

// Next, use lode.Many and lode.One to define the relations between your models.
func (author *Author) Books(ctx context.Context, db *gorm.DB) (Books, error) {
	return lode.Many(ctx, lode.RelationSpec[uint, *Author, *Book]{
		CacheKey:    "books",
		Model:       author,
		ModelKey:    func(author *Author) (uint, bool) { return author.ID, true },
		RelationKey: func(book *Book) uint { return *book.AuthorID },
		Fetch:       lodegorm.Fetch[*Book, uint](db, "author_id"),
	})
}

func (book *Book) Chapters(ctx context.Context, db *gorm.DB) (Chapters, error) {
	return lode.Many(ctx, lode.RelationSpec[uint, *Book, *Chapter]{
		CacheKey:    "chapters",
		Model:       book,
		ModelKey:    func(book *Book) (uint, bool) { return book.ID, true },
		RelationKey: func(chapter *Chapter) uint { return chapter.BookID },
		Fetch:       lodegorm.Fetch[*Chapter, uint](db, "book_id"),
	})
}

func (book *Book) Author(ctx context.Context, db *gorm.DB) (*Author, error) {
	return lode.One(ctx, lode.RelationSpec[uint, *Book, *Author]{
		CacheKey:    "author",
		Model:       book,
		ModelKey:    func(book *Book) (uint, bool) { return lode.FromPtr(book.AuthorID) },
		RelationKey: func(author *Author) uint { return author.ID },
		Fetch:       lodegorm.Fetch[*Author, uint](db, "id"),
	})
}

// You can create other methods that use the lode methods!  This is nice because
// it facilitates encapsulation - the caller can be ignorant of the relations
// required to compute the outcome.
func (author *Author) NumChapters(ctx context.Context, db *gorm.DB) (int, error) {
	books, err := author.Books(ctx, db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, book := range books {
		chapters, err := book.Chapters(ctx, db)
		if err != nil {
			return 0, err
		}
		total += len(chapters)
	}
	return total, nil
}

// Sometimes the lode.One and lode.Many methods are not what is needed.  You can
// use Resolve to build a custom query.  Here, we build a query that counts the
// number of chapters for each author directly without loading the books and
// chapters.
func (author *Author) NumChaptersUsingQuery(ctx context.Context, db *gorm.DB) (int, error) {
	return lode.Resolve(ctx, lode.ResolveSpec[*Author, int]{
		CacheKey: "num_chapters",
		Model:    author,
		Build: func(ctx context.Context, models []*Author) (lode.ResolverFunc[*Author, int], error) {
			var authorIDs []uint
			for _, model := range models {
				authorIDs = append(authorIDs, model.ID)
			}
			var rows []struct {
				ChapterCount int
				AuthorID     uint
			}
			err := db.WithContext(ctx).
				Model(&Chapter{}).
				Joins("JOIN books ON books.id = chapters.book_id").
				Where("books.author_id IN ?", authorIDs).
				Group("books.author_id").
				Select("COUNT(*) as chapter_count, books.author_id").Find(&rows).Error
			if err != nil {
				return nil, err
			}
			grouped := make(map[uint]int)
			for _, row := range rows {
				grouped[row.AuthorID] = row.ChapterCount
			}
			return func(model *Author) int { return grouped[model.ID] }, nil
		},
	})
}

var (
	//go:embed 001_schema.sql
	schema string
	//go:embed 002_seed.sql
	seed string
)

func main() {
	ctx := context.Background()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	for _, str := range []string{schema, seed} {
		for _, stmt := range strings.Split(str, ";") {
			if err := db.Exec(stmt).Error; err != nil {
				log.Fatal(err)
			}
		}
	}

	// Register the gorm callback if you want all models to be automatically
	// initialized with the lode engine (which you probably do!).  That way,
	// you can immediately call the lode methods on the models.
	engine := lode.NewEngine()
	lodegorm.RegisterCallback(engine, db)

	var authors Authors // this will also work with []Author or []*Author
	if err := db.Find(&authors).Error; err != nil {
		log.Fatal(err)
	}

	for _, author := range authors {
		fmt.Println(author.Name)

		// Even though this looks like a N+1 query within a for loop, only a
		// single query is made for all books!
		books, err := author.Books(ctx, db)
		if err != nil {
			log.Fatal(err)
		}

		for _, book := range books {
			fmt.Println("  ", book.Title)

			// Note that this Chapters call will do a single database query for
			// all chapters across not only all books but all authors as well!
			chapters, err := book.Chapters(ctx, db)
			if err != nil {
				log.Fatal(err)
			}

			for _, chapter := range chapters {
				fmt.Println("    ", chapter.Title)
			}
		}
	}
}

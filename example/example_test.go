package main

import (
	"context"
	"strings"
	"testing"

	"github.com/willhf/lode"
	"github.com/willhf/lode/lodegorm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// these tests exist here rather than in lodegorm because they rely on sqlite
// and we don't want to add sqlite as a dependency to lodegorm

func seededSetup(t *testing.T) (*gorm.DB, *lode.Engine) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open error: %v", err)
	}
	engine := lode.NewEngine()
	lodegorm.RegisterCallback(engine, db)
	for _, str := range []string{schema, seed} {
		for _, stmt := range strings.Split(str, ";") {
			if err := db.Exec(stmt).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return db, engine
}

const knownAuthorName = "Alice Pennington"

func findInSlice[T any](slice []T, predicate func(T) bool) T {
	for _, v := range slice {
		if predicate(v) {
			return v
		}
	}
	var zero T
	return zero
}

func TestFirst(t *testing.T) {
	ctx := context.Background()
	db, _ := seededSetup(t)

	var author Author
	if err := db.Where("name = ?", knownAuthorName).First(&author).Error; err != nil {
		t.Fatal(err)
	}

	books, err := author.Books(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatal(len(books))
	}
}

func TestFind_AuthorPointer(t *testing.T) {
	ctx := context.Background()
	db, _ := seededSetup(t)

	var authors []*Author
	if err := db.Find(&authors).Error; err != nil {
		t.Fatal(err)
	}

	if len(authors) != 5 {
		t.Fatal(len(authors))
	}

	var knownAuthor = findInSlice(authors, func(a *Author) bool { return a.Name == knownAuthorName })
	if knownAuthor == nil {
		t.Fatal("knownAuthor not found")
	}

	books, err := knownAuthor.Books(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatal(len(books))
	}
}

func TestFind_AuthorStruct(t *testing.T) {
	ctx := context.Background()
	db, _ := seededSetup(t)

	var authors []Author
	if err := db.Find(&authors).Error; err != nil {
		t.Fatal(err)
	}

	if len(authors) != 5 {
		t.Fatal(len(authors))
	}

	var knownAuthor = findInSlice(authors, func(a Author) bool { return a.Name == knownAuthorName })
	if knownAuthor.Name != knownAuthorName {
		t.Fatal("knownAuthor not found")
	}

	books, err := knownAuthor.Books(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatal(len(books))
	}
}

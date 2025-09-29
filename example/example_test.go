package main

import (
	"context"
	"log"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func BenchmarkLodegorm(b *testing.B) {
	b.ReportAllocs()
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

	b.ResetTimer()

	var authors []*Author
	if err := db.Find(&authors).Error; err != nil {
		b.Fatalf("Failed to find authors: %v", err)
	}

	authors[0].Books(ctx, db)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, author := range authors {
			author.Books(ctx, db)
		}
	}
}

package lodegorm

import (
	"context"

	"github.com/willhf/lode"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func RegisterCallback(engine *lode.Engine, db *gorm.DB) {
	const cbName = "lodegorm:init"
	db.Callback().Query().After("gorm:query").Register(cbName, func(tx *gorm.DB) {
		engine.InitHandles(tx.Statement.Dest)
	})
}

// Fetch is a helper function to fetch models by their keys.
func Fetch[Model any, Key any](db *gorm.DB, joinColumn string) func(context.Context, []Key) ([]Model, error) {
	return func(ctx context.Context, ids []Key) ([]Model, error) {
		idInterfaceSlice := make([]interface{}, len(ids))
		for i, id := range ids {
			idInterfaceSlice[i] = id
		}
		var models []Model
		err := db.WithContext(ctx).
			Where(clause.IN{Column: clause.Column{Name: joinColumn}, Values: idInterfaceSlice}).
			Find(&models).Error
		return models, err
	}
}

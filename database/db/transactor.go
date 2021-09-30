package db

import (
	"database/sql"

	"gorm.io/gorm"
)

type TransactionFunc = func(db *gorm.DB) error

func WithTransaction(db *gorm.DB, p TransactionFunc, opts ...*sql.TxOptions) error {
	tx := db.Begin(opts...)
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := p(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

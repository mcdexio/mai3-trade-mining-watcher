package db

import (
	"database/sql"

	"gorm.io/gorm"
)

type TransactionFunc = func(db *gorm.DB) error

type TransactorOption interface {
	Set(opt *sql.TxOptions)
}

type OptionReadOnly struct {
}

func (o OptionReadOnly) Set(opt *sql.TxOptions) {
	opt.ReadOnly = true
}

type OptionRepeatableRead struct{}

func (o OptionRepeatableRead) Set(opt *sql.TxOptions) {
	opt.Isolation = sql.LevelRepeatableRead
}

func WithTransaction(db *gorm.DB, p TransactionFunc, opts ...*sql.TxOptions) error {
	tx := db.Begin(opts...)
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := p(tx); err != nil {
		return tx.Rollback().Error
	}
	return tx.Commit().Error
}

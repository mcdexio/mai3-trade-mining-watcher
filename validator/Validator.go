package validator

import (
	"context"

	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type Validator struct {
	config *Config

	master *gorm.DB
	backup *gorm.DB
}

func NewValidator(config *Config) (*Validator, error) {
	conf := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger: logger.Default.LogMode(logger.Silent), // silent orm logs
	}
	mdb, err := gorm.Open(postgres.Open(config.MainDB), conf)
	if err != nil {
		return nil, err
	}
	bdb, err := gorm.Open(postgres.Open(config.BackupDB), conf)
	if err != nil {
		return nil, err
	}
	return &Validator{
		config: config,
		master: mdb,
		backup: bdb,
	}, nil
}

func (v *Validator) Run(ctx context.Context) error {
	return nil
}

func (v *Validator) aggregate(db *gorm.DB, timstamp int64) (decimal.Decimal, error) {
	var users []*mining.Snapshot
	if err := db.Where("timestamp=?", timstamp).Find(&users).Error; err != nil {
		return , err
	}
}

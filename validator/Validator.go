package validator

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type Validator struct {
	config      *Config
	replicas    []*gorm.DB
	lastChecked int64
	logger      logging.Logger
}

func NewValidator(config *Config, logger logging.Logger) (*Validator, error) {
	conf := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger: glogger.Default.LogMode(glogger.Silent), // silent orm logs
	}
	rpls := make([]*gorm.DB, len(config.DatabaseURLs))
	for i, u := range config.DatabaseURLs {
		db, err := gorm.Open(postgres.Open(u), conf)
		if err != nil {
			return nil, err
		}
		rpls[i] = db
	}
	return &Validator{
		config:   config,
		replicas: rpls,
		logger:   logger,
	}, nil
}

func (v *Validator) Run(ctx context.Context) error {
	for {
		hourly := time.Now().Unix()/3600*3600 - 3600
		v.logger.Info("going to check snapshot at %v", hourly)
		if hourly > v.lastChecked {
			if err := v.run(ctx, hourly); err != nil {
				v.logger.Warn("error occurs while check snapshots: timestamp=%v, error=%v", formatTime(hourly), err)
			} else {
				v.lastChecked = hourly
			}
		}
		time.Sleep(1 * time.Minute)
	}
}

func (v *Validator) run(ctx context.Context, timestamp int64) error {
	if len(v.replicas) == 0 {
		return errors.New("no replicas")
	}
	ok, err := v.ensureProgress(ctx, timestamp)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("not all replica are ready for snapshot check %w", err)
	}
	ok, err = v.compareDigest(ctx, timestamp)
	if err != nil {
		return err
	}
	if ok {
		return v.OnOK(ctx, timestamp)
	}
	return v.OnConflict(ctx, timestamp)
}

func (v *Validator) OnOK(ctx context.Context, timestamp int64) error {
	v.logger.Info("all snapshots verified. timestamp=%v", timestamp)
	return nil
}

func (v *Validator) OnConflict(ctx context.Context, timestamp int64) error {
	v.logger.Warn("found conflict snapshot score checksum. timestamp=%v", timestamp)
	return nil
}

func (v *Validator) ensureProgress(ctx context.Context, timestamp int64) (bool, error) {
	if len(v.replicas) == 0 {
		return false, errors.New("no replicas")
	}
	for i, db := range v.replicas {
		p, err := v.getLastProgress(db)
		if err != nil {
			return false, fmt.Errorf("progress not ready: replica_index=%v %w", i, err)
		}
		if p < timestamp {
			return false, fmt.Errorf("replica not ready: replica=%v, progress=%v %w", i, timestamp, err)
		}
	}
	return true, nil
}

func (v *Validator) getLastProgress(db *gorm.DB) (int64, error) {
	var p mining.Progress
	err := db.Where("table_name=snapshot").Order("epoch desc").First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("fail to get last progress: table=user_info %w", err)
	}
	return p.From, nil
}

func (v *Validator) compareDigest(ctx context.Context, timestamp int64) (bool, error) {
	var expect []byte
	for _, db := range v.replicas {
		d, err := v.calcScoreDigest(db, timestamp)
		if err != nil {
			return false, err
		}
		if len(expect) == 0 {
			expect = d
			continue
		}
		if !bytes.Equal(d, expect) {
			return false, nil
		}
	}
	return true, nil
}

func (v *Validator) calcScoreDigest(db *gorm.DB, timstamp int64) ([]byte, error) {
	var users []*mining.Snapshot
	if err := db.Where("timestamp=?", timstamp).Order("trader asc").Find(&users).Error; err != nil {
		return nil, err
	}
	h := md5.New()
	for _, u := range users {
		if _, err := h.Write([]byte(u.Trader)); err != nil {
			return nil, err
		}
		if _, err := h.Write([]byte(u.Score.String())); err != nil {
			return nil, err
		}
	}
	return h.Sum(nil), nil
}

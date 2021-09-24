package trading_mining

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"math"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
)

type Calculator struct {
	logger                    logging.Logger
	ctx                       context.Context
	db                        *gorm.DB
	interval                  time.Duration
	intervalDecimal           decimal.Decimal
	lastCheckpoint            *time.Time
	lastTimestamp             *time.Time
	startTime                 *time.Time
	thisEpochStartTime        *time.Time
	thisEpochStartTimeDecimal decimal.Decimal
	initFee                   map[string]decimal.Decimal
	epoch                     int64 // index from 0
}

func NewCalculator(ctx context.Context, logger logging.Logger, intervalSec int, startTime *time.Time) (*Calculator, error) {
	cal := &Calculator{
		ctx:                ctx,
		logger:             logger,
		db:                 database.GetDB(),
		interval:           time.Duration(intervalSec),
		startTime:          startTime,
		thisEpochStartTime: startTime,
		lastTimestamp:      startTime,
		initFee:            make(map[string]decimal.Decimal),
	}
	cal.intervalDecimal = decimal.NewFromFloat((cal.interval * time.Second).Minutes())
	cal.thisEpochStartTimeDecimal = decimal.NewFromInt(cal.thisEpochStartTime.Unix())
	return cal, nil
}

func (c *Calculator) updateEpoch(now time.Time) {
	c.epoch = (now.Unix() - c.startTime.Unix()) / 60 / 60 / 24 / 14
}

func (c *Calculator) updateInitFee() {
	// reset
	c.initFee = make(map[string]decimal.Decimal)
	if c.epoch <= 0 {
		// there is no previous epoch
		return
	}

	var countTraders []struct {
		Trader string
	}
	// 1. get count traders first
	var count int
	err := c.db.Model(&mining.UserInfo{}).Select("DISTINCT trader").Where("epoch = ?", c.epoch-1).Scan(&countTraders).Error
	if err != nil {
		c.logger.Error("failed to get distinct count trader %s", err)
	} else {
		count = len(countTraders)
		c.logger.Info("there are %d distinct trader on previous epoch", count)
	}
	if count == 0 {
		// there are no trader on previous epoch
		return
	}

	// 2. get last fee of previous epoch
	var Fees []struct {
		Trader string
		Fee    decimal.Decimal
	}
	err = c.db.Model(&mining.UserInfo{}).Limit(count).Order("timestamp desc").Select("trader, fee").Where("epoch = ?", c.epoch-1).Scan(&Fees).Error
	for _, f := range Fees {
		c.initFee[f.Trader] = f.Fee
	}
}

func (c *Calculator) Run() error {
	c.updateEpoch(time.Now())
	c.updateInitFee()
	for {
		select {
		case <-c.ctx.Done():
			return nil
		case <-time.After(336 * time.Hour): // 14 days
			c.endThisEpoch(time.Now())
		case <-time.After(60 * time.Minute): // 1 hour TODO(champFu): checkpoint
			c.checkpoint(time.Now())
		case <-time.After(c.interval * time.Second):
			c.calculate(time.Now())
		}
	}
}

func (c *Calculator) checkpoint(now time.Time) {
	if c.startTime.After(now) {
		// not yet
		return
	}
	c.logger.Info("Checkpoint trading mining...")
	c.lastCheckpoint = &now
}

func (c *Calculator) endThisEpoch(now time.Time) {
	if c.startTime.After(now) {
		// not yet
		return
	}
	c.logger.Info("Ending this epoch...")
	// main logic TODO(champFu): endThisEpoch

	// setup
	c.updateEpoch(now)
	c.updateInitFee()
	c.thisEpochStartTime = &now
	c.thisEpochStartTimeDecimal = decimal.NewFromInt(c.thisEpochStartTime.Unix())
}

func (c *Calculator) calculate(now time.Time) {
	if c.startTime.After(now) {
		// not yet
		return
	}
	nowDecimal := decimal.NewFromInt(now.Unix())
	lastTimestampDecimal := decimal.NewFromInt(c.lastTimestamp.Unix())
	minuteDecimal := decimal.NewFromInt(60)

	c.logger.Info("Calculation trading mining...")
	fromThisEpochStartTimeToNow := (nowDecimal.Add(c.thisEpochStartTimeDecimal.Neg())).Div(minuteDecimal)
	c.logger.Debug("fromThisEpochStartTimeToNow %s", fromThisEpochStartTimeToNow.String())
	if fromThisEpochStartTimeToNow.LessThanOrEqual(decimal.Zero) {
		c.logger.Error("this epoch start time decimal %s", c.thisEpochStartTimeDecimal.String())
		c.logger.Error("now %d", now.Unix())
		return
	}
	fromThisEpochStartTimeToLast := (lastTimestampDecimal.Add(c.thisEpochStartTimeDecimal.Neg())).Div(minuteDecimal)
	c.logger.Debug("fromThisEpochStartTimeToLast %s", fromThisEpochStartTimeToLast.String())
	if fromThisEpochStartTimeToLast.LessThanOrEqual(decimal.Zero) {
		c.logger.Warn("it will happen when first time doing calculation")
		c.logger.Warn("this epoch start time decimal %s", c.thisEpochStartTimeDecimal.String())
		c.logger.Warn("last %d", c.lastTimestamp.Unix())
	}

	var countTraders []struct {
		Trader string
	}
	var feeResults []struct {
		Trader    string
		Fee       decimal.Decimal
		Timestamp int64
	}
	var stakeResults []struct {
		Trader string
		Stake  decimal.Decimal
	}
	var positionResults []struct {
		Trader     string
		EntryValue decimal.Decimal
	}
	var userInfoResults []struct {
		Trader    string
		Fee       decimal.Decimal
		Stake     decimal.Decimal
		OI        decimal.Decimal
		Timestamp int64
	}

	err := c.db.Model(&mining.Fee{}).Select("DISTINCT trader").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&countTraders).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}
	userCount := len(countTraders)

	// only get the latest one for all user {userCount}
	err = c.db.Model(&mining.Fee{}).Limit(userCount).Order("timestamp desc").Select("trader, fee, timestamp").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}

	for _, r := range feeResults {
		trader := r.Trader
		fee := r.Fee
		timestamp := r.Timestamp
		var stake decimal.Decimal
		var entryValue decimal.Decimal
		err = c.db.Model(&mining.Stake{}).Limit(1).Select("trader, AVG(stake) as stake").Where("trader = ? and timestamp > ?", trader, c.lastTimestamp.Unix()).Group("trader").Scan(&stakeResults).Error
		if err != nil {
			c.logger.Error("failed to get stake %s", err)
			return
		}
		if len(stakeResults) == 1 {
			stake = stakeResults[0].Stake
		} else if len(stakeResults) == 0 {
			// means this user don't have stake now.
			stake = decimal.Zero
		}

		err = c.db.Model(&mining.Position{}).Limit(1).Select("trader, AVG(entry_value) as entry_value").Where("trader = ? and timestamp > ?", trader, c.lastTimestamp.Unix()).Group("trader").Scan(&positionResults).Error
		if err != nil {
			c.logger.Error("failed to get position %s", err)
			return
		}
		if len(positionResults) == 1 {
			entryValue = positionResults[0].EntryValue
		} else if len(positionResults) == 0 {
			// means this user don't have position now.
			entryValue = decimal.Zero
		}

		// the fee is now_fee - start_fee.
		if iFee, match := c.initFee[r.Trader]; match {
			fee = fee.Add(iFee.Neg())
			if fee.LessThan(decimal.Zero) { // can be zero
				c.logger.Error("feeResult %+v", r)
				c.logger.Error("fee %s", fee.String())
				c.logger.Error("init fee %s", iFee.Neg().String())
				return
			}
		}
		// the stake is (stake * interval) + (pre_stake * (lastTimeStamp - start_time)) / now - start_time
		// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
		thisEntryValue := entryValue.Mul(c.intervalDecimal)
		thisStakeValue := stake.Mul(c.intervalDecimal)
		if thisStakeValue.LessThan(decimal.Zero) {
			c.logger.Error("thisStakeValue is less than zero")
			c.logger.Error("value %s", stake.String())
			return
		}
		if thisEntryValue.LessThan(decimal.Zero) {
			c.logger.Error("thisEntryValue is less than zero")
			c.logger.Error("value %s", entryValue.String())
			return
		}

		// get this epoch but latest info
		err = c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee, stake, oi").Where("trader = ? and epoch = ?", trader, c.epoch).Scan(&userInfoResults).Error
		if err != nil {
			c.logger.Error("failed to get user info %s", err)
			return
		}
		var finalOI decimal.Decimal
		var finalStake decimal.Decimal
		if len(userInfoResults) == 0 {
			// there is no previous info, means stake without pre_stake, oi without pre_oi
			finalOI = thisEntryValue.Div(fromThisEpochStartTimeToNow)
			finalStake = thisStakeValue.Div(fromThisEpochStartTimeToNow)
		} else {
			// the stake is (stake * interval) + (pre_stake * (lastTimeStamp - start_time)) / now - start_time
			// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
			pre := userInfoResults[0]
			preEntryValue := pre.OI.Mul(fromThisEpochStartTimeToLast)
			if preEntryValue.LessThan(decimal.Zero) {
				c.logger.Error("preEntry is less than zero")
				c.logger.Error("value %s", pre.OI.String())
				return
			}
			preStake := pre.Stake.Mul(fromThisEpochStartTimeToLast)
			if preStake.LessThan(decimal.Zero) {
				c.logger.Error("preStake is less than zero")
				c.logger.Error("value %s", pre.Stake.String())
				return
			}
			finalOI = (thisEntryValue.Add(preEntryValue)).Div(fromThisEpochStartTimeToNow)
			finalStake = (thisStakeValue.Add(preStake)).Div(fromThisEpochStartTimeToNow)
		}
		score := c.calScore(fee, finalOI, finalStake)
		c.db.Create(&mining.UserInfo{
			Trader:    trader,
			Fee:       fee,
			OI:        finalOI,
			Stake:     finalStake,
			Score:     score,
			Timestamp: timestamp,
			Epoch:     c.epoch,
		})
	}
	c.lastTimestamp = &now
}

func (c *Calculator) calScore(fee, oi, stake decimal.Decimal) decimal.Decimal {
	feeInflate, _ := fee.Float64()
	oiInflate, _ := oi.Float64()
	stakeInflate, _ := stake.Float64()
	score := math.Pow(feeInflate, 0.7) + math.Pow(oiInflate, 0.3) + math.Pow(stakeInflate, 0.3)
	return decimal.NewFromFloat(score)
}

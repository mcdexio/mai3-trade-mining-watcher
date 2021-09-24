package trading_mining

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	database "github.com/mcdexio/mai3-trade-mining-watcher/database/db"
)

type Calculator struct {
	logger           logging.Logger
	ctx              context.Context
	db               *gorm.DB
	interval         time.Duration
	intervalDecimal  decimal.Decimal
	lastCheckpoint   *time.Time
	lastTimestamp    *time.Time
	startTime        *time.Time
	startTimeDecimal decimal.Decimal
	initFee          map[string]decimal.Decimal
}

func NewCalculator(ctx context.Context, logger logging.Logger, intervalSec int, startTime *time.Time) (*Calculator, error) {
	cal := &Calculator{
		ctx:           ctx,
		logger:        logger,
		db:            database.GetDB(),
		interval:      time.Duration(intervalSec),
		startTime:     startTime,
		lastTimestamp: startTime,
		initFee:       make(map[string]decimal.Decimal),
	}
	cal.intervalDecimal = decimal.NewFromFloat((cal.interval * time.Second).Minutes())
	cal.startTimeDecimal = decimal.NewFromInt(cal.startTime.Unix())

	var iniFee []struct {
		Trader string
		Fee    decimal.Decimal
	}

	// get the init fee
	err := cal.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee").Where("timestamp < ?", startTime.Unix()).Scan(&iniFee).Error
	if err != nil {
		logger.Error("failed to get user info %s", err)
	}
	for _, fee := range iniFee {
		cal.initFee[fee.Trader] = fee.Fee
	}
	return cal, nil
}

func (c *Calculator) Run() error {
	for {
		select {
		case <-c.ctx.Done():
			return nil
		case <-time.After(c.interval * time.Second):
			c.calculate()
		case <-time.After(60 * time.Minute): // 1 hour TODO(champFu): checkpoint
			c.checkpoint()
		case <-time.After(350 * time.Hour): // 14 days
			c.endThisEpoch()
		}
	}
}

func (c *Calculator) checkpoint() {
	now := time.Now()
	if c.startTime.After(now) {
		// not yet
		return
	}
	c.logger.Info("Checkpoint trading mining...")
	c.lastCheckpoint = &now
}

func (c *Calculator) endThisEpoch() {
	now := time.Now()
	if c.startTime.After(now) {
		// not yet
		return
	}
	c.logger.Info("Endding this epoch...")
	// main logic TODO(champFu): endThisEpoch

	// setup
	c.startTime = &now
	c.startTimeDecimal = decimal.NewFromInt(c.startTime.Unix())

	// get the init fee
	var iniFee []struct {
		Trader string
		Fee    decimal.Decimal
	}
	err := c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee").Where("timestamp < ?", c.startTime.Unix()).Scan(&iniFee).Error
	if err != nil {
		c.logger.Error("failed to get user info %s", err)
	}
	for _, fee := range iniFee {
		c.initFee[fee.Trader] = fee.Fee
	}
}

func (c *Calculator) calculate() {
	now := time.Now()
	nowDecimal := decimal.NewFromInt(now.Unix())
	lastTimestampDecimal := decimal.NewFromInt(c.lastTimestamp.Unix())
	if c.startTime.After(now) {
		// not yet
		return
	}

	minuteDecimal := decimal.NewFromInt(60)

	c.logger.Info("Calculation trading mining...")
	fromStartTimeToNow := (nowDecimal.Add(c.startTimeDecimal.Neg())).Div(minuteDecimal)
	c.logger.Debug("fromStartTimeToNow %s", fromStartTimeToNow.String())
	if fromStartTimeToNow.LessThanOrEqual(decimal.Zero) {
		c.logger.Error("start time decimal %s", c.startTimeDecimal.String())
		c.logger.Error("now %d", now.Unix())
		return
	}
	fromStartTimeToLast := (lastTimestampDecimal.Add(c.startTimeDecimal.Neg())).Div(minuteDecimal)
	c.logger.Debug("fromStartTimeToLast %s", fromStartTimeToLast.String())
	if fromStartTimeToLast.LessThanOrEqual(decimal.Zero) {
		c.logger.Warn("it will happen when first time doing calculation")
		c.logger.Warn("start time decimal %s", c.startTimeDecimal.String())
		c.logger.Warn("last %d", c.lastTimestamp.Unix())
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

	err := c.db.Model(&mining.Fee{}).Select("DISTINCT trader").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}
	userCount := len(feeResults)

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
			entryValue = decimal.Zero
			// means this user don't have position now.
		}

		// the fee is now_fee - start_fee.
		// the stake is (stake * interval) + (pre_stake * (lastTimeStamp - start_time)) / now - start_time
		// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
		if iFee, match := c.initFee[r.Trader]; match {
			fee = fee.Add(iFee.Neg())
			if fee.LessThanOrEqual(decimal.Zero) {
				c.logger.Error("feeResult %+v", r)
				c.logger.Error("fee %s", fee.String())
				c.logger.Error("init fee %s", iFee.Neg().String())
				return
			}
		}
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

		err = c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee, stake, oi").Where("trader = ?", trader).Scan(&userInfoResults).Error
		if err != nil {
			c.logger.Error("failed to get user info %s", err)
		}
		if len(userInfoResults) == 0 {
			// there is no previous info, means fee equal to totalFee, stake without pre_stake, oi without pre_oi
			c.db.Create(&mining.UserInfo{
				Trader:    trader,
				Fee:       fee,
				OI:        thisEntryValue.Div(fromStartTimeToNow),
				Stake:     thisStakeValue.Div(fromStartTimeToNow),
				Timestamp: timestamp,
			})
		} else {
			pre := userInfoResults[0]
			preEntryValue := pre.OI.Mul(fromStartTimeToLast)
			if preEntryValue.LessThan(decimal.Zero) {
				c.logger.Error("preEntry is less than zero")
				c.logger.Error("value %s", pre.OI.String())
				return
			}
			preStake := pre.Stake.Mul(fromStartTimeToLast)
			if preStake.LessThan(decimal.Zero) {
				c.logger.Error("preStake is less than zero")
				c.logger.Error("value %s", pre.Stake.String())
				return
			}
			c.db.Create(&mining.UserInfo{
				Trader:    trader,
				Fee:       fee,
				OI:        (thisEntryValue.Add(preEntryValue)).Div(fromStartTimeToNow),
				Stake:     (thisStakeValue.Add(preStake)).Div(fromStartTimeToNow),
				Timestamp: timestamp,
			})
		}
	}
	c.lastTimestamp = &now
}

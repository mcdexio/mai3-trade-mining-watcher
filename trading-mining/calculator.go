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
	cal.intervalDecimal = decimal.NewFromFloat((cal.interval * time.Second).Seconds())
	cal.startTimeDecimal = decimal.NewFromInt(cal.startTime.Unix())

	var iniFee []struct {
		UserAdd string
		Fee     decimal.Decimal
	}

	// get the init fee
	err := cal.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee").Where("timestamp < ?", startTime.Unix()).Scan(&iniFee).Error
	if err != nil {
		logger.Error("failed to get user info %s", err)
	}
	for _, fee := range iniFee {
		cal.initFee[fee.UserAdd] = fee.Fee
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
		UserAdd string
		Fee     decimal.Decimal
	}
	err := c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee").Where("timestamp < ?", c.startTime.Unix()).Scan(&iniFee).Error
	if err != nil {
		c.logger.Error("failed to get user info %s", err)
	}
	for _, fee := range iniFee {
		c.initFee[fee.UserAdd] = fee.Fee
	}
}

func (c *Calculator) calculate() {
	now := time.Now()
	if c.startTime.After(now) {
		// not yet
		return
	}

	c.logger.Info("Calculation trading mining...")
	fromStartTimeToNow := decimal.NewFromInt(now.Unix()).Add(c.startTimeDecimal.Neg())
	c.logger.Debug("fromStartTimeToNow %s", fromStartTimeToNow.String())
	if fromStartTimeToNow.LessThanOrEqual(decimal.Zero) {
		c.logger.Error("start time decimal %s", c.startTimeDecimal.String())
		c.logger.Error("now %d", now.Unix())
		return
	}
	fromStartTimeToLast := decimal.NewFromInt(c.lastTimestamp.Unix()).Add(c.startTimeDecimal.Neg())
	c.logger.Debug("fromStartTimeToLast %s", fromStartTimeToLast.String())
	if fromStartTimeToLast.LessThanOrEqual(decimal.Zero) {
		c.logger.Warn("it will happen when first time doing calculation")
		c.logger.Warn("start time decimal %s", c.startTimeDecimal.String())
		c.logger.Warn("last %d", c.lastTimestamp.Unix())
	}

	var feeResults []struct {
		UserAdd   string
		Fee       decimal.Decimal
		Timestamp int64
	}
	var stackResults []struct {
		UserAdd string
		Stack   decimal.Decimal
	}
	var positionResults []struct {
		UserAdd    string
		EntryValue decimal.Decimal
	}
	var userInfoResults []struct {
		UserAdd   string
		Fee       decimal.Decimal
		Stack     decimal.Decimal
		OI        decimal.Decimal
		Timestamp int64
	}

	err := c.db.Model(&mining.Fee{}).Select("DISTINCT user_add").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}
	userCount := len(feeResults)

	// only get the latest one for all user {userCount}
	err = c.db.Model(&mining.Fee{}).Limit(userCount).Order("timestamp desc").Select("user_add, fee, timestamp").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}

	for _, r := range feeResults {
		userAdd := r.UserAdd
		fee := r.Fee
		timestamp := r.Timestamp
		stack := decimal.Zero
		entryValue := decimal.Zero
		err = c.db.Model(&mining.Stack{}).Limit(1).Select("user_add, AVG(stack) as stack").Where("user_add = ? and timestamp > ?", userAdd, c.lastTimestamp.Unix()).Group("user_add").Scan(&stackResults).Error
		if err != nil {
			c.logger.Error("failed to get stack %s", err)
			return
		}
		if len(stackResults) == 1 {
			stack = stackResults[0].Stack
		} else if len(stackResults) == 0 {
			// means this user don't have stack now.
		}

		err = c.db.Model(&mining.Position{}).Limit(1).Select("user_add, AVG(entry_value) as entry_value").Where("user_add = ? and timestamp > ?", userAdd, c.lastTimestamp.Unix()).Group("user_add").Scan(&positionResults).Error
		if err != nil {
			c.logger.Error("failed to get position %s", err)
			return
		}
		if len(positionResults) == 1 {
			entryValue = positionResults[0].EntryValue
		} else if len(positionResults) == 0 {
			// means this user don't have position now.
		}

		// the fee is now_fee - start_fee.
		// the stack is (stack * interval) + (pre_stack * (lastTimeStamp - start_time)) / now - start_time
		// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
		if iFee, match := c.initFee[r.UserAdd]; match {
			fee = fee.Add(iFee.Neg())
			if fee.LessThanOrEqual(decimal.Zero) {
				c.logger.Error("feeResult %+v", r)
				c.logger.Error("fee %s", fee.String())
				c.logger.Error("init fee %s", iFee.Neg().String())
				return
			}
		}
		thisEntryValue := entryValue.Mul(c.intervalDecimal)
		thisStackValue := stack.Mul(c.intervalDecimal)
		if thisStackValue.LessThan(decimal.Zero) {
			c.logger.Error("thisStackValue is less than zero")
			c.logger.Error("value %s", stack.String())
			return
		}
		if thisEntryValue.LessThan(decimal.Zero) {
			c.logger.Error("thisEntryValue is less than zero")
			c.logger.Error("value %s", entryValue.String())
			return
		}

		err = c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee, stack, oi").Where("user_add = ?", userAdd).Scan(&userInfoResults).Error
		if err != nil {
			c.logger.Error("failed to get user info %s", err)
		}
		if len(userInfoResults) == 0 {
			// there is no previous info, means fee equal to totalFee, stack without pre_stack, oi without pre_oi
			c.db.Create(&mining.UserInfo{
				UserAdd:   userAdd,
				Fee:       fee,
				OI:        thisEntryValue.Div(fromStartTimeToNow),
				Stack:     thisStackValue.Div(fromStartTimeToNow),
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
			preStack := pre.Stack.Mul(fromStartTimeToLast)
			if preStack.LessThan(decimal.Zero) {
				c.logger.Error("preStack is less than zero")
				c.logger.Error("value %s", pre.Stack.String())
				return
			}
			c.db.Create(&mining.UserInfo{
				UserAdd:   userAdd,
				Fee:       fee,
				OI:        (thisEntryValue.Add(preEntryValue)).Div(fromStartTimeToNow),
				Stack:     (thisStackValue.Add(preStack)).Div(fromStartTimeToNow),
				Timestamp: timestamp,
			})
		}
	}
	c.lastTimestamp = &now
}

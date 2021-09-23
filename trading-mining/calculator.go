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
	logger         logging.Logger
	ctx            context.Context
	db             *gorm.DB
	interval       time.Duration
	lastCheckpoint *time.Time
	lastTimestamp  *time.Time
	startTime      *time.Time
}

func NewCalculator(ctx context.Context, logger logging.Logger, intervalSec int, startTime *time.Time) (*Calculator, error) {
	cal := &Calculator{
		ctx:       ctx,
		logger:    logger,
		db:        database.GetDB(),
		interval:  time.Duration(intervalSec),
		startTime: startTime,
		lastTimestamp: startTime,
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
		case <-time.After(350 * time.Hour): // 14 days TODO(champFu): endThisEpoch
			c.endThisEpoch()
		}
	}
}

type info struct {
	Fee        decimal.Decimal
	Stack      decimal.Decimal
	EntryValue decimal.Decimal
	Timestamp  int64
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
	c.logger.Info("End this epoch...")
	c.startTime = &now
}

func (c *Calculator) calculate() {
	now := time.Now()
	if c.startTime.After(now) {
		// not yet
		return
	}

	intervalDecimal := decimal.NewFromFloat(c.interval.Seconds())
	starTimeDecimal := decimal.NewFromInt(c.startTime.Unix())
	fromStartTimeToNow := decimal.NewFromInt(now.Unix()).Add(starTimeDecimal.Neg())
	if fromStartTimeToNow.LessThan(decimal.Zero) {
		c.logger.Error("fromStartTimeToNow is less than zero")
		c.logger.Error("value %s", fromStartTimeToNow.String())
		c.logger.Error("start time decimal %s", starTimeDecimal.String())
		c.logger.Error("start time %s", c.startTime.Unix())
		c.logger.Error("now %s", now.Unix())
		return
	}
	fromStartTimeToLast := decimal.NewFromInt(c.lastTimestamp.Unix()).Add(starTimeDecimal.Neg())
	if fromStartTimeToLast.LessThan(decimal.Zero) {
		c.logger.Error("fromStartTimeToLast is less than zero")
		c.logger.Error("value %s", fromStartTimeToLast.String())
		c.logger.Error("start time decimal %s", starTimeDecimal.String())
		c.logger.Error("start time %d", c.startTime.Unix())
		c.logger.Error("last %d", c.lastTimestamp.Unix())
		return
	}

	c.logger.Info("Calculation trading mining...")
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

	// only get the latest one
	err = c.db.Model(&mining.Fee{}).Limit(userCount).Order("timestamp desc").Select("user_add, fee, timestamp").Where("timestamp > ?", c.lastTimestamp.Unix()).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}

	uniqueUser := make(map[string]*info)
	for _, r := range feeResults {
		uniqueUser[r.UserAdd] = &info{Fee: r.Fee, Timestamp: r.Timestamp}
		err = c.db.Model(&mining.Stack{}).Select("user_add, AVG(stack) as stack").Where("user_add = ? and timestamp > ?", r.UserAdd, c.lastTimestamp.Unix()).Group("user_add").Scan(&stackResults).Error
		if err != nil {
			c.logger.Error("failed to get stack %s", err)
			return
		}
		if len(stackResults) == 1 {
			// means this user has stack now.
			if in, match := uniqueUser[r.UserAdd]; match {
				// also has fee
				in.Stack = stackResults[0].Stack
			} else {
				// don't have fee
				uniqueUser[r.UserAdd] = &info{Stack: stackResults[0].Stack, Timestamp: r.Timestamp}
			}
		} else if len(stackResults) == 0 {
			// means this user don't have stack now.
		} else {
			c.logger.Error("can't reach here")
			return
		}

		err = c.db.Model(&mining.Position{}).Select("user_add, AVG(entry_value) as entry_value").Where("user_add = ? and timestamp > ?", r.UserAdd, c.lastTimestamp.Unix()).Group("user_add").Scan(&positionResults).Error
		if err != nil {
			c.logger.Error("failed to get position %s", err)
			return
		}
		if len(positionResults) == 1 {
			// means this user has position now.
			if in, match := uniqueUser[r.UserAdd]; match {
				// also has fee or stack
				in.EntryValue = positionResults[0].EntryValue
			} else {
				// don't have fee and stack
				uniqueUser[r.UserAdd] = &info{EntryValue: positionResults[0].EntryValue, Timestamp: r.Timestamp}
			}
		} else if len(positionResults) == 0 {
			// means this user don't have position now.
		} else {
			c.logger.Error("can't reach here")
			return
		}
	}

	for k, v := range uniqueUser {
		// the fee is now_fee - start_fee.
		// the stack is (stack * interval) + (pre_stack * (lastTimeStamp - start_time)) / now - start_time
		// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
		fee := v.Fee // default
		thisEntryValue := v.EntryValue.Mul(intervalDecimal)
		thisStackValue := v.Stack.Mul(intervalDecimal)
		if thisStackValue.LessThan(decimal.Zero) {
			c.logger.Error("thisStackValue is less than zero")
			c.logger.Error("value %s", v.Stack.String())
			return
		}
		if thisEntryValue.LessThan(decimal.Zero) {
			c.logger.Error("thisEntryValue is less than zero")
			c.logger.Error("value %s", v.EntryValue.String())
			return
		}

		err = c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee, stack, oi").Where("user_add = ?", k).Scan(&userInfoResults).Error
		if err != nil {
			c.logger.Error("failed to get user info %s", err)
		}
		if len(userInfoResults) == 0 {
			// there is no previous info, means fee equal to totalFee, stack without pre_stack, oi without pre_oi
			c.db.Create(&mining.UserInfo{
				UserAdd:   k,
				Fee:       fee,
				OI:        thisEntryValue.Div(fromStartTimeToNow),
				Stack:     thisStackValue.Div(fromStartTimeToNow),
				Timestamp: v.Timestamp,
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
			err = c.db.Model(&mining.UserInfo{}).Limit(1).Order("timestamp desc").Select("fee").Where("user_add = ? and timestamp < ?", k, c.startTime.Unix()).Scan(&userInfoResults).Error
			if err != nil {
				c.logger.Error("failed to get user info %s", err)
			}
			if len(userInfoResults) == 1 {
				begin := userInfoResults[0]
				fee = fee.Add(begin.Fee.Neg())
			}
			c.db.Create(&mining.UserInfo{
				UserAdd: k,
				Fee:     fee,
				OI:      thisEntryValue.Add(preEntryValue).Div(fromStartTimeToNow),
				Stack:   thisStackValue.Add(preStack).Div(fromStartTimeToNow),
				Timestamp: v.Timestamp,
			})
		}
	}

	c.lastTimestamp = &now
}

package trading_mining

import (
	"context"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models/mining"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"sort"
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

	if c.lastTimestamp == nil {
		// first time to calculate, assign a long time ago
		date := time.Date(2019, time.January, 0, 0, 0, 0, 0, time.Local)
		c.lastTimestamp = &date
	}
	intervalDecimal := decimal.NewFromFloat(c.interval.Seconds())
	starTimeDecimal := decimal.NewFromInt(c.startTime.Unix())
	fromStartTimeToNow := decimal.NewFromInt(now.Unix()).Add(starTimeDecimal.Neg())
	fromStartTimeToLast := decimal.NewFromInt(c.lastTimestamp.Unix()).Add(starTimeDecimal.Neg())

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

	err := c.db.Model(&mining.Fee{}).Select("DISTINCT user_add").Where("created_at > ?", c.lastTimestamp).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}
	userCount := len(feeResults)

	// feeResults may contain duplication user_add with different timestamp
	err = c.db.Model(&mining.Fee{}).Select("user_add, fee, timestamp").Where("created_at > ?", c.lastTimestamp).Scan(&feeResults).Error
	if err != nil {
		c.logger.Error("failed to get fee %s", err)
		return
	}
	sort.Slice(feeResults, func(i, j int) bool { return feeResults[i].Timestamp < feeResults[j].Timestamp })

	uniqueUser := make(map[string]*info)
	for i, r := range feeResults {
		if i == userCount-1 {
			// we only get the latest fee
			break
		}
		uniqueUser[r.UserAdd] = &info{Fee: r.Fee, Timestamp: r.Timestamp}
		err = c.db.Model(&mining.Stack{}).Select("user_add, AVG(stack) as stack").Where("user_add = ? and created_at > ?", r.UserAdd, c.lastTimestamp).Group("user_add").Scan(&stackResults).Error
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

		err = c.db.Model(&mining.Position{}).Select("user_add, AVG(entry_value) as entry_value").Where("user_add = ? and created_at > ?", r.UserAdd, c.lastTimestamp).Group("user_add").Scan(&positionResults).Error
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
		err = c.db.Model(&mining.UserInfo{}).Select("fee, stack, oi").Where("user_add = ?", k).Scan(&userInfoResults).Error
		if err != nil {
			c.logger.Error("failed to get user info %s", err)
		}
		// the fee is now_fee - start_fee.
		// the stack is (stack * interval) + (pre_stack * (lastTimeStamp - start_time)) / now - start_time
		// the oi is (oi * interval) + (pre_oi * (lastTimeStamp - start_time)) / now - start_time
		fee := v.Fee // default
		thisEntryValue := v.EntryValue.Mul(intervalDecimal)
		thisStackValue := v.Stack.Mul(intervalDecimal)
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
			sort.Slice(userInfoResults, func(i, j int) bool { return userInfoResults[i].Timestamp < userInfoResults[j].Timestamp })
			for _, in := range userInfoResults {
				// get the fee before startTime
				if in.Timestamp > c.startTime.Unix() {
					continue
				} else {
					// get the closest time when is before startTime
					// and the difference is the totalFee from startTime
					fee = fee.Add(in.Fee.Neg())
					break
				}
			}
			pre := userInfoResults[0]
			preEntryValue := pre.OI.Mul(fromStartTimeToLast)
			preStack := pre.Stack.Mul(fromStartTimeToLast)
			c.db.Create(&mining.UserInfo{
				UserAdd: k,
				Fee:     fee,
				OI:      thisEntryValue.Add(preEntryValue).Div(fromStartTimeToNow),
				Stack:   thisStackValue.Add(preStack).Div(fromStartTimeToNow),
				Timestamp: v.Timestamp
			})
		}
	}

	c.lastTimestamp = &now
}

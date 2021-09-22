package db

import (
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
	"reflect"
	"testing"
)

type InitializeTablesTestSuite struct {
	suite.Suite
}

func (s *InitializeTablesTestSuite) SetupSuite() {
	Initialize()
	Reset(GetDB(), types.Watcher, true)
}

func (s *InitializeTablesTestSuite) TearDownSuite() {
	Finalize()
}

func (s *InitializeTablesTestSuite) TestEmpty() {
	db := GetDB()
	require.NotNil(s.T(), db, "GetDB() must not return nil")

	var recordList []interface{}
	err := initializeTables(recordList, db)
	ok := assert.Nil(s.T(), err, "InitializeTables() failed. err(%s)", err)
	if !ok {
		s.logRecordList(recordList)
		return
	}
}

func initializeTables(recordList []interface{}, tx *gorm.DB) error {
	for idx, record := range recordList {
		typeOfRecord := reflect.TypeOf(record)
		if typeOfRecord.Kind() != reflect.Ptr {
			return fmt.Errorf("typeOfRecord.Kind() != reflect.Ptr")
		}

		if result := tx.Create(record); result.Error != nil {
			logger.Error("tx.Create() (%d/%d) for %s failed! record(%+v), "+
				"result.Error(%s)",
				idx+1,
				len(recordList),
				typeOfRecord.Elem().Name(),
				record,
				result.Error)
			return result.Error
		}
	}
	return nil
}

func (s *InitializeTablesTestSuite) TestMiningRecord() {
	db := GetDB()
	require.NotNil(s.T(), db, "GetDB() must not return nil")
}

func (s *InitializeTablesTestSuite) TestUserInfoRecord() {
	db := GetDB()
	require.NotNil(s.T(), db, "GetDB() must not return nil")
}

func (s *InitializeTablesTestSuite) logRecordList(recordList []interface{}) {
	for idx, record := range recordList {
		s.T().Logf("idx(%d), %+v", idx, record)
	}
}

func TestInitializeTables(t *testing.T) {
	suite.Run(t, &InitializeTablesTestSuite{})
}

package db

import (
	"database/sql"
	"fmt"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/config"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/db/miningdb"
	"github.com/mcdexio/mai3-trade-mining-watcher/database/models"
	"github.com/mcdexio/mai3-trade-mining-watcher/env"
	"github.com/mcdexio/mai3-trade-mining-watcher/types"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"net/url"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Host specifies the database host.
type Host string

// Host enums.
const (
	Default Host = "default"
	Master  Host = "master"
)

var logger = logging.NewLoggerTag("database")

// DB is the global Database instance.
var dbMap map[Host]*gorm.DB
var dbMapMutex sync.Mutex

var dbName = randomDBName()

// NewDB configures given params in shared flow and returns an ORM DB instance with same
// functionalities and features. In this package, we create global DB instance with shared params.
// For external package, e.g: record, it connects to restored instance with different params.
func NewDB(args string, _ Host) (db *gorm.DB, err error) {
	dialector := postgres.Open(args)
	db, err = gorm.Open(dialector,
		&gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
		},
	)
	if err != nil {
		logger.Warn("failed to open gorm db err=%v", err)
		return
	}
	db.Logger.LogMode(0)
	var sqlDB *sql.DB
	sqlDB, err = db.DB()
	if err != nil {
		logger.Warn("failed to get sql.DB %+v from gorm db %+v err=%v", sqlDB, db, err)
		return
	}

	// Set database parameters.
	sqlDB.SetMaxIdleConns(config.GetInt("DB_MAX_IDLE_CONNS"))
	sqlDB.SetMaxOpenConns(config.GetInt("DB_MAX_OPEN_CONNS"))
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	return
}

func randomDBName() string {
	return fmt.Sprintf("test_%v", time.Now().UnixNano())
}

// Initialize initializes models.
// It only creates the connection instance, doesn't reset or migrate anything.
func Initialize(extraHosts ...Host) {
	dbMapMutex.Lock()
	defer dbMapMutex.Unlock()

	hosts := append(extraHosts, Default)
	dbMap = make(map[Host]*gorm.DB)

	for _, host := range hosts {
		if _, e := dbMap[host]; !e {
			logger.Info("Initializing %s database ...", host)
			dbMap[host] = dialDB(host)
		}
	}

	if env.IsCI() {
		if str := config.GetString("DBNAME", ""); str != "" {
			dbName = str
		}
		err := dbMap[Default].Exec("CREATE DATABASE " + dbName).Error
		if err != nil {
			logger.Warn("create database: %v", err)
		}

		var sqlDB *sql.DB
		for key, db := range dbMap {
			sqlDB, err = db.DB()
			if err != nil {
				logger.Warn("failed to get db %v, err=%v", sqlDB, err)
				continue
			}
			if sqlDB != nil {
				err = sqlDB.Close()
				if err != nil {
					logger.Warn("failed to close db %v, err=%v", sqlDB, err)
				}
				delete(dbMap, key)
			}
		}

		args := config.GetString("DB_ARGS")
		req, err := url.Parse(args)
		if err != nil {
			panic(err)
		}
		req.Path = "/" + dbName
		args = req.String()
		logger.Info("Dial to %s", args)

		db, err := NewDB(args, Default)
		if err != nil {
			logger.Critical(err.Error())
		}

		dbMap[Default] = db
	}
	logger.Info("Initialize DONE")
}

// Finalize closes the database and delete it from dbMap.
func Finalize() {
	dbMapMutex.Lock()
	defer dbMapMutex.Unlock()

	for key, db := range dbMap {
		sqlDB, err := db.DB()
		if err != nil {
			logger.Warn("failed to get db %v, err=%v", sqlDB, err)
			continue
		}
		if sqlDB != nil {
			err = sqlDB.Close()
			if err != nil {
				logger.Warn("failed to close db %v, err=%v", sqlDB, err)
			}
			delete(dbMap, key)
		}
	}
}

// GetDB returns the database handle.
func GetDB(host ...Host) *gorm.DB {
	if len(host) > 1 {
		panic("invalid usage of GetDB")
	}

	target := Default
	if len(host) == 1 {
		target = host[0]
	}

	dbMapMutex.Lock()
	ret := dbMap[target]
	dbMapMutex.Unlock()

	if ret != nil {
		return ret
	}
	Initialize(target)

	dbMapMutex.Lock()
	ret = dbMap[target]
	dbMapMutex.Unlock()

	if ret == nil {
		panic("gets nil db: " + target)
	}
	return ret
}

// Return DBApp given an app type.
func dbAppFromType(appType types.AppType) (dbApp DBApp) {
	switch appType {
	case types.Watcher:
		dbApp = &miningdb.WatcherDBApp{}
	default:
		panic("undefined application environment")
	}
	return
}

// Reset resets the entire database. It will:
// 1. Drop all database. 2. Do migration (contains initial schema & default records).
func Reset(db *gorm.DB, appType types.AppType, force bool) {
	dbApp := dbAppFromType(appType)

	if !force && !dbApp.IsEmpty(db) {
		logger.Critical("cryptrade database exists, reset aborted.")
	}

	logger.Info("Resetting database ...")

	// Must make sure only reset() can calls dropAllDatabase().
	if !env.IsCI() || !hasTables(db, dbApp) {
		dropAllDatabase(db, dbApp)
	} else {
		// Ignore schema creation and only insert default record.
		DeleteAllData(appType)
		logger.Info("already has table, ignored")

		err := Transaction(db, func(tx *gorm.DB) error {
			logger.Info("Running post reset hook ...")
			return dbApp.PostReset(tx)
		})

		if err != nil {
			logger.Error("post reset: %v", err)
		}
		return
	}

	// Install UUID extension.
	installUUIDExtension(db)

	// Install gist extension.
	installGISTExtension(db)

	logger.Info("Creating models ...")

	err := Transaction(db, func(tx *gorm.DB) error {
		// Create all tables.
		stmt := &gorm.Statement{DB: db}
		for _, model := range dbApp.Models() {
			err := stmt.Parse(model)
			if err != nil {
				logger.Warn("failed to parse model %+v, err=%v", model, err)
				continue
			}
			logger.Info("tableName %+v", stmt.Schema.Table)
			if e := tx.AutoMigrate(model); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	err = Transaction(db, func(tx *gorm.DB) error {
		logger.Info("Creating indices and constraints ...")
		stmt := &gorm.Statement{DB: db}
		for _, v := range dbApp.Models() {
			err = stmt.Parse(v)
			if err != nil {
				logger.Warn("failed to parse model %+v, err=%v", v, err)
				continue
			}
			tableName := stmt.Schema.Table
			logger.Info("tableName %+v", tableName)
			if e := CreateCustomIndices(tx, v, tableName); e != nil {
				return err
			}

			if e := CreateForeignKeyConstraintsSelf(tx, v, tableName); e != nil {
				return e
			}
		}

		logger.Info("Running post reset hook ...")
		return dbApp.PostReset(tx)
	})
	if err != nil {
		panic(err)

	}
	logger.Info("Reset Done")
}

func dialDB(host Host) *gorm.DB {
	var (
		args string
		db   *gorm.DB
		err  error
	)
	switch host {
	case Default, Master:
		args = config.GetString("DB_ARGS")
	}
	db, err = NewDB(args, host)
	if err != nil {
		logger.Critical(err.Error())
	}
	return db
}

// Transaction wraps the database transaction and to proper error handling.
func Transaction(db *gorm.DB, body func(*gorm.DB) error) (err error) {
	tx := db.Begin()
	if tx.Error != nil {
		logger.Error("Transaction: Cannot open transaction %s", tx.Error.Error())
		return tx.Error
	}

	// Error checking and panic safenet.
	defer func() {
		if err != nil {
			logger.Warn("Transaction: rollback due to error: %v", err)
			if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
				panic(rollbackErr)
			}
		}

		if recovered := recover(); recovered != nil {
			logger.Error("Transaction: rollback due to panic: %v\n%s",
				recovered, string(debug.Stack()))

			err = tx.Rollback().Error
			if err != nil {
				logger.Error("Transaction: rollback failed: %v", err)
			}
			panic(recovered)
		}
	}()

	// Execute main body.
	if err = body(tx); err != nil {
		return err
	}

	err = tx.Commit().Error
	if err != nil {
		return err
	}

	return nil
}

func hasTables(db *gorm.DB, app DBApp) bool {
	// db has all tables in DBApp database or not.
	dbAllModels := app.Models()

	type Result struct {
		TableName string
	}

	var results []Result
	err := db.Raw(`SELECT table_name FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema = CURRENT_SCHEMA()`).Scan(&results).Error
	if err != nil {
		return false
	}

	if len(results) != len(dbAllModels) {
		return false
	}

	stmt := &gorm.Statement{DB: db}
	// iterate all DBApp models.
	for _, v := range dbAllModels {
		err = stmt.Parse(v)
		if err != nil {
			logger.Warn("failed to parse model %+v", v)
			continue
		}
		tableName := stmt.Schema.Table
		found := false
		for _, res := range results {
			if res.TableName == tableName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// DeleteAllData with `DELETE` since TRUNCATE runs so slow in cockroachdb
// This method provides same functionality as `dropAllDatabase`
func DeleteAllData(appType types.AppType) {
	dbApp := dbAppFromType(appType)

	logger.Info("`DELETE` data in all tables")
	var db *gorm.DB
	dbMapMutex.Lock()
	defer dbMapMutex.Unlock()
	if dbMap != nil {
		db = dbMap[Default]
	}
	if db == nil {
		db = dialDB(Master)
		sqlDB, err := db.DB()
		if err != nil {
			logger.Warn("failed to get db %v, err=%v", sqlDB, err)
		}
		if sqlDB != nil {
			defer sqlDB.Close() //nolint:errcheck
		}
	}
	err := dbApp.PreReset(db)
	if err != nil {
		logger.Warn("failed to PreReset db %+v, err=%v", db, err)
	}

	tx := db.Begin()
	allModels := dbApp.Models()
	stmt := &gorm.Statement{DB: db}
	for i := 0; i < len(allModels); i++ {
		m := allModels[len(allModels)-1-i] // TODO(ChampFu): why get the last i index
		err = stmt.Parse(m)
		if err != nil {
			logger.Warn("failed to parse model %+v err=%v", m, err)
			continue
		}
		tx.Exec(fmt.Sprintf("DELETE FROM \"%v\"", stmt.Schema.Table))
	}
	// We may call DeleteAllData() at SetupSuite and call Reset() at SetupTest.
	// It will cause TRUNCATE failed, so we ignore Commit().Error.
	tx.Commit()

	logger.Info("DeleteAllData Done")
}

// CreateCustomIndices creates custom indices if model implements models.CustomIndexer.
func CreateCustomIndices(tx *gorm.DB, model interface{}, tableName string) error {
	if m, ok := model.(models.CustomIndexer); ok {
		for _, idx := range m.Indexes() {
			unique := ""
			extension := ""
			if idx.Unique {
				unique = "UNIQUE"
			}
			if 0 != len(idx.Type) {
				extension = "USING " + idx.Type
			}
			columns := strings.Join(idx.Fields, ",")
			idxStat := fmt.Sprintf(
				`CREATE %s INDEX IF NOT EXISTS %s_%s ON "%s" %s(%s) %s`,
				unique, tableName, idx.Name, tableName, extension, columns, idx.Condition)
			err := tx.Model(model).Exec(idxStat).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// buildForeignKeyName is copy from gorm.
func buildForeignKeyName(tableName, field, dest string) string {
	keyName := fmt.Sprintf("%s_%s_%s_foreign", tableName, field, dest)
	keyName = regexp.MustCompile("(_*[^a-zA-Z]+_*|_+)").ReplaceAllString(keyName, "_")
	return keyName
}

// CreateForeignKeyConstraintsSelf creates foreign key constraint if model implements
// models.ForeignKeyConstrainer.
func CreateForeignKeyConstraintsSelf(tx *gorm.DB, model interface{}, tableName string) error {
	if m, ok := model.(models.ForeignKeyConstrainer); ok {
		for _, c := range m.ForeignKeyConstraints() {
			keyName := buildForeignKeyName(tableName, c.Field, c.Dest)

			// result: ALTER TABLE table_name IF EXISTS ADD CONSTRAINT key_name FOREIGN KEY (field) REFERENCES dest ON DELETE onDelete ON UPDATE onUpdate;
			err := tx.Exec(fmt.Sprintf("ALTER TABLE IF EXISTS \"%s\" ADD CONSTRAINT "+
				"%s FOREIGN KEY (%s) REFERENCES %s ON DELETE %s ON UPDATE %s", tableName, keyName, c.Field, c.Dest, c.OnDelete, c.OnUpdate)).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func installUUIDExtension(db *gorm.DB) {
	err := db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"").Error
	if err != nil {
		logger.Error(err.Error())
	}
}

func installGISTExtension(db *gorm.DB) {
	err := db.Exec("CREATE EXTENSION IF NOT EXISTS btree_gist").Error
	if err != nil {
		logger.Error(err.Error())
	}
}

func dropAllDatabase(db *gorm.DB, dbApp DBApp) {
	logger.Info("Dropping old database ...")
	tx := db.Begin()
	stmt := &gorm.Statement{DB: db}

	for _, model := range dbApp.Models() {
		err := stmt.Parse(model)
		if err != nil {
			logger.Warn("failed to parse model %+v, err=%v", model, err)
			continue
		}
		if stmt.Schema.Table == "system" {
			logger.Info("Skip system table")
			continue
		}
		sql := fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE", stmt.Schema.Table)
		if err := tx.Exec(sql).Error; err != nil {
			logger.Error("Exec '%s' failed. err: %s", sql, err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		panic(err)
	}
}

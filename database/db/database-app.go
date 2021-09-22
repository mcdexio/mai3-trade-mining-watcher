package db

import "gorm.io/gorm"

// DBApp is an interface for different database applications.
type DBApp interface {
	// Models returns the models for a given database app.
	Models() []interface{}

	// IsEmpty check if a given database is empty.
	IsEmpty(db *gorm.DB) bool

	// PreReset is executed before db is reset.
	PreReset(db *gorm.DB) error

	// PostReset is executed after db is reset.
	PostReset(db *gorm.DB) error
}

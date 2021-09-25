package types

// AppType specifies app type.
type AppType string

// Watcher AppType enums.
const (
	Watcher AppType = "watch"
)

// SysVar specifies the system variables.
type SysVar string

// SysVarSchemaVersion SysVar enums.
const (
	SysVarSchemaVersion SysVar = "schema_version"
)

// TableName specifies table name.
type TableName string

const (
	Block TableName = "Block"
)

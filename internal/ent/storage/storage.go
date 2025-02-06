// Code generated by ent, DO NOT EDIT.

package storage

import (
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

const (
	// Label holds the string label denoting the storage type in the database.
	Label = "storage"
	// FieldID holds the string denoting the id field in the database.
	FieldID = "id"
	// FieldUpdateType holds the string denoting the update_type field in the database.
	FieldUpdateType = "update_type"
	// FieldOs holds the string denoting the os field in the database.
	FieldOs = "os"
	// FieldArch holds the string denoting the arch field in the database.
	FieldArch = "arch"
	// FieldPackagePath holds the string denoting the package_path field in the database.
	FieldPackagePath = "package_path"
	// FieldPackageHash holds the string denoting the package_hash field in the database.
	FieldPackageHash = "package_hash"
	// FieldResourcePath holds the string denoting the resource_path field in the database.
	FieldResourcePath = "resource_path"
	// FieldFileHashes holds the string denoting the file_hashes field in the database.
	FieldFileHashes = "file_hashes"
	// FieldCreatedAt holds the string denoting the created_at field in the database.
	FieldCreatedAt = "created_at"
	// EdgeVersion holds the string denoting the version edge name in mutations.
	EdgeVersion = "version"
	// EdgeOldVersion holds the string denoting the old_version edge name in mutations.
	EdgeOldVersion = "old_version"
	// Table holds the table name of the storage in the database.
	Table = "storages"
	// VersionTable is the table that holds the version relation/edge.
	VersionTable = "storages"
	// VersionInverseTable is the table name for the Version entity.
	// It exists in this package in order to avoid circular dependency with the "version" package.
	VersionInverseTable = "versions"
	// VersionColumn is the table column denoting the version relation/edge.
	VersionColumn = "version_storages"
	// OldVersionTable is the table that holds the old_version relation/edge.
	OldVersionTable = "storages"
	// OldVersionInverseTable is the table name for the Version entity.
	// It exists in this package in order to avoid circular dependency with the "version" package.
	OldVersionInverseTable = "versions"
	// OldVersionColumn is the table column denoting the old_version relation/edge.
	OldVersionColumn = "storage_old_version"
)

// Columns holds all SQL columns for storage fields.
var Columns = []string{
	FieldID,
	FieldUpdateType,
	FieldOs,
	FieldArch,
	FieldPackagePath,
	FieldPackageHash,
	FieldResourcePath,
	FieldFileHashes,
	FieldCreatedAt,
}

// ForeignKeys holds the SQL foreign-keys that are owned by the "storages"
// table and are not defined as standalone fields in the schema.
var ForeignKeys = []string{
	"storage_old_version",
	"version_storages",
}

// ValidColumn reports if the column name is valid (part of the table columns).
func ValidColumn(column string) bool {
	for i := range Columns {
		if column == Columns[i] {
			return true
		}
	}
	for i := range ForeignKeys {
		if column == ForeignKeys[i] {
			return true
		}
	}
	return false
}

var (
	// DefaultOs holds the default value on creation for the "os" field.
	DefaultOs string
	// DefaultArch holds the default value on creation for the "arch" field.
	DefaultArch string
	// DefaultCreatedAt holds the default value on creation for the "created_at" field.
	DefaultCreatedAt func() time.Time
)

// UpdateType defines the type for the "update_type" enum field.
type UpdateType string

// UpdateType values.
const (
	UpdateTypeFull        UpdateType = "full"
	UpdateTypeIncremental UpdateType = "incremental"
)

func (ut UpdateType) String() string {
	return string(ut)
}

// UpdateTypeValidator is a validator for the "update_type" field enum values. It is called by the builders before save.
func UpdateTypeValidator(ut UpdateType) error {
	switch ut {
	case UpdateTypeFull, UpdateTypeIncremental:
		return nil
	default:
		return fmt.Errorf("storage: invalid enum value for update_type field: %q", ut)
	}
}

// OrderOption defines the ordering options for the Storage queries.
type OrderOption func(*sql.Selector)

// ByID orders the results by the id field.
func ByID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldID, opts...).ToFunc()
}

// ByUpdateType orders the results by the update_type field.
func ByUpdateType(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldUpdateType, opts...).ToFunc()
}

// ByOs orders the results by the os field.
func ByOs(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldOs, opts...).ToFunc()
}

// ByArch orders the results by the arch field.
func ByArch(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldArch, opts...).ToFunc()
}

// ByPackagePath orders the results by the package_path field.
func ByPackagePath(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldPackagePath, opts...).ToFunc()
}

// ByPackageHash orders the results by the package_hash field.
func ByPackageHash(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldPackageHash, opts...).ToFunc()
}

// ByResourcePath orders the results by the resource_path field.
func ByResourcePath(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldResourcePath, opts...).ToFunc()
}

// ByCreatedAt orders the results by the created_at field.
func ByCreatedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldCreatedAt, opts...).ToFunc()
}

// ByVersionField orders the results by version field.
func ByVersionField(field string, opts ...sql.OrderTermOption) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, newVersionStep(), sql.OrderByField(field, opts...))
	}
}

// ByOldVersionField orders the results by old_version field.
func ByOldVersionField(field string, opts ...sql.OrderTermOption) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, newOldVersionStep(), sql.OrderByField(field, opts...))
	}
}
func newVersionStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From(Table, FieldID),
		sqlgraph.To(VersionInverseTable, FieldID),
		sqlgraph.Edge(sqlgraph.M2O, true, VersionTable, VersionColumn),
	)
}
func newOldVersionStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From(Table, FieldID),
		sqlgraph.To(OldVersionInverseTable, FieldID),
		sqlgraph.Edge(sqlgraph.M2O, false, OldVersionTable, OldVersionColumn),
	)
}

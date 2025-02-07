// Code generated by ent, DO NOT EDIT.

package version

import (
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

const (
	// Label holds the string label denoting the version type in the database.
	Label = "version"
	// FieldID holds the string denoting the id field in the database.
	FieldID = "id"
	// FieldChannel holds the string denoting the channel field in the database.
	FieldChannel = "channel"
	// FieldName holds the string denoting the name field in the database.
	FieldName = "name"
	// FieldNumber holds the string denoting the number field in the database.
	FieldNumber = "number"
	// FieldReleaseNote holds the string denoting the release_note field in the database.
	FieldReleaseNote = "release_note"
	// FieldCustomData holds the string denoting the custom_data field in the database.
	FieldCustomData = "custom_data"
	// FieldCreatedAt holds the string denoting the created_at field in the database.
	FieldCreatedAt = "created_at"
	// EdgeStorages holds the string denoting the storages edge name in mutations.
	EdgeStorages = "storages"
	// EdgeResource holds the string denoting the resource edge name in mutations.
	EdgeResource = "resource"
	// Table holds the table name of the version in the database.
	Table = "versions"
	// StoragesTable is the table that holds the storages relation/edge.
	StoragesTable = "storages"
	// StoragesInverseTable is the table name for the Storage entity.
	// It exists in this package in order to avoid circular dependency with the "storage" package.
	StoragesInverseTable = "storages"
	// StoragesColumn is the table column denoting the storages relation/edge.
	StoragesColumn = "version_storages"
	// ResourceTable is the table that holds the resource relation/edge.
	ResourceTable = "versions"
	// ResourceInverseTable is the table name for the Resource entity.
	// It exists in this package in order to avoid circular dependency with the "resource" package.
	ResourceInverseTable = "resources"
	// ResourceColumn is the table column denoting the resource relation/edge.
	ResourceColumn = "resource_versions"
)

// Columns holds all SQL columns for version fields.
var Columns = []string{
	FieldID,
	FieldChannel,
	FieldName,
	FieldNumber,
	FieldReleaseNote,
	FieldCustomData,
	FieldCreatedAt,
}

// ForeignKeys holds the SQL foreign-keys that are owned by the "versions"
// table and are not defined as standalone fields in the schema.
var ForeignKeys = []string{
	"resource_versions",
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
	// NameValidator is a validator for the "name" field. It is called by the builders before save.
	NameValidator func(string) error
	// DefaultReleaseNote holds the default value on creation for the "release_note" field.
	DefaultReleaseNote string
	// DefaultCustomData holds the default value on creation for the "custom_data" field.
	DefaultCustomData string
	// DefaultCreatedAt holds the default value on creation for the "created_at" field.
	DefaultCreatedAt func() time.Time
)

// Channel defines the type for the "channel" enum field.
type Channel string

// ChannelStable is the default value of the Channel enum.
const DefaultChannel = ChannelStable

// Channel values.
const (
	ChannelStable Channel = "stable"
	ChannelAlpha  Channel = "alpha"
	ChannelBeta   Channel = "beta"
)

func (c Channel) String() string {
	return string(c)
}

// ChannelValidator is a validator for the "channel" field enum values. It is called by the builders before save.
func ChannelValidator(c Channel) error {
	switch c {
	case ChannelStable, ChannelAlpha, ChannelBeta:
		return nil
	default:
		return fmt.Errorf("version: invalid enum value for channel field: %q", c)
	}
}

// OrderOption defines the ordering options for the Version queries.
type OrderOption func(*sql.Selector)

// ByID orders the results by the id field.
func ByID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldID, opts...).ToFunc()
}

// ByChannel orders the results by the channel field.
func ByChannel(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldChannel, opts...).ToFunc()
}

// ByName orders the results by the name field.
func ByName(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldName, opts...).ToFunc()
}

// ByNumber orders the results by the number field.
func ByNumber(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldNumber, opts...).ToFunc()
}

// ByReleaseNote orders the results by the release_note field.
func ByReleaseNote(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldReleaseNote, opts...).ToFunc()
}

// ByCustomData orders the results by the custom_data field.
func ByCustomData(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldCustomData, opts...).ToFunc()
}

// ByCreatedAt orders the results by the created_at field.
func ByCreatedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldCreatedAt, opts...).ToFunc()
}

// ByStoragesCount orders the results by storages count.
func ByStoragesCount(opts ...sql.OrderTermOption) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborsCount(s, newStoragesStep(), opts...)
	}
}

// ByStorages orders the results by storages terms.
func ByStorages(term sql.OrderTerm, terms ...sql.OrderTerm) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, newStoragesStep(), append([]sql.OrderTerm{term}, terms...)...)
	}
}

// ByResourceField orders the results by resource field.
func ByResourceField(field string, opts ...sql.OrderTermOption) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, newResourceStep(), sql.OrderByField(field, opts...))
	}
}
func newStoragesStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From(Table, FieldID),
		sqlgraph.To(StoragesInverseTable, FieldID),
		sqlgraph.Edge(sqlgraph.O2M, false, StoragesTable, StoragesColumn),
	)
}
func newResourceStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From(Table, FieldID),
		sqlgraph.To(ResourceInverseTable, FieldID),
		sqlgraph.Edge(sqlgraph.M2O, true, ResourceTable, ResourceColumn),
	)
}

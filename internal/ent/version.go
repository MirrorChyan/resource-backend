// Code generated by ent, DO NOT EDIT.

package ent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

// Version is the model entity for the Version schema.
type Version struct {
	config `json:"-"`
	// ID of the ent.
	ID int `json:"id,omitempty"`
	// Name holds the value of the "name" field.
	Name string `json:"name,omitempty"`
	// Number holds the value of the "number" field.
	Number uint64 `json:"number,omitempty"`
	// FileHashes holds the value of the "file_hashes" field.
	FileHashes map[string]string `json:"file_hashes,omitempty"`
	// CreatedAt holds the value of the "created_at" field.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// Edges holds the relations/edges for other nodes in the graph.
	// The values are being populated by the VersionQuery when eager-loading is set.
	Edges             VersionEdges `json:"edges"`
	resource_versions *string
	selectValues      sql.SelectValues
}

// VersionEdges holds the relations/edges for other nodes in the graph.
type VersionEdges struct {
	// Storage holds the value of the storage edge.
	Storage *Storage `json:"storage,omitempty"`
	// Resource holds the value of the resource edge.
	Resource *Resource `json:"resource,omitempty"`
	// loadedTypes holds the information for reporting if a
	// type was loaded (or requested) in eager-loading or not.
	loadedTypes [2]bool
}

// StorageOrErr returns the Storage value or an error if the edge
// was not loaded in eager-loading, or loaded but was not found.
func (e VersionEdges) StorageOrErr() (*Storage, error) {
	if e.Storage != nil {
		return e.Storage, nil
	} else if e.loadedTypes[0] {
		return nil, &NotFoundError{label: storage.Label}
	}
	return nil, &NotLoadedError{edge: "storage"}
}

// ResourceOrErr returns the Resource value or an error if the edge
// was not loaded in eager-loading, or loaded but was not found.
func (e VersionEdges) ResourceOrErr() (*Resource, error) {
	if e.Resource != nil {
		return e.Resource, nil
	} else if e.loadedTypes[1] {
		return nil, &NotFoundError{label: resource.Label}
	}
	return nil, &NotLoadedError{edge: "resource"}
}

// scanValues returns the types for scanning values from sql.Rows.
func (*Version) scanValues(columns []string) ([]any, error) {
	values := make([]any, len(columns))
	for i := range columns {
		switch columns[i] {
		case version.FieldFileHashes:
			values[i] = new([]byte)
		case version.FieldID, version.FieldNumber:
			values[i] = new(sql.NullInt64)
		case version.FieldName:
			values[i] = new(sql.NullString)
		case version.FieldCreatedAt:
			values[i] = new(sql.NullTime)
		case version.ForeignKeys[0]: // resource_versions
			values[i] = new(sql.NullString)
		default:
			values[i] = new(sql.UnknownType)
		}
	}
	return values, nil
}

// assignValues assigns the values that were returned from sql.Rows (after scanning)
// to the Version fields.
func (v *Version) assignValues(columns []string, values []any) error {
	if m, n := len(values), len(columns); m < n {
		return fmt.Errorf("mismatch number of scan values: %d != %d", m, n)
	}
	for i := range columns {
		switch columns[i] {
		case version.FieldID:
			value, ok := values[i].(*sql.NullInt64)
			if !ok {
				return fmt.Errorf("unexpected type %T for field id", value)
			}
			v.ID = int(value.Int64)
		case version.FieldName:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field name", values[i])
			} else if value.Valid {
				v.Name = value.String
			}
		case version.FieldNumber:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field number", values[i])
			} else if value.Valid {
				v.Number = uint64(value.Int64)
			}
		case version.FieldFileHashes:
			if value, ok := values[i].(*[]byte); !ok {
				return fmt.Errorf("unexpected type %T for field file_hashes", values[i])
			} else if value != nil && len(*value) > 0 {
				if err := json.Unmarshal(*value, &v.FileHashes); err != nil {
					return fmt.Errorf("unmarshal field file_hashes: %w", err)
				}
			}
		case version.FieldCreatedAt:
			if value, ok := values[i].(*sql.NullTime); !ok {
				return fmt.Errorf("unexpected type %T for field created_at", values[i])
			} else if value.Valid {
				v.CreatedAt = value.Time
			}
		case version.ForeignKeys[0]:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field resource_versions", values[i])
			} else if value.Valid {
				v.resource_versions = new(string)
				*v.resource_versions = value.String
			}
		default:
			v.selectValues.Set(columns[i], values[i])
		}
	}
	return nil
}

// Value returns the ent.Value that was dynamically selected and assigned to the Version.
// This includes values selected through modifiers, order, etc.
func (v *Version) Value(name string) (ent.Value, error) {
	return v.selectValues.Get(name)
}

// QueryStorage queries the "storage" edge of the Version entity.
func (v *Version) QueryStorage() *StorageQuery {
	return NewVersionClient(v.config).QueryStorage(v)
}

// QueryResource queries the "resource" edge of the Version entity.
func (v *Version) QueryResource() *ResourceQuery {
	return NewVersionClient(v.config).QueryResource(v)
}

// Update returns a builder for updating this Version.
// Note that you need to call Version.Unwrap() before calling this method if this Version
// was returned from a transaction, and the transaction was committed or rolled back.
func (v *Version) Update() *VersionUpdateOne {
	return NewVersionClient(v.config).UpdateOne(v)
}

// Unwrap unwraps the Version entity that was returned from a transaction after it was closed,
// so that all future queries will be executed through the driver which created the transaction.
func (v *Version) Unwrap() *Version {
	_tx, ok := v.config.driver.(*txDriver)
	if !ok {
		panic("ent: Version is not a transactional entity")
	}
	v.config.driver = _tx.drv
	return v
}

// String implements the fmt.Stringer.
func (v *Version) String() string {
	var builder strings.Builder
	builder.WriteString("Version(")
	builder.WriteString(fmt.Sprintf("id=%v, ", v.ID))
	builder.WriteString("name=")
	builder.WriteString(v.Name)
	builder.WriteString(", ")
	builder.WriteString("number=")
	builder.WriteString(fmt.Sprintf("%v", v.Number))
	builder.WriteString(", ")
	builder.WriteString("file_hashes=")
	builder.WriteString(fmt.Sprintf("%v", v.FileHashes))
	builder.WriteString(", ")
	builder.WriteString("created_at=")
	builder.WriteString(v.CreatedAt.Format(time.ANSIC))
	builder.WriteByte(')')
	return builder.String()
}

// Versions is a parsable slice of Version.
type Versions []*Version

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// StorageFileHash holds the compressed file hashes of a full-update storage.
// A full storage without file hashes has no row in this table.
type StorageFileHash struct {
	ent.Schema
}

// Annotations of the StorageFileHash.
func (StorageFileHash) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "storage_file_hashes"},
	}
}

// Fields of the StorageFileHash.
func (StorageFileHash) Fields() []ent.Field {
	return []ent.Field{
		field.Int("storage_id").
			Unique().
			Immutable(),
		field.Uint8("format"),
		field.Bytes("data").
			SchemaType(map[string]string{
				dialect.MySQL: "longblob",
			}),
		field.Int("raw_size"),
		field.Int("entries"),
	}
}

// Edges of the StorageFileHash.
func (StorageFileHash) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("storage", Storage.Type).
			Ref("file_hash").
			Field("storage_id").
			Unique().
			Required().
			Immutable(),
	}
}

package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Storage holds the schema definition for the Storage entity.
type Storage struct {
	ent.Schema
}

// Fields of the Storage.
func (Storage) Fields() []ent.Field {
	return []ent.Field{
		field.String("directory"),
		field.Time("created_at").
			Default(time.Now()),
	}
}

// Edges of the Storage.
func (Storage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("version", Version.Type).
			Ref("storage").
			Unique(),
	}
}

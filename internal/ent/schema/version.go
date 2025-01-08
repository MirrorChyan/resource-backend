package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Version holds the schema definition for the Version entity.
type Version struct {
	ent.Schema
}

// Fields of the Version.
func (Version) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Uint64("number"),
		field.JSON("file_hashes", map[string]string{}).
			Optional(),
		field.Time("created_at").
			Default(time.Now()),
	}
}

// Edges of the Version.
func (Version) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("storage", Storage.Type),
		edge.From("resource", Resource.Type).
			Ref("versions").
			Unique(),
	}
}

package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Resource holds the schema definition for the Resource entity.
type Resource struct {
	ent.Schema
}

// Fields of the Resource.
func (Resource) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("description"),
		field.String("latest_version").
			Optional(),
		field.Time("created_at").
			Default(time.Now),
	}
}

// Edges of the Resource.
func (Resource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("versions", Version.Type),
	}
}

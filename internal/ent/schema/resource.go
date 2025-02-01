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
		field.String("id").
			NotEmpty().
			Unique(),
		field.String("name").
			NotEmpty(),
		field.String("description"),
		field.Time("created_at").
			Default(time.Now),
	}
}

// Edges of the Resource.
func (Resource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("versions", Version.Type),
		edge.To("latest_versions", LatestVersion.Type),
	}
}

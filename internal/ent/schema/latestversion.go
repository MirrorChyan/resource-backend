package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LatestVersion holds the schema definition for the LatestVersion entity.
type LatestVersion struct {
	ent.Schema
}

// Fields of the LatestVersion.
func (LatestVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("channel").
			Values("stable", "beta", "alpha").
			Default("stable"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the LatestVersion.
func (LatestVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("resource", Resource.Type).
			Ref("latest_versions").
			Unique().
			Required(),
		edge.To("version", Version.Type).
			Unique().
			Required(),
	}
}

// Indexs of the LatestVersion
func (LatestVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel").
			Edges("resource").
			Unique(),
	}
}

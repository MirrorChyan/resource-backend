package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
)

// Version holds the schema definition for the Version entity.
type Version struct {
	ent.Schema
}

// Fields of the Version.
func (Version) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("channel").
			Values(
				types.ChannelStable.String(),
				types.ChannelBeta.String(),
				types.ChannelAlpha.String(),
			).
			Default(types.ChannelStable.String()),
		field.String("name").
			NotEmpty(),
		field.Uint64("number"),
		field.String("release_note").
			SchemaType(
				map[string]string{
					dialect.MySQL: "longtext",
				}).
			Default(""),
		field.String("custom_data").
			SchemaType(
				map[string]string{
					dialect.MySQL: "longtext",
				}).
			Default(""),
		field.Time("created_at").
			Default(time.Now),
	}
}

// Edges of the Version.
func (Version) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("storages", Storage.Type),
		edge.From("resource", Resource.Type).
			Ref("versions").
			Unique(),
	}
}

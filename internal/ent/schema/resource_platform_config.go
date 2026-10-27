package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
)

// ResourcePlatformConfig holds the settings one platform of a resource has of its own;
// a platform without a row uses the settings of the resource.
type ResourcePlatformConfig struct {
	ent.Schema
}

// Fields of the ResourcePlatformConfig.
func (ResourcePlatformConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("resource_id").
			NotEmpty(),
		// os and arch take the values of storages: "" is the platform-independent
		// package there, so it names that platform here and is no wildcard.
		// short columns keep the unique index well under the InnoDB key length limit
		field.String("os").
			MaxLen(32).
			Default(""),
		field.String("arch").
			MaxLen(32).
			Default(""),
		field.Enum("update_type").
			Values(
				types.UpdateFull.String(),
				types.UpdateIncremental.String(),
			),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the ResourcePlatformConfig.
func (ResourcePlatformConfig) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("resource", Resource.Type).
			Ref("platform_configs").
			Field("resource_id").
			Unique().
			Required(),
	}
}

// Indexes of the ResourcePlatformConfig.
func (ResourcePlatformConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("resource_id", "os", "arch").
			Unique(),
	}
}

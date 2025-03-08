package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
)

// Storage holds the schema definition for the Storage entity.
type Storage struct {
	ent.Schema
}

// Fields of the Storage.
func (Storage) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("update_type").
			Values(
				types.UpdateFull.String(),
				types.UpdateIncremental.String(),
			),
		field.String("os").
			Default(""),
		field.String("arch").
			Default(""),
		field.String("package_path").
			Optional(),
		field.String("package_hash_sha256").
			Optional(),
		field.String("resource_path").
			Optional().
			Comment("only for full update"),
		field.JSON("file_hashes", map[string]string{}).
			Optional().
			Comment("only for full update"),
		field.Time("created_at").
			Default(time.Now),
		field.Int("version_storages"),
	}
}

// Edges of the Storage.
func (Storage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("version", Version.Type).
			Field("version_storages").
			Ref("storages").
			Unique().
			Required(),
		edge.To("old_version", Version.Type).
			Unique().
			Comment("only for incremental update"),
	}
}

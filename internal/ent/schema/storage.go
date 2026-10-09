package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
		// zip, tgz, etc..
		field.String("file_type").
			Optional().
			Comment("only for full update"),
		field.Int64("file_size").
			Default(0).
			Comment("file size"),
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

// Indexes of the Storage.
func (Storage) Indexes() []ent.Index {
	return []ent.Index{
		// one incremental package per version pair and platform; a full storage has no
		// old version and NULLs never collide, so only incremental storages are bound
		index.Fields("version_storages", "os", "arch", "update_type").
			Edges("old_version").
			Unique().
			StorageKey("storage_version_os_arch_type_old_version"),
	}
}

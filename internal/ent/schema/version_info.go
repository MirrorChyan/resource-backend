package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"time"
)

// VersionInfo holds the schema definition for the VersionInfo entity.
type VersionInfo struct {
	ent.Schema
}

// Fields of the VersionInfo.
func (VersionInfo) Fields() []ent.Field {
	return []ent.Field{
		field.String("version_name").NotEmpty(),
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

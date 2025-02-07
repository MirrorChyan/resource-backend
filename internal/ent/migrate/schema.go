// Code generated by ent, DO NOT EDIT.

package migrate

import (
	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
)

var (
	// LatestVersionsColumns holds the columns for the "latest_versions" table.
	LatestVersionsColumns = []*schema.Column{
		{Name: "id", Type: field.TypeInt, Increment: true},
		{Name: "channel", Type: field.TypeEnum, Enums: []string{"stable", "beta", "alpha"}, Default: "stable"},
		{Name: "updated_at", Type: field.TypeTime},
		{Name: "latest_version_version", Type: field.TypeInt},
		{Name: "resource_latest_versions", Type: field.TypeString},
	}
	// LatestVersionsTable holds the schema information for the "latest_versions" table.
	LatestVersionsTable = &schema.Table{
		Name:       "latest_versions",
		Columns:    LatestVersionsColumns,
		PrimaryKey: []*schema.Column{LatestVersionsColumns[0]},
		ForeignKeys: []*schema.ForeignKey{
			{
				Symbol:     "latest_versions_versions_version",
				Columns:    []*schema.Column{LatestVersionsColumns[3]},
				RefColumns: []*schema.Column{VersionsColumns[0]},
				OnDelete:   schema.NoAction,
			},
			{
				Symbol:     "latest_versions_resources_latest_versions",
				Columns:    []*schema.Column{LatestVersionsColumns[4]},
				RefColumns: []*schema.Column{ResourcesColumns[0]},
				OnDelete:   schema.NoAction,
			},
		},
		Indexes: []*schema.Index{
			{
				Name:    "latestversion_channel_resource_latest_versions",
				Unique:  true,
				Columns: []*schema.Column{LatestVersionsColumns[1], LatestVersionsColumns[4]},
			},
		},
	}
	// ResourcesColumns holds the columns for the "resources" table.
	ResourcesColumns = []*schema.Column{
		{Name: "id", Type: field.TypeString, Unique: true},
		{Name: "name", Type: field.TypeString},
		{Name: "description", Type: field.TypeString},
		{Name: "created_at", Type: field.TypeTime},
	}
	// ResourcesTable holds the schema information for the "resources" table.
	ResourcesTable = &schema.Table{
		Name:       "resources",
		Columns:    ResourcesColumns,
		PrimaryKey: []*schema.Column{ResourcesColumns[0]},
	}
	// StoragesColumns holds the columns for the "storages" table.
	StoragesColumns = []*schema.Column{
		{Name: "id", Type: field.TypeInt, Increment: true},
		{Name: "update_type", Type: field.TypeEnum, Enums: []string{"full", "incremental"}},
		{Name: "os", Type: field.TypeString, Default: ""},
		{Name: "arch", Type: field.TypeString, Default: ""},
		{Name: "package_path", Type: field.TypeString, Nullable: true},
		{Name: "package_hash_sha256", Type: field.TypeString, Nullable: true},
		{Name: "resource_path", Type: field.TypeString, Nullable: true},
		{Name: "file_hashes", Type: field.TypeJSON, Nullable: true},
		{Name: "created_at", Type: field.TypeTime},
		{Name: "storage_old_version", Type: field.TypeInt, Nullable: true},
		{Name: "version_storages", Type: field.TypeInt},
	}
	// StoragesTable holds the schema information for the "storages" table.
	StoragesTable = &schema.Table{
		Name:       "storages",
		Columns:    StoragesColumns,
		PrimaryKey: []*schema.Column{StoragesColumns[0]},
		ForeignKeys: []*schema.ForeignKey{
			{
				Symbol:     "storages_versions_old_version",
				Columns:    []*schema.Column{StoragesColumns[9]},
				RefColumns: []*schema.Column{VersionsColumns[0]},
				OnDelete:   schema.SetNull,
			},
			{
				Symbol:     "storages_versions_storages",
				Columns:    []*schema.Column{StoragesColumns[10]},
				RefColumns: []*schema.Column{VersionsColumns[0]},
				OnDelete:   schema.NoAction,
			},
		},
	}
	// VersionsColumns holds the columns for the "versions" table.
	VersionsColumns = []*schema.Column{
		{Name: "id", Type: field.TypeInt, Increment: true},
		{Name: "channel", Type: field.TypeEnum, Enums: []string{"stable", "alpha", "beta"}, Default: "stable"},
		{Name: "name", Type: field.TypeString},
		{Name: "number", Type: field.TypeUint64},
		{Name: "release_note_summary", Type: field.TypeString, Default: ""},
		{Name: "release_note_detail", Type: field.TypeString, Default: "", SchemaType: map[string]string{"mysql": "longtext"}},
		{Name: "created_at", Type: field.TypeTime},
		{Name: "resource_versions", Type: field.TypeString, Nullable: true},
	}
	// VersionsTable holds the schema information for the "versions" table.
	VersionsTable = &schema.Table{
		Name:       "versions",
		Columns:    VersionsColumns,
		PrimaryKey: []*schema.Column{VersionsColumns[0]},
		ForeignKeys: []*schema.ForeignKey{
			{
				Symbol:     "versions_resources_versions",
				Columns:    []*schema.Column{VersionsColumns[7]},
				RefColumns: []*schema.Column{ResourcesColumns[0]},
				OnDelete:   schema.SetNull,
			},
		},
	}
	// Tables holds all the tables in the schema.
	Tables = []*schema.Table{
		LatestVersionsTable,
		ResourcesTable,
		StoragesTable,
		VersionsTable,
	}
)

func init() {
	LatestVersionsTable.ForeignKeys[0].RefTable = VersionsTable
	LatestVersionsTable.ForeignKeys[1].RefTable = ResourcesTable
	StoragesTable.ForeignKeys[0].RefTable = VersionsTable
	StoragesTable.ForeignKeys[1].RefTable = VersionsTable
	VersionsTable.ForeignKeys[0].RefTable = ResourcesTable
}

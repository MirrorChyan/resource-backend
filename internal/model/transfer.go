package model

import (
	"database/sql"
	"time"
)

type VersionInfo struct {
	VersionId         int       `db:"version_id"`
	VersionName       string    `db:"version_name"`
	CreatedAt         time.Time `db:"created_at"`
	Channel           string    `db:"channel"`
	OS                string    `db:"os"`
	Arch              string    `db:"arch"`
	PackageHashSha256 string    `db:"package_hash_sha256"`
	PackagePath       string    `db:"package_path"`
	ResourcePath      string    `db:"resource_path"`
}
type LatestVersionInfo struct {
	VersionId     int            `db:"version_id"`
	VersionName   string         `db:"version_name"`
	VersionNumber uint64         `db:"version_number"`
	ReleaseNote   string         `db:"release_note"`
	CustomData    string         `db:"custom_data"`
	OS            string         `db:"os"`
	Arch          string         `db:"arch"`
	Channel       string         `db:"channel"`
	PackageHash   sql.NullString `db:"package_hash_sha256"`
	PackagePath   sql.NullString `db:"package_path"`
	CreatedAt     time.Time      `db:"created_at"`
	VersionSerial int            `db:"version_serial"`
}

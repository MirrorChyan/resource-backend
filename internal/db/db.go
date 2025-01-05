package db

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
)

func New(conf *config.Config) (*ent.Client, error) {
	return ent.Open("sqlite3", "./data.db")
}

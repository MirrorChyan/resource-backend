package provider

import (
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/google/wire"
)

var RepoSet = wire.NewSet(
	repo.NewRepo,
	repo.NewRawQuery,
	repo.NewResource,
	repo.NewVersion,
	repo.NewStorage,
)

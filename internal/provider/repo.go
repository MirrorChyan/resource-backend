package provider

import (
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/google/wire"
)

var RepoSet = wire.NewSet(
	repo.NewRepo,
	repo.NewResource,
	repo.NewVersion,
	repo.NewLatestVersion,
	repo.NewStorage,
)

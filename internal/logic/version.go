package logic

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"go.uber.org/zap"
)

type VersionLogic struct {
	logger *zap.Logger
	db     *ent.Client
}

func NewVersionLogic(logger *zap.Logger, db *ent.Client) *VersionLogic {
	return &VersionLogic{
		logger: logger,
		db:     db,
	}
}

func (l *VersionLogic) GetLatest(ctx context.Context, resourceID int) (*ent.Version, error) {
	return l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resourceID))).
		Order(ent.Desc("number")).
		First(ctx)
}

type CreateVersionParam struct {
	ResourceID  int
	Name        string
	ResourceDir string
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	var number uint64
	latest, err := l.GetLatest(ctx, param.ResourceID)
	if err != nil {
		// todo: handle other error
		// no version yet
		number = 1
	}
	if latest != nil {
		number = latest.Number + 1
	}

	fileHashes, err := filehash.GetAll(param.ResourceDir)
	if err != nil {
		return nil, err
	}
	v, err := l.db.Version.Create().
		SetResourceID(param.ResourceID).
		SetName(param.Name).
		SetNumber(number).
		SetFileHashes(fileHashes).
		Save(ctx)
	return v, err
}

type ListVersionParam struct {
	ResourceID int
	Offset     int
	Limit      int
}

func (l *VersionLogic) List(ctx context.Context, param ListVersionParam) (int, []*ent.Version, error) {
	query := l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(param.ResourceID)))

	count, err := query.Count(ctx)
	if err != nil {
		return 0, nil, err
	}

	versions, err := query.
		Offset(param.Offset).
		Limit(param.Limit).
		Order(ent.Desc("number")).
		All(ctx)
	if err != nil {
		return 0, nil, err
	}

	return count, versions, nil
}

type GetVersionByNameParam struct {
	ResourceID int
	Name       string
}

func (l *VersionLogic) GetByName(ctx context.Context, param GetVersionByNameParam) (*ent.Version, error) {
	return l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(param.ResourceID)), version.Name(param.Name)).
		First(ctx)
}

func (l *VersionLogic) Delete(ctx context.Context, id int) error {
	return l.db.Version.
		DeleteOneID(id).
		Exec(ctx)
}

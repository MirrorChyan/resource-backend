package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/latestversion"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
)

type LatestVersion struct {
	db *ent.Client
}

func NewLatestVersion(db *ent.Client) *LatestVersion {
	return &LatestVersion{
		db: db,
	}
}

func (r *LatestVersion) UpsertLatestVersion(ctx context.Context, tx *ent.Tx, resID string, channel latestversion.Channel, ver *ent.Version) error {
	return tx.LatestVersion.Create().
		SetResourceID(resID).
		SetChannel(channel).
		SetVersion(ver).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
}

func (r *LatestVersion) GetLatestStableVersion(ctx context.Context, tx *ent.Tx, resID string) (*ent.Version, error) {
	return r.getLatestVersionByChannel(ctx, tx, resID, latestversion.ChannelStable)
}

func (r *LatestVersion) GetLatestStableVersionWithoutTx(ctx context.Context, resID string) (*ent.Version, error) {
	return r.getLatestVersionByChannel(ctx, nil, resID, latestversion.ChannelStable)
}

func (r *LatestVersion) GetLatestBetaVersion(ctx context.Context, tx *ent.Tx, resID string) (*ent.Version, error) {
	return r.getLatestVersionByChannel(ctx, tx, resID, latestversion.ChannelBeta)
}

func (r *LatestVersion) GetLatestBetaVersionWithoutTx(ctx context.Context, resID string) (*ent.Version, error) {
	return r.getLatestVersionByChannel(ctx, nil, resID, latestversion.ChannelBeta)
}

func (r *LatestVersion) GetLatestAlphaVersion(ctx context.Context, tx *ent.Tx, resID string) (*ent.Version, error) {
	return r.getLatestVersionByChannel(ctx, tx, resID, latestversion.ChannelAlpha)
}

func (r *LatestVersion) GetLatestAlphaVersionWithoutTx(ctx context.Context, resID string) (*ent.Version, error) {
	return r.getLatestVersionByChannel(ctx, nil, resID, latestversion.ChannelAlpha)
}

func (r *LatestVersion) getLatestVersionByChannel(
	ctx context.Context,
	tx *ent.Tx,
	resID string,
	channel latestversion.Channel,
) (*ent.Version, error) {
	queryClient := r.db.LatestVersion.Query()
	if tx != nil {
		queryClient = tx.LatestVersion.Query()
	}

	lv, err := queryClient.
		Where(
			latestversion.HasResourceWith(resource.ID(resID)),
			latestversion.ChannelEQ(channel),
			latestversion.HasVersion(),
		).
		WithVersion().
		Only(ctx)

	if err != nil {
		return nil, err
	}
	return lv.Edges.Version, nil
}

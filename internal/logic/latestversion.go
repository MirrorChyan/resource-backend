package logic

import (
	"context"
	"errors"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/latestversion"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/MirrorChyan/resource-backend/internal/vercomp"
	"go.uber.org/zap"
)

type LatestVersionLogic struct {
	logger            *zap.Logger
	latestVersionRepo *repo.LatestVersion
	verComparator     *vercomp.VersionComparator
}

func NewLatestVersionLogic(
	logger *zap.Logger,
	latestVersionRepo *repo.LatestVersion,
	verComparator *vercomp.VersionComparator,
) *LatestVersionLogic {
	return &LatestVersionLogic{
		logger:            logger,
		latestVersionRepo: latestVersionRepo,
		verComparator:     verComparator,
	}
}

func (l *LatestVersionLogic) UpdateLatestVersion(ctx context.Context, tx *ent.Tx, resID string, channel latestversion.Channel, ver *ent.Version) error {
	switch channel {
	case latestversion.ChannelStable:
		return l.updateLatestStableVersion(ctx, tx, resID, ver)
	case latestversion.ChannelBeta:
		return l.updateLatestBetaVersion(ctx, tx, resID, ver)
	case latestversion.ChannelAlpha:
		return l.updateLatestAlphaVersion(ctx, tx, resID, ver)
	default:
		return l.updateLatestStableVersion(ctx, tx, resID, ver)
	}
}

func (l *LatestVersionLogic) updateLatestStableVersion(ctx context.Context, tx *ent.Tx, resID string, stableVer *ent.Version) error {
	err := l.latestVersionRepo.UpsertLatestVersion(ctx, tx, resID, latestversion.ChannelStable, stableVer)
	if err != nil {
		l.logger.Error("Failed to upsert latest stable version",
			zap.String("resource id", resID),
			zap.String("channel", latestversion.ChannelStable.String()),
			zap.String("version name", stableVer.Name),
			zap.Error(err),
		)
		return err
	}

	betaVer, err := l.latestVersionRepo.GetLatestBetaVersion(ctx, tx, resID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}

		l.logger.Error("Failed to get latest beta version",
			zap.String("resource id", resID),
			zap.Error(err),
		)
		return err
	}

	ret := l.verComparator.Compare(stableVer.Name, betaVer.Name)
	if !ret.Comparable {
		err = errors.New("Failed to compare versions")
		l.logger.Error("Failed to compare versions",
			zap.Int("stable version id", stableVer.ID),
			zap.String("stable version name", stableVer.Name),
			zap.String("resource id", resID),
			zap.Int("beta version id", betaVer.ID),
			zap.String("beta version name", betaVer.Name),
			zap.Error(err),
		)
		return err
	}

	if ret.Result == vercomp.Less {
		return nil
	}

	err = l.updateLatestBetaVersion(ctx, tx, resID, stableVer)
	if err != nil {
		l.logger.Error("Failed to upsert latest beta version",
			zap.String("resource id", resID),
			zap.String("channel", latestversion.ChannelBeta.String()),
			zap.String("version name", stableVer.Name),
			zap.Error(err),
		)
		return err
	}

	return nil
}

func (l *LatestVersionLogic) updateLatestBetaVersion(ctx context.Context, tx *ent.Tx, resID string, betaVer *ent.Version) error {
	err := l.latestVersionRepo.UpsertLatestVersion(ctx, tx, resID, latestversion.ChannelBeta, betaVer)
	if err != nil {
		l.logger.Error("Failed to upsert latest beta version",
			zap.String("resource id", resID),
			zap.String("channel", latestversion.ChannelBeta.String()),
			zap.String("version name", betaVer.Name),
			zap.Error(err),
		)
		return err
	}

	alphaVer, err := l.latestVersionRepo.GetLatestAlphaVersion(ctx, tx, resID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}

		l.logger.Error("Failed to get latest alpha version",
			zap.String("resource id", resID),
			zap.Error(err),
		)
		return err
	}

	ret := l.verComparator.Compare(betaVer.Name, alphaVer.Name)
	if !ret.Comparable {
		err = errors.New("Failed to compare versions")
		l.logger.Error("Failed to compare versions",
			zap.String("resource id", resID),
			zap.Int("beta version id", betaVer.ID),
			zap.String("beta version name", betaVer.Name),
			zap.Int("alpha version id", alphaVer.ID),
			zap.String("alpha version name", alphaVer.Name),
			zap.Error(err),
		)
		return err
	}

	if ret.Result == vercomp.Less {
		return nil
	}

	err = l.updateLatestAlphaVersion(ctx, tx, resID, betaVer)
	if err != nil {
		l.logger.Error("Failed to upsert latest alpha version",
			zap.String("resource id", resID),
			zap.String("channel", latestversion.ChannelAlpha.String()),
			zap.String("version name", betaVer.Name),
			zap.Error(err),
		)
		return err
	}

	return nil
}

func (l *LatestVersionLogic) updateLatestAlphaVersion(ctx context.Context, tx *ent.Tx, resID string, alphaVer *ent.Version) error {
	err := l.latestVersionRepo.UpsertLatestVersion(ctx, tx, resID, latestversion.ChannelAlpha, alphaVer)
	if err != nil {
		l.logger.Error("Failed to upsert latest alpha version",
			zap.String("resource id", resID),
			zap.String("channel", latestversion.ChannelAlpha.String()),
			zap.String("version name", alphaVer.Name),
			zap.Error(err),
		)
		return err
	}

	return nil
}

func (l *LatestVersionLogic) GetLatestStableVersion(ctx context.Context, resID string) (*ent.Version, error) {
	return l.latestVersionRepo.GetLatestStableVersionWithoutTx(ctx, resID)
}

func (l *LatestVersionLogic) GetLatestBetaVersion(ctx context.Context, resID string) (*ent.Version, error) {
	return l.latestVersionRepo.GetLatestBetaVersionWithoutTx(ctx, resID)
}

func (l *LatestVersionLogic) GetLatestAlphaVersion(ctx context.Context, resID string) (*ent.Version, error) {
	return l.latestVersionRepo.GetLatestAlphaVersionWithoutTx(ctx, resID)
}

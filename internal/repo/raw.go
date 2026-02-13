package repo

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"go.uber.org/zap"
)

type RawQuery struct {
	*Repo
}

func NewRawQuery(db *Repo) *RawQuery {
	return &RawQuery{
		Repo: db,
	}
}

const (
	sql1 = `
with latest as (select v.id                                                                         as version_id,
                       v.name                                                                       as version_name,
                       v.number                                                                     as version_number,
                       v.release_note                                                               as release_note,
                       v.custom_data                                                                as custom_data,
                       v.channel                                                                    as channel,
                       s.os                                                                         as os,
                       s.arch                                                                       as arch,
                       s.package_hash_sha256                                                        as package_hash_sha256,
                       s.package_path                                                               as package_path,
                       v.created_at                                                                 as created_at,
                       row_number() over (partition by channel,os,arch order by s.created_at desc ) as version_serial
                from versions v
                         left join storages s on v.id = s.version_storages
                where s.package_path is not null
                  and v.resource_versions = ?
                  and s.os = ?
                  and s.arch = ?
                  and s.update_type = 'full')
select *
from latest
where latest.version_serial = 1
`
	sql2 = `
with latest as (select v.name                                                                       as version_name,
                       v.channel                                                                    as channel,
                       v.resource_versions                                                          as resource_id,
                       v.id                                                                         as version_id,
                       s.os                                                                         as os,
                       s.arch                                                                       as arch,
                       s.id                                                                         as storage_id,
                       row_number() over (partition by channel,os,arch order by s.created_at desc ) as version_serial
                from versions v
                         left join storages s on v.id = s.version_storages
                where s.package_path is not null
                  and v.resource_versions = ?
                  and s.update_type = 'full')
select *
from latest
where latest.version_serial not in (1, 2)
`
)

func (r *RawQuery) GetSpecifiedLatestVersion(resourceId, os, arch string) ([]model.LatestVersionInfo, error) {
	var result []model.LatestVersionInfo
	if config.GConfig.Extra.SqlDebugMode {
		zap.L().Info("GetSpecifiedLatestVersion",
			zap.String("resource id", resourceId),
			zap.String("os", os),
			zap.String("arch", arch),
		)
	}
	err := r.dx.Select(&result, sql1, resourceId, os, arch)
	if err != nil {
		zap.L().Error("GetSpecifiedLatestVersion",
			zap.String("resource id", resourceId),
			zap.String("os", os),
			zap.String("arch", arch),
			zap.Error(err),
		)
		return nil, err
	}
	return result, err
}

func (r *RawQuery) GetReadyToPurgeInfo(resourceId string) ([]model.ResourcePurgeInfo, error) {
	if len(resourceId) == 0 {
		return nil, nil
	}
	var result []model.ResourcePurgeInfo
	if config.GConfig.Extra.SqlDebugMode {
		zap.L().Info("GetReadyToPurgeInfo",
			zap.String("resourceId", resourceId),
		)
	}
	err := r.dx.Select(&result, sql2, resourceId)
	if err != nil {
		zap.L().Error("GetReadyToPurgeInfo",
			zap.String("resourceId", resourceId),
			zap.Error(err),
		)
		return nil, err
	}

	return result, err
}

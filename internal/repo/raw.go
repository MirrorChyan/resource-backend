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
select id                  as version_id,
       name                as version_name,
       number              as version_number,
       release_note        as release_note,
       custom_data         as custom_data,
       os                  as os,
       arch                as arch,
       channel             as channel,
       package_hash_sha256 as package_hash_sha256,
       package_path        as package_path,
       created_at          as created_at,
       vs                  as version_serial
from (select t.*,
             row_number() over (partition by channel,os,arch order by vs) as rn
      from (select lv.*,
                   s.os,
                   s.arch,
                   s.package_hash_sha256,
                   s.package_path
            from (select row_number() over (partition by channel order by created_at desc ) as vs,
                         v.id,
                         v.created_at,
                         v.name,
                         v.number,
                         v.channel,
                         v.release_note,
                         v.custom_data
                  from versions v
                  where resource_versions = ?) lv
                     left join storages s on lv.id = s.version_storages
            where vs between 1 and 100
              and s.update_type = 'full'
              and s.os = ?
              and s.arch = ?)
               as t) t2
where rn = 1
`
	sql2 = `select name              as version_name,
       channel           as channel,
       resource_versions as resource_id,
       id                as version_id,
       os                as os,
       arch              as arch,
       sid               as storage_id,
       vs                as version_serial
from (select t.*,
             row_number() over (partition by channel,os,arch order by vs) as rn
      from (select lv.*,
                   s.os,
                   s.arch,
                   s.id as sid,
                   s.update_type
            from (select row_number() over (partition by channel order by created_at desc ) as vs,
                         v.id,
                         v.name,
                         v.channel,
                         v.resource_versions
                  from versions v
                  where resource_versions = ?) lv
                     left join storages s on lv.id = s.version_storages
            where vs between 1 and 100
              and s.update_type = 'full')
               as t) t2
where rn not in (1, 2)`
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

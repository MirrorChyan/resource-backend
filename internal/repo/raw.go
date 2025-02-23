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
            where vs between 1 and 5
              and s.update_type = 'full'
              and s.os = ?
              and s.arch = ?)
               as t) t2
where rn = 1
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
	return result, err
}

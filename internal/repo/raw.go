package repo

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/model"
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
	err := r.dx.Select(&result, sql1, resourceId, os, arch)
	return result, err
}

func (r *RawQuery) CreateNewVersionTx(ctx context.Context, resourceId, name, channel string) error {
	tx, err := r.dx.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		switch p := recover(); {
		case p != nil:
			_ = tx.Rollback()
		}
	}()
	return nil
}

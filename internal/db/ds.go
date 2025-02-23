package db

import (
	"entgo.io/ent/dialect/sql"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/config"
	s "github.com/MirrorChyan/resource-backend/internal/db/sql"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const DriverName = "mysql"

func LoadDataSource() (*ent.Client, *sqlx.DB, error) {
	drv, err := NewDataSource()
	if err != nil {
		return nil, nil, err
	}
	client, err := s.NewEntClient(drv)
	if err != nil {
		return nil, nil, err
	}
	return client, s.NewSqlx(drv), nil
}

func NewDataSource() (*sql.Driver, error) {
	var (
		conf = config.GConfig
	)
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=True",
		conf.Database.Username,
		conf.Database.Password,
		conf.Database.Host,
		conf.Database.Port,
		conf.Database.Name,
	)
	drv, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, err
	}
	return drv, nil
}

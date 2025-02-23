package sql

import (
	"entgo.io/ent/dialect/sql"
	"github.com/jmoiron/sqlx"
)

const DriverName = "mysql"

func NewSqlx(drv *sql.Driver) *sqlx.DB {
	return sqlx.NewDb(drv.DB(), DriverName)
}

package sql

import (
	"entgo.io/ent/dialect/sql"
	"github.com/jmoiron/sqlx"
)

// NewSqlx shares the connection pool of the ent driver; the dialect names the bind
// variables sqlx uses.
func NewSqlx(drv *sql.Driver) *sqlx.DB {
	return sqlx.NewDb(drv.DB(), drv.Dialect())
}

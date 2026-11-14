package db

import (
	stdsql "database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/config"
	s "github.com/MirrorChyan/resource-backend/internal/db/sql"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const (
	DriverName = "mysql"
	// DriverSQLite runs the service on a local file, for development without MySQL
	DriverSQLite = "sqlite"
)

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
	switch conf.Database.Driver {
	case "", DriverName:
		return newMySQL(conf.Database)
	case DriverSQLite:
		return newSQLite(conf.Database)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", conf.Database.Driver)
	}
}

func newMySQL(conf config.DatabaseConfig) (*sql.Driver, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=True",
		conf.Username,
		conf.Password,
		conf.Host,
		conf.Port,
		conf.Name,
	)
	drv, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, err
	}
	db := drv.DB()
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(50)
	db.SetConnMaxLifetime(25 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return drv, nil
}

func newSQLite(conf config.DatabaseConfig) (*sql.Driver, error) {
	if conf.Path == "" {
		return nil, errors.New("database.path is required for the sqlite driver")
	}
	// ent needs foreign keys; WAL lets reads run beside a write
	dsn := "file:" + filepath.ToSlash(conf.Path) +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"
	db, err := stdsql.Open(DriverSQLite, dsn)
	if err != nil {
		return nil, err
	}
	// sqlite has one writer: a single connection makes the handlers and the task workers
	// queue for it instead of failing with "database is locked"
	db.SetMaxOpenConns(1)
	return sql.OpenDB(dialect.SQLite, db), nil
}

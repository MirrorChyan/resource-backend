package repo

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"

	"github.com/MirrorChyan/resource-backend/internal/ent"
)

type Repo struct {
	db *ent.Client
	dx *sqlx.DB
}

func NewRepo(db *ent.Client, dx *sqlx.DB) *Repo {
	return &Repo{
		db: db,
		dx: dx,
	}
}

func (r *Repo) WithTx(ctx context.Context, fn func(tx *ent.Tx) error) error {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			err = errors.Join(err, fmt.Errorf("rolling back transaction: %v", rerr))
		}
		return err
	}

	return tx.Commit()
}

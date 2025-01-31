// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

// StorageCreate is the builder for creating a Storage entity.
type StorageCreate struct {
	config
	mutation *StorageMutation
	hooks    []Hook
	conflict []sql.ConflictOption
}

// SetUpdateType sets the "update_type" field.
func (sc *StorageCreate) SetUpdateType(st storage.UpdateType) *StorageCreate {
	sc.mutation.SetUpdateType(st)
	return sc
}

// SetOs sets the "os" field.
func (sc *StorageCreate) SetOs(s string) *StorageCreate {
	sc.mutation.SetOs(s)
	return sc
}

// SetNillableOs sets the "os" field if the given value is not nil.
func (sc *StorageCreate) SetNillableOs(s *string) *StorageCreate {
	if s != nil {
		sc.SetOs(*s)
	}
	return sc
}

// SetArch sets the "arch" field.
func (sc *StorageCreate) SetArch(s string) *StorageCreate {
	sc.mutation.SetArch(s)
	return sc
}

// SetNillableArch sets the "arch" field if the given value is not nil.
func (sc *StorageCreate) SetNillableArch(s *string) *StorageCreate {
	if s != nil {
		sc.SetArch(*s)
	}
	return sc
}

// SetPackagePath sets the "package_path" field.
func (sc *StorageCreate) SetPackagePath(s string) *StorageCreate {
	sc.mutation.SetPackagePath(s)
	return sc
}

// SetResourcePath sets the "resource_path" field.
func (sc *StorageCreate) SetResourcePath(s string) *StorageCreate {
	sc.mutation.SetResourcePath(s)
	return sc
}

// SetNillableResourcePath sets the "resource_path" field if the given value is not nil.
func (sc *StorageCreate) SetNillableResourcePath(s *string) *StorageCreate {
	if s != nil {
		sc.SetResourcePath(*s)
	}
	return sc
}

// SetFileHashes sets the "file_hashes" field.
func (sc *StorageCreate) SetFileHashes(m map[string]string) *StorageCreate {
	sc.mutation.SetFileHashes(m)
	return sc
}

// SetCreatedAt sets the "created_at" field.
func (sc *StorageCreate) SetCreatedAt(t time.Time) *StorageCreate {
	sc.mutation.SetCreatedAt(t)
	return sc
}

// SetNillableCreatedAt sets the "created_at" field if the given value is not nil.
func (sc *StorageCreate) SetNillableCreatedAt(t *time.Time) *StorageCreate {
	if t != nil {
		sc.SetCreatedAt(*t)
	}
	return sc
}

// SetVersionID sets the "version" edge to the Version entity by ID.
func (sc *StorageCreate) SetVersionID(id int) *StorageCreate {
	sc.mutation.SetVersionID(id)
	return sc
}

// SetVersion sets the "version" edge to the Version entity.
func (sc *StorageCreate) SetVersion(v *Version) *StorageCreate {
	return sc.SetVersionID(v.ID)
}

// SetOldVersionID sets the "old_version" edge to the Version entity by ID.
func (sc *StorageCreate) SetOldVersionID(id int) *StorageCreate {
	sc.mutation.SetOldVersionID(id)
	return sc
}

// SetNillableOldVersionID sets the "old_version" edge to the Version entity by ID if the given value is not nil.
func (sc *StorageCreate) SetNillableOldVersionID(id *int) *StorageCreate {
	if id != nil {
		sc = sc.SetOldVersionID(*id)
	}
	return sc
}

// SetOldVersion sets the "old_version" edge to the Version entity.
func (sc *StorageCreate) SetOldVersion(v *Version) *StorageCreate {
	return sc.SetOldVersionID(v.ID)
}

// Mutation returns the StorageMutation object of the builder.
func (sc *StorageCreate) Mutation() *StorageMutation {
	return sc.mutation
}

// Save creates the Storage in the database.
func (sc *StorageCreate) Save(ctx context.Context) (*Storage, error) {
	sc.defaults()
	return withHooks(ctx, sc.sqlSave, sc.mutation, sc.hooks)
}

// SaveX calls Save and panics if Save returns an error.
func (sc *StorageCreate) SaveX(ctx context.Context) *Storage {
	v, err := sc.Save(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

// Exec executes the query.
func (sc *StorageCreate) Exec(ctx context.Context) error {
	_, err := sc.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (sc *StorageCreate) ExecX(ctx context.Context) {
	if err := sc.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (sc *StorageCreate) defaults() {
	if _, ok := sc.mutation.CreatedAt(); !ok {
		v := storage.DefaultCreatedAt
		sc.mutation.SetCreatedAt(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (sc *StorageCreate) check() error {
	if _, ok := sc.mutation.UpdateType(); !ok {
		return &ValidationError{Name: "update_type", err: errors.New(`ent: missing required field "Storage.update_type"`)}
	}
	if v, ok := sc.mutation.UpdateType(); ok {
		if err := storage.UpdateTypeValidator(v); err != nil {
			return &ValidationError{Name: "update_type", err: fmt.Errorf(`ent: validator failed for field "Storage.update_type": %w`, err)}
		}
	}
	if _, ok := sc.mutation.PackagePath(); !ok {
		return &ValidationError{Name: "package_path", err: errors.New(`ent: missing required field "Storage.package_path"`)}
	}
	if v, ok := sc.mutation.PackagePath(); ok {
		if err := storage.PackagePathValidator(v); err != nil {
			return &ValidationError{Name: "package_path", err: fmt.Errorf(`ent: validator failed for field "Storage.package_path": %w`, err)}
		}
	}
	if _, ok := sc.mutation.CreatedAt(); !ok {
		return &ValidationError{Name: "created_at", err: errors.New(`ent: missing required field "Storage.created_at"`)}
	}
	if len(sc.mutation.VersionIDs()) == 0 {
		return &ValidationError{Name: "version", err: errors.New(`ent: missing required edge "Storage.version"`)}
	}
	return nil
}

func (sc *StorageCreate) sqlSave(ctx context.Context) (*Storage, error) {
	if err := sc.check(); err != nil {
		return nil, err
	}
	_node, _spec := sc.createSpec()
	if err := sqlgraph.CreateNode(ctx, sc.driver, _spec); err != nil {
		if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return nil, err
	}
	id := _spec.ID.Value.(int64)
	_node.ID = int(id)
	sc.mutation.id = &_node.ID
	sc.mutation.done = true
	return _node, nil
}

func (sc *StorageCreate) createSpec() (*Storage, *sqlgraph.CreateSpec) {
	var (
		_node = &Storage{config: sc.config}
		_spec = sqlgraph.NewCreateSpec(storage.Table, sqlgraph.NewFieldSpec(storage.FieldID, field.TypeInt))
	)
	_spec.OnConflict = sc.conflict
	if value, ok := sc.mutation.UpdateType(); ok {
		_spec.SetField(storage.FieldUpdateType, field.TypeEnum, value)
		_node.UpdateType = value
	}
	if value, ok := sc.mutation.Os(); ok {
		_spec.SetField(storage.FieldOs, field.TypeString, value)
		_node.Os = value
	}
	if value, ok := sc.mutation.Arch(); ok {
		_spec.SetField(storage.FieldArch, field.TypeString, value)
		_node.Arch = value
	}
	if value, ok := sc.mutation.PackagePath(); ok {
		_spec.SetField(storage.FieldPackagePath, field.TypeString, value)
		_node.PackagePath = value
	}
	if value, ok := sc.mutation.ResourcePath(); ok {
		_spec.SetField(storage.FieldResourcePath, field.TypeString, value)
		_node.ResourcePath = value
	}
	if value, ok := sc.mutation.FileHashes(); ok {
		_spec.SetField(storage.FieldFileHashes, field.TypeJSON, value)
		_node.FileHashes = value
	}
	if value, ok := sc.mutation.CreatedAt(); ok {
		_spec.SetField(storage.FieldCreatedAt, field.TypeTime, value)
		_node.CreatedAt = value
	}
	if nodes := sc.mutation.VersionIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.M2O,
			Inverse: true,
			Table:   storage.VersionTable,
			Columns: []string{storage.VersionColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(version.FieldID, field.TypeInt),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_node.version_storages = &nodes[0]
		_spec.Edges = append(_spec.Edges, edge)
	}
	if nodes := sc.mutation.OldVersionIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.M2O,
			Inverse: false,
			Table:   storage.OldVersionTable,
			Columns: []string{storage.OldVersionColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(version.FieldID, field.TypeInt),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_node.storage_old_version = &nodes[0]
		_spec.Edges = append(_spec.Edges, edge)
	}
	return _node, _spec
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.Storage.Create().
//		SetUpdateType(v).
//		OnConflict(
//			// Update the row with the new values
//			// the was proposed for insertion.
//			sql.ResolveWithNewValues(),
//		).
//		// Override some of the fields with custom
//		// update values.
//		Update(func(u *ent.StorageUpsert) {
//			SetUpdateType(v+v).
//		}).
//		Exec(ctx)
func (sc *StorageCreate) OnConflict(opts ...sql.ConflictOption) *StorageUpsertOne {
	sc.conflict = opts
	return &StorageUpsertOne{
		create: sc,
	}
}

// OnConflictColumns calls `OnConflict` and configures the columns
// as conflict target. Using this option is equivalent to using:
//
//	client.Storage.Create().
//		OnConflict(sql.ConflictColumns(columns...)).
//		Exec(ctx)
func (sc *StorageCreate) OnConflictColumns(columns ...string) *StorageUpsertOne {
	sc.conflict = append(sc.conflict, sql.ConflictColumns(columns...))
	return &StorageUpsertOne{
		create: sc,
	}
}

type (
	// StorageUpsertOne is the builder for "upsert"-ing
	//  one Storage node.
	StorageUpsertOne struct {
		create *StorageCreate
	}

	// StorageUpsert is the "OnConflict" setter.
	StorageUpsert struct {
		*sql.UpdateSet
	}
)

// SetUpdateType sets the "update_type" field.
func (u *StorageUpsert) SetUpdateType(v storage.UpdateType) *StorageUpsert {
	u.Set(storage.FieldUpdateType, v)
	return u
}

// UpdateUpdateType sets the "update_type" field to the value that was provided on create.
func (u *StorageUpsert) UpdateUpdateType() *StorageUpsert {
	u.SetExcluded(storage.FieldUpdateType)
	return u
}

// SetOs sets the "os" field.
func (u *StorageUpsert) SetOs(v string) *StorageUpsert {
	u.Set(storage.FieldOs, v)
	return u
}

// UpdateOs sets the "os" field to the value that was provided on create.
func (u *StorageUpsert) UpdateOs() *StorageUpsert {
	u.SetExcluded(storage.FieldOs)
	return u
}

// ClearOs clears the value of the "os" field.
func (u *StorageUpsert) ClearOs() *StorageUpsert {
	u.SetNull(storage.FieldOs)
	return u
}

// SetArch sets the "arch" field.
func (u *StorageUpsert) SetArch(v string) *StorageUpsert {
	u.Set(storage.FieldArch, v)
	return u
}

// UpdateArch sets the "arch" field to the value that was provided on create.
func (u *StorageUpsert) UpdateArch() *StorageUpsert {
	u.SetExcluded(storage.FieldArch)
	return u
}

// ClearArch clears the value of the "arch" field.
func (u *StorageUpsert) ClearArch() *StorageUpsert {
	u.SetNull(storage.FieldArch)
	return u
}

// SetPackagePath sets the "package_path" field.
func (u *StorageUpsert) SetPackagePath(v string) *StorageUpsert {
	u.Set(storage.FieldPackagePath, v)
	return u
}

// UpdatePackagePath sets the "package_path" field to the value that was provided on create.
func (u *StorageUpsert) UpdatePackagePath() *StorageUpsert {
	u.SetExcluded(storage.FieldPackagePath)
	return u
}

// SetResourcePath sets the "resource_path" field.
func (u *StorageUpsert) SetResourcePath(v string) *StorageUpsert {
	u.Set(storage.FieldResourcePath, v)
	return u
}

// UpdateResourcePath sets the "resource_path" field to the value that was provided on create.
func (u *StorageUpsert) UpdateResourcePath() *StorageUpsert {
	u.SetExcluded(storage.FieldResourcePath)
	return u
}

// ClearResourcePath clears the value of the "resource_path" field.
func (u *StorageUpsert) ClearResourcePath() *StorageUpsert {
	u.SetNull(storage.FieldResourcePath)
	return u
}

// SetFileHashes sets the "file_hashes" field.
func (u *StorageUpsert) SetFileHashes(v map[string]string) *StorageUpsert {
	u.Set(storage.FieldFileHashes, v)
	return u
}

// UpdateFileHashes sets the "file_hashes" field to the value that was provided on create.
func (u *StorageUpsert) UpdateFileHashes() *StorageUpsert {
	u.SetExcluded(storage.FieldFileHashes)
	return u
}

// ClearFileHashes clears the value of the "file_hashes" field.
func (u *StorageUpsert) ClearFileHashes() *StorageUpsert {
	u.SetNull(storage.FieldFileHashes)
	return u
}

// SetCreatedAt sets the "created_at" field.
func (u *StorageUpsert) SetCreatedAt(v time.Time) *StorageUpsert {
	u.Set(storage.FieldCreatedAt, v)
	return u
}

// UpdateCreatedAt sets the "created_at" field to the value that was provided on create.
func (u *StorageUpsert) UpdateCreatedAt() *StorageUpsert {
	u.SetExcluded(storage.FieldCreatedAt)
	return u
}

// UpdateNewValues updates the mutable fields using the new values that were set on create.
// Using this option is equivalent to using:
//
//	client.Storage.Create().
//		OnConflict(
//			sql.ResolveWithNewValues(),
//		).
//		Exec(ctx)
func (u *StorageUpsertOne) UpdateNewValues() *StorageUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithNewValues())
	return u
}

// Ignore sets each column to itself in case of conflict.
// Using this option is equivalent to using:
//
//	client.Storage.Create().
//	    OnConflict(sql.ResolveWithIgnore()).
//	    Exec(ctx)
func (u *StorageUpsertOne) Ignore() *StorageUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING`.
// Supported only by SQLite and PostgreSQL.
func (u *StorageUpsertOne) DoNothing() *StorageUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.DoNothing())
	return u
}

// Update allows overriding fields `UPDATE` values. See the StorageCreate.OnConflict
// documentation for more info.
func (u *StorageUpsertOne) Update(set func(*StorageUpsert)) *StorageUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&StorageUpsert{UpdateSet: update})
	}))
	return u
}

// SetUpdateType sets the "update_type" field.
func (u *StorageUpsertOne) SetUpdateType(v storage.UpdateType) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetUpdateType(v)
	})
}

// UpdateUpdateType sets the "update_type" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdateUpdateType() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateUpdateType()
	})
}

// SetOs sets the "os" field.
func (u *StorageUpsertOne) SetOs(v string) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetOs(v)
	})
}

// UpdateOs sets the "os" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdateOs() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateOs()
	})
}

// ClearOs clears the value of the "os" field.
func (u *StorageUpsertOne) ClearOs() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.ClearOs()
	})
}

// SetArch sets the "arch" field.
func (u *StorageUpsertOne) SetArch(v string) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetArch(v)
	})
}

// UpdateArch sets the "arch" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdateArch() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateArch()
	})
}

// ClearArch clears the value of the "arch" field.
func (u *StorageUpsertOne) ClearArch() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.ClearArch()
	})
}

// SetPackagePath sets the "package_path" field.
func (u *StorageUpsertOne) SetPackagePath(v string) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetPackagePath(v)
	})
}

// UpdatePackagePath sets the "package_path" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdatePackagePath() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdatePackagePath()
	})
}

// SetResourcePath sets the "resource_path" field.
func (u *StorageUpsertOne) SetResourcePath(v string) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetResourcePath(v)
	})
}

// UpdateResourcePath sets the "resource_path" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdateResourcePath() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateResourcePath()
	})
}

// ClearResourcePath clears the value of the "resource_path" field.
func (u *StorageUpsertOne) ClearResourcePath() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.ClearResourcePath()
	})
}

// SetFileHashes sets the "file_hashes" field.
func (u *StorageUpsertOne) SetFileHashes(v map[string]string) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetFileHashes(v)
	})
}

// UpdateFileHashes sets the "file_hashes" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdateFileHashes() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateFileHashes()
	})
}

// ClearFileHashes clears the value of the "file_hashes" field.
func (u *StorageUpsertOne) ClearFileHashes() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.ClearFileHashes()
	})
}

// SetCreatedAt sets the "created_at" field.
func (u *StorageUpsertOne) SetCreatedAt(v time.Time) *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.SetCreatedAt(v)
	})
}

// UpdateCreatedAt sets the "created_at" field to the value that was provided on create.
func (u *StorageUpsertOne) UpdateCreatedAt() *StorageUpsertOne {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateCreatedAt()
	})
}

// Exec executes the query.
func (u *StorageUpsertOne) Exec(ctx context.Context) error {
	if len(u.create.conflict) == 0 {
		return errors.New("ent: missing options for StorageCreate.OnConflict")
	}
	return u.create.Exec(ctx)
}

// ExecX is like Exec, but panics if an error occurs.
func (u *StorageUpsertOne) ExecX(ctx context.Context) {
	if err := u.create.Exec(ctx); err != nil {
		panic(err)
	}
}

// Exec executes the UPSERT query and returns the inserted/updated ID.
func (u *StorageUpsertOne) ID(ctx context.Context) (id int, err error) {
	node, err := u.create.Save(ctx)
	if err != nil {
		return id, err
	}
	return node.ID, nil
}

// IDX is like ID, but panics if an error occurs.
func (u *StorageUpsertOne) IDX(ctx context.Context) int {
	id, err := u.ID(ctx)
	if err != nil {
		panic(err)
	}
	return id
}

// StorageCreateBulk is the builder for creating many Storage entities in bulk.
type StorageCreateBulk struct {
	config
	err      error
	builders []*StorageCreate
	conflict []sql.ConflictOption
}

// Save creates the Storage entities in the database.
func (scb *StorageCreateBulk) Save(ctx context.Context) ([]*Storage, error) {
	if scb.err != nil {
		return nil, scb.err
	}
	specs := make([]*sqlgraph.CreateSpec, len(scb.builders))
	nodes := make([]*Storage, len(scb.builders))
	mutators := make([]Mutator, len(scb.builders))
	for i := range scb.builders {
		func(i int, root context.Context) {
			builder := scb.builders[i]
			builder.defaults()
			var mut Mutator = MutateFunc(func(ctx context.Context, m Mutation) (Value, error) {
				mutation, ok := m.(*StorageMutation)
				if !ok {
					return nil, fmt.Errorf("unexpected mutation type %T", m)
				}
				if err := builder.check(); err != nil {
					return nil, err
				}
				builder.mutation = mutation
				var err error
				nodes[i], specs[i] = builder.createSpec()
				if i < len(mutators)-1 {
					_, err = mutators[i+1].Mutate(root, scb.builders[i+1].mutation)
				} else {
					spec := &sqlgraph.BatchCreateSpec{Nodes: specs}
					spec.OnConflict = scb.conflict
					// Invoke the actual operation on the latest mutation in the chain.
					if err = sqlgraph.BatchCreate(ctx, scb.driver, spec); err != nil {
						if sqlgraph.IsConstraintError(err) {
							err = &ConstraintError{msg: err.Error(), wrap: err}
						}
					}
				}
				if err != nil {
					return nil, err
				}
				mutation.id = &nodes[i].ID
				if specs[i].ID.Value != nil {
					id := specs[i].ID.Value.(int64)
					nodes[i].ID = int(id)
				}
				mutation.done = true
				return nodes[i], nil
			})
			for i := len(builder.hooks) - 1; i >= 0; i-- {
				mut = builder.hooks[i](mut)
			}
			mutators[i] = mut
		}(i, ctx)
	}
	if len(mutators) > 0 {
		if _, err := mutators[0].Mutate(ctx, scb.builders[0].mutation); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

// SaveX is like Save, but panics if an error occurs.
func (scb *StorageCreateBulk) SaveX(ctx context.Context) []*Storage {
	v, err := scb.Save(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

// Exec executes the query.
func (scb *StorageCreateBulk) Exec(ctx context.Context) error {
	_, err := scb.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (scb *StorageCreateBulk) ExecX(ctx context.Context) {
	if err := scb.Exec(ctx); err != nil {
		panic(err)
	}
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.Storage.CreateBulk(builders...).
//		OnConflict(
//			// Update the row with the new values
//			// the was proposed for insertion.
//			sql.ResolveWithNewValues(),
//		).
//		// Override some of the fields with custom
//		// update values.
//		Update(func(u *ent.StorageUpsert) {
//			SetUpdateType(v+v).
//		}).
//		Exec(ctx)
func (scb *StorageCreateBulk) OnConflict(opts ...sql.ConflictOption) *StorageUpsertBulk {
	scb.conflict = opts
	return &StorageUpsertBulk{
		create: scb,
	}
}

// OnConflictColumns calls `OnConflict` and configures the columns
// as conflict target. Using this option is equivalent to using:
//
//	client.Storage.Create().
//		OnConflict(sql.ConflictColumns(columns...)).
//		Exec(ctx)
func (scb *StorageCreateBulk) OnConflictColumns(columns ...string) *StorageUpsertBulk {
	scb.conflict = append(scb.conflict, sql.ConflictColumns(columns...))
	return &StorageUpsertBulk{
		create: scb,
	}
}

// StorageUpsertBulk is the builder for "upsert"-ing
// a bulk of Storage nodes.
type StorageUpsertBulk struct {
	create *StorageCreateBulk
}

// UpdateNewValues updates the mutable fields using the new values that
// were set on create. Using this option is equivalent to using:
//
//	client.Storage.Create().
//		OnConflict(
//			sql.ResolveWithNewValues(),
//		).
//		Exec(ctx)
func (u *StorageUpsertBulk) UpdateNewValues() *StorageUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithNewValues())
	return u
}

// Ignore sets each column to itself in case of conflict.
// Using this option is equivalent to using:
//
//	client.Storage.Create().
//		OnConflict(sql.ResolveWithIgnore()).
//		Exec(ctx)
func (u *StorageUpsertBulk) Ignore() *StorageUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING`.
// Supported only by SQLite and PostgreSQL.
func (u *StorageUpsertBulk) DoNothing() *StorageUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.DoNothing())
	return u
}

// Update allows overriding fields `UPDATE` values. See the StorageCreateBulk.OnConflict
// documentation for more info.
func (u *StorageUpsertBulk) Update(set func(*StorageUpsert)) *StorageUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&StorageUpsert{UpdateSet: update})
	}))
	return u
}

// SetUpdateType sets the "update_type" field.
func (u *StorageUpsertBulk) SetUpdateType(v storage.UpdateType) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetUpdateType(v)
	})
}

// UpdateUpdateType sets the "update_type" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdateUpdateType() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateUpdateType()
	})
}

// SetOs sets the "os" field.
func (u *StorageUpsertBulk) SetOs(v string) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetOs(v)
	})
}

// UpdateOs sets the "os" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdateOs() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateOs()
	})
}

// ClearOs clears the value of the "os" field.
func (u *StorageUpsertBulk) ClearOs() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.ClearOs()
	})
}

// SetArch sets the "arch" field.
func (u *StorageUpsertBulk) SetArch(v string) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetArch(v)
	})
}

// UpdateArch sets the "arch" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdateArch() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateArch()
	})
}

// ClearArch clears the value of the "arch" field.
func (u *StorageUpsertBulk) ClearArch() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.ClearArch()
	})
}

// SetPackagePath sets the "package_path" field.
func (u *StorageUpsertBulk) SetPackagePath(v string) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetPackagePath(v)
	})
}

// UpdatePackagePath sets the "package_path" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdatePackagePath() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdatePackagePath()
	})
}

// SetResourcePath sets the "resource_path" field.
func (u *StorageUpsertBulk) SetResourcePath(v string) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetResourcePath(v)
	})
}

// UpdateResourcePath sets the "resource_path" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdateResourcePath() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateResourcePath()
	})
}

// ClearResourcePath clears the value of the "resource_path" field.
func (u *StorageUpsertBulk) ClearResourcePath() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.ClearResourcePath()
	})
}

// SetFileHashes sets the "file_hashes" field.
func (u *StorageUpsertBulk) SetFileHashes(v map[string]string) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetFileHashes(v)
	})
}

// UpdateFileHashes sets the "file_hashes" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdateFileHashes() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateFileHashes()
	})
}

// ClearFileHashes clears the value of the "file_hashes" field.
func (u *StorageUpsertBulk) ClearFileHashes() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.ClearFileHashes()
	})
}

// SetCreatedAt sets the "created_at" field.
func (u *StorageUpsertBulk) SetCreatedAt(v time.Time) *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.SetCreatedAt(v)
	})
}

// UpdateCreatedAt sets the "created_at" field to the value that was provided on create.
func (u *StorageUpsertBulk) UpdateCreatedAt() *StorageUpsertBulk {
	return u.Update(func(s *StorageUpsert) {
		s.UpdateCreatedAt()
	})
}

// Exec executes the query.
func (u *StorageUpsertBulk) Exec(ctx context.Context) error {
	if u.create.err != nil {
		return u.create.err
	}
	for i, b := range u.create.builders {
		if len(b.conflict) != 0 {
			return fmt.Errorf("ent: OnConflict was set for builder %d. Set it on the StorageCreateBulk instead", i)
		}
	}
	if len(u.create.conflict) == 0 {
		return errors.New("ent: missing options for StorageCreateBulk.OnConflict")
	}
	return u.create.Exec(ctx)
}

// ExecX is like Exec, but panics if an error occurs.
func (u *StorageUpsertBulk) ExecX(ctx context.Context) {
	if err := u.create.Exec(ctx); err != nil {
		panic(err)
	}
}

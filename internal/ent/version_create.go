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
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

// VersionCreate is the builder for creating a Version entity.
type VersionCreate struct {
	config
	mutation *VersionMutation
	hooks    []Hook
	conflict []sql.ConflictOption
}

// SetChannel sets the "channel" field.
func (vc *VersionCreate) SetChannel(v version.Channel) *VersionCreate {
	vc.mutation.SetChannel(v)
	return vc
}

// SetNillableChannel sets the "channel" field if the given value is not nil.
func (vc *VersionCreate) SetNillableChannel(v *version.Channel) *VersionCreate {
	if v != nil {
		vc.SetChannel(*v)
	}
	return vc
}

// SetName sets the "name" field.
func (vc *VersionCreate) SetName(s string) *VersionCreate {
	vc.mutation.SetName(s)
	return vc
}

// SetNumber sets the "number" field.
func (vc *VersionCreate) SetNumber(u uint64) *VersionCreate {
	vc.mutation.SetNumber(u)
	return vc
}

// SetReleaseNote sets the "release_note" field.
func (vc *VersionCreate) SetReleaseNote(s string) *VersionCreate {
	vc.mutation.SetReleaseNote(s)
	return vc
}

// SetNillableReleaseNote sets the "release_note" field if the given value is not nil.
func (vc *VersionCreate) SetNillableReleaseNote(s *string) *VersionCreate {
	if s != nil {
		vc.SetReleaseNote(*s)
	}
	return vc
}

// SetCustomData sets the "custom_data" field.
func (vc *VersionCreate) SetCustomData(s string) *VersionCreate {
	vc.mutation.SetCustomData(s)
	return vc
}

// SetNillableCustomData sets the "custom_data" field if the given value is not nil.
func (vc *VersionCreate) SetNillableCustomData(s *string) *VersionCreate {
	if s != nil {
		vc.SetCustomData(*s)
	}
	return vc
}

// SetCreatedAt sets the "created_at" field.
func (vc *VersionCreate) SetCreatedAt(t time.Time) *VersionCreate {
	vc.mutation.SetCreatedAt(t)
	return vc
}

// SetNillableCreatedAt sets the "created_at" field if the given value is not nil.
func (vc *VersionCreate) SetNillableCreatedAt(t *time.Time) *VersionCreate {
	if t != nil {
		vc.SetCreatedAt(*t)
	}
	return vc
}

// AddStorageIDs adds the "storages" edge to the Storage entity by IDs.
func (vc *VersionCreate) AddStorageIDs(ids ...int) *VersionCreate {
	vc.mutation.AddStorageIDs(ids...)
	return vc
}

// AddStorages adds the "storages" edges to the Storage entity.
func (vc *VersionCreate) AddStorages(s ...*Storage) *VersionCreate {
	ids := make([]int, len(s))
	for i := range s {
		ids[i] = s[i].ID
	}
	return vc.AddStorageIDs(ids...)
}

// SetResourceID sets the "resource" edge to the Resource entity by ID.
func (vc *VersionCreate) SetResourceID(id string) *VersionCreate {
	vc.mutation.SetResourceID(id)
	return vc
}

// SetNillableResourceID sets the "resource" edge to the Resource entity by ID if the given value is not nil.
func (vc *VersionCreate) SetNillableResourceID(id *string) *VersionCreate {
	if id != nil {
		vc = vc.SetResourceID(*id)
	}
	return vc
}

// SetResource sets the "resource" edge to the Resource entity.
func (vc *VersionCreate) SetResource(r *Resource) *VersionCreate {
	return vc.SetResourceID(r.ID)
}

// Mutation returns the VersionMutation object of the builder.
func (vc *VersionCreate) Mutation() *VersionMutation {
	return vc.mutation
}

// Save creates the Version in the database.
func (vc *VersionCreate) Save(ctx context.Context) (*Version, error) {
	vc.defaults()
	return withHooks(ctx, vc.sqlSave, vc.mutation, vc.hooks)
}

// SaveX calls Save and panics if Save returns an error.
func (vc *VersionCreate) SaveX(ctx context.Context) *Version {
	v, err := vc.Save(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

// Exec executes the query.
func (vc *VersionCreate) Exec(ctx context.Context) error {
	_, err := vc.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (vc *VersionCreate) ExecX(ctx context.Context) {
	if err := vc.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (vc *VersionCreate) defaults() {
	if _, ok := vc.mutation.Channel(); !ok {
		v := version.DefaultChannel
		vc.mutation.SetChannel(v)
	}
	if _, ok := vc.mutation.ReleaseNote(); !ok {
		v := version.DefaultReleaseNote
		vc.mutation.SetReleaseNote(v)
	}
	if _, ok := vc.mutation.CustomData(); !ok {
		v := version.DefaultCustomData
		vc.mutation.SetCustomData(v)
	}
	if _, ok := vc.mutation.CreatedAt(); !ok {
		v := version.DefaultCreatedAt()
		vc.mutation.SetCreatedAt(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (vc *VersionCreate) check() error {
	if _, ok := vc.mutation.Channel(); !ok {
		return &ValidationError{Name: "channel", err: errors.New(`ent: missing required field "Version.channel"`)}
	}
	if v, ok := vc.mutation.Channel(); ok {
		if err := version.ChannelValidator(v); err != nil {
			return &ValidationError{Name: "channel", err: fmt.Errorf(`ent: validator failed for field "Version.channel": %w`, err)}
		}
	}
	if _, ok := vc.mutation.Name(); !ok {
		return &ValidationError{Name: "name", err: errors.New(`ent: missing required field "Version.name"`)}
	}
	if v, ok := vc.mutation.Name(); ok {
		if err := version.NameValidator(v); err != nil {
			return &ValidationError{Name: "name", err: fmt.Errorf(`ent: validator failed for field "Version.name": %w`, err)}
		}
	}
	if _, ok := vc.mutation.Number(); !ok {
		return &ValidationError{Name: "number", err: errors.New(`ent: missing required field "Version.number"`)}
	}
	if _, ok := vc.mutation.ReleaseNote(); !ok {
		return &ValidationError{Name: "release_note", err: errors.New(`ent: missing required field "Version.release_note"`)}
	}
	if _, ok := vc.mutation.CustomData(); !ok {
		return &ValidationError{Name: "custom_data", err: errors.New(`ent: missing required field "Version.custom_data"`)}
	}
	if _, ok := vc.mutation.CreatedAt(); !ok {
		return &ValidationError{Name: "created_at", err: errors.New(`ent: missing required field "Version.created_at"`)}
	}
	return nil
}

func (vc *VersionCreate) sqlSave(ctx context.Context) (*Version, error) {
	if err := vc.check(); err != nil {
		return nil, err
	}
	_node, _spec := vc.createSpec()
	if err := sqlgraph.CreateNode(ctx, vc.driver, _spec); err != nil {
		if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return nil, err
	}
	id := _spec.ID.Value.(int64)
	_node.ID = int(id)
	vc.mutation.id = &_node.ID
	vc.mutation.done = true
	return _node, nil
}

func (vc *VersionCreate) createSpec() (*Version, *sqlgraph.CreateSpec) {
	var (
		_node = &Version{config: vc.config}
		_spec = sqlgraph.NewCreateSpec(version.Table, sqlgraph.NewFieldSpec(version.FieldID, field.TypeInt))
	)
	_spec.OnConflict = vc.conflict
	if value, ok := vc.mutation.Channel(); ok {
		_spec.SetField(version.FieldChannel, field.TypeEnum, value)
		_node.Channel = value
	}
	if value, ok := vc.mutation.Name(); ok {
		_spec.SetField(version.FieldName, field.TypeString, value)
		_node.Name = value
	}
	if value, ok := vc.mutation.Number(); ok {
		_spec.SetField(version.FieldNumber, field.TypeUint64, value)
		_node.Number = value
	}
	if value, ok := vc.mutation.ReleaseNote(); ok {
		_spec.SetField(version.FieldReleaseNote, field.TypeString, value)
		_node.ReleaseNote = value
	}
	if value, ok := vc.mutation.CustomData(); ok {
		_spec.SetField(version.FieldCustomData, field.TypeString, value)
		_node.CustomData = value
	}
	if value, ok := vc.mutation.CreatedAt(); ok {
		_spec.SetField(version.FieldCreatedAt, field.TypeTime, value)
		_node.CreatedAt = value
	}
	if nodes := vc.mutation.StoragesIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   version.StoragesTable,
			Columns: []string{version.StoragesColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(storage.FieldID, field.TypeInt),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_spec.Edges = append(_spec.Edges, edge)
	}
	if nodes := vc.mutation.ResourceIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.M2O,
			Inverse: true,
			Table:   version.ResourceTable,
			Columns: []string{version.ResourceColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(resource.FieldID, field.TypeString),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_node.resource_versions = &nodes[0]
		_spec.Edges = append(_spec.Edges, edge)
	}
	return _node, _spec
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.Version.Create().
//		SetChannel(v).
//		OnConflict(
//			// Update the row with the new values
//			// the was proposed for insertion.
//			sql.ResolveWithNewValues(),
//		).
//		// Override some of the fields with custom
//		// update values.
//		Update(func(u *ent.VersionUpsert) {
//			SetChannel(v+v).
//		}).
//		Exec(ctx)
func (vc *VersionCreate) OnConflict(opts ...sql.ConflictOption) *VersionUpsertOne {
	vc.conflict = opts
	return &VersionUpsertOne{
		create: vc,
	}
}

// OnConflictColumns calls `OnConflict` and configures the columns
// as conflict target. Using this option is equivalent to using:
//
//	client.Version.Create().
//		OnConflict(sql.ConflictColumns(columns...)).
//		Exec(ctx)
func (vc *VersionCreate) OnConflictColumns(columns ...string) *VersionUpsertOne {
	vc.conflict = append(vc.conflict, sql.ConflictColumns(columns...))
	return &VersionUpsertOne{
		create: vc,
	}
}

type (
	// VersionUpsertOne is the builder for "upsert"-ing
	//  one Version node.
	VersionUpsertOne struct {
		create *VersionCreate
	}

	// VersionUpsert is the "OnConflict" setter.
	VersionUpsert struct {
		*sql.UpdateSet
	}
)

// SetChannel sets the "channel" field.
func (u *VersionUpsert) SetChannel(v version.Channel) *VersionUpsert {
	u.Set(version.FieldChannel, v)
	return u
}

// UpdateChannel sets the "channel" field to the value that was provided on create.
func (u *VersionUpsert) UpdateChannel() *VersionUpsert {
	u.SetExcluded(version.FieldChannel)
	return u
}

// SetName sets the "name" field.
func (u *VersionUpsert) SetName(v string) *VersionUpsert {
	u.Set(version.FieldName, v)
	return u
}

// UpdateName sets the "name" field to the value that was provided on create.
func (u *VersionUpsert) UpdateName() *VersionUpsert {
	u.SetExcluded(version.FieldName)
	return u
}

// SetNumber sets the "number" field.
func (u *VersionUpsert) SetNumber(v uint64) *VersionUpsert {
	u.Set(version.FieldNumber, v)
	return u
}

// UpdateNumber sets the "number" field to the value that was provided on create.
func (u *VersionUpsert) UpdateNumber() *VersionUpsert {
	u.SetExcluded(version.FieldNumber)
	return u
}

// AddNumber adds v to the "number" field.
func (u *VersionUpsert) AddNumber(v uint64) *VersionUpsert {
	u.Add(version.FieldNumber, v)
	return u
}

// SetReleaseNote sets the "release_note" field.
func (u *VersionUpsert) SetReleaseNote(v string) *VersionUpsert {
	u.Set(version.FieldReleaseNote, v)
	return u
}

// UpdateReleaseNote sets the "release_note" field to the value that was provided on create.
func (u *VersionUpsert) UpdateReleaseNote() *VersionUpsert {
	u.SetExcluded(version.FieldReleaseNote)
	return u
}

// SetCustomData sets the "custom_data" field.
func (u *VersionUpsert) SetCustomData(v string) *VersionUpsert {
	u.Set(version.FieldCustomData, v)
	return u
}

// UpdateCustomData sets the "custom_data" field to the value that was provided on create.
func (u *VersionUpsert) UpdateCustomData() *VersionUpsert {
	u.SetExcluded(version.FieldCustomData)
	return u
}

// SetCreatedAt sets the "created_at" field.
func (u *VersionUpsert) SetCreatedAt(v time.Time) *VersionUpsert {
	u.Set(version.FieldCreatedAt, v)
	return u
}

// UpdateCreatedAt sets the "created_at" field to the value that was provided on create.
func (u *VersionUpsert) UpdateCreatedAt() *VersionUpsert {
	u.SetExcluded(version.FieldCreatedAt)
	return u
}

// UpdateNewValues updates the mutable fields using the new values that were set on create.
// Using this option is equivalent to using:
//
//	client.Version.Create().
//		OnConflict(
//			sql.ResolveWithNewValues(),
//		).
//		Exec(ctx)
func (u *VersionUpsertOne) UpdateNewValues() *VersionUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithNewValues())
	return u
}

// Ignore sets each column to itself in case of conflict.
// Using this option is equivalent to using:
//
//	client.Version.Create().
//	    OnConflict(sql.ResolveWithIgnore()).
//	    Exec(ctx)
func (u *VersionUpsertOne) Ignore() *VersionUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING`.
// Supported only by SQLite and PostgreSQL.
func (u *VersionUpsertOne) DoNothing() *VersionUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.DoNothing())
	return u
}

// Update allows overriding fields `UPDATE` values. See the VersionCreate.OnConflict
// documentation for more info.
func (u *VersionUpsertOne) Update(set func(*VersionUpsert)) *VersionUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&VersionUpsert{UpdateSet: update})
	}))
	return u
}

// SetChannel sets the "channel" field.
func (u *VersionUpsertOne) SetChannel(v version.Channel) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.SetChannel(v)
	})
}

// UpdateChannel sets the "channel" field to the value that was provided on create.
func (u *VersionUpsertOne) UpdateChannel() *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateChannel()
	})
}

// SetName sets the "name" field.
func (u *VersionUpsertOne) SetName(v string) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.SetName(v)
	})
}

// UpdateName sets the "name" field to the value that was provided on create.
func (u *VersionUpsertOne) UpdateName() *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateName()
	})
}

// SetNumber sets the "number" field.
func (u *VersionUpsertOne) SetNumber(v uint64) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.SetNumber(v)
	})
}

// AddNumber adds v to the "number" field.
func (u *VersionUpsertOne) AddNumber(v uint64) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.AddNumber(v)
	})
}

// UpdateNumber sets the "number" field to the value that was provided on create.
func (u *VersionUpsertOne) UpdateNumber() *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateNumber()
	})
}

// SetReleaseNote sets the "release_note" field.
func (u *VersionUpsertOne) SetReleaseNote(v string) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.SetReleaseNote(v)
	})
}

// UpdateReleaseNote sets the "release_note" field to the value that was provided on create.
func (u *VersionUpsertOne) UpdateReleaseNote() *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateReleaseNote()
	})
}

// SetCustomData sets the "custom_data" field.
func (u *VersionUpsertOne) SetCustomData(v string) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.SetCustomData(v)
	})
}

// UpdateCustomData sets the "custom_data" field to the value that was provided on create.
func (u *VersionUpsertOne) UpdateCustomData() *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateCustomData()
	})
}

// SetCreatedAt sets the "created_at" field.
func (u *VersionUpsertOne) SetCreatedAt(v time.Time) *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.SetCreatedAt(v)
	})
}

// UpdateCreatedAt sets the "created_at" field to the value that was provided on create.
func (u *VersionUpsertOne) UpdateCreatedAt() *VersionUpsertOne {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateCreatedAt()
	})
}

// Exec executes the query.
func (u *VersionUpsertOne) Exec(ctx context.Context) error {
	if len(u.create.conflict) == 0 {
		return errors.New("ent: missing options for VersionCreate.OnConflict")
	}
	return u.create.Exec(ctx)
}

// ExecX is like Exec, but panics if an error occurs.
func (u *VersionUpsertOne) ExecX(ctx context.Context) {
	if err := u.create.Exec(ctx); err != nil {
		panic(err)
	}
}

// Exec executes the UPSERT query and returns the inserted/updated ID.
func (u *VersionUpsertOne) ID(ctx context.Context) (id int, err error) {
	node, err := u.create.Save(ctx)
	if err != nil {
		return id, err
	}
	return node.ID, nil
}

// IDX is like ID, but panics if an error occurs.
func (u *VersionUpsertOne) IDX(ctx context.Context) int {
	id, err := u.ID(ctx)
	if err != nil {
		panic(err)
	}
	return id
}

// VersionCreateBulk is the builder for creating many Version entities in bulk.
type VersionCreateBulk struct {
	config
	err      error
	builders []*VersionCreate
	conflict []sql.ConflictOption
}

// Save creates the Version entities in the database.
func (vcb *VersionCreateBulk) Save(ctx context.Context) ([]*Version, error) {
	if vcb.err != nil {
		return nil, vcb.err
	}
	specs := make([]*sqlgraph.CreateSpec, len(vcb.builders))
	nodes := make([]*Version, len(vcb.builders))
	mutators := make([]Mutator, len(vcb.builders))
	for i := range vcb.builders {
		func(i int, root context.Context) {
			builder := vcb.builders[i]
			builder.defaults()
			var mut Mutator = MutateFunc(func(ctx context.Context, m Mutation) (Value, error) {
				mutation, ok := m.(*VersionMutation)
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
					_, err = mutators[i+1].Mutate(root, vcb.builders[i+1].mutation)
				} else {
					spec := &sqlgraph.BatchCreateSpec{Nodes: specs}
					spec.OnConflict = vcb.conflict
					// Invoke the actual operation on the latest mutation in the chain.
					if err = sqlgraph.BatchCreate(ctx, vcb.driver, spec); err != nil {
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
		if _, err := mutators[0].Mutate(ctx, vcb.builders[0].mutation); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

// SaveX is like Save, but panics if an error occurs.
func (vcb *VersionCreateBulk) SaveX(ctx context.Context) []*Version {
	v, err := vcb.Save(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

// Exec executes the query.
func (vcb *VersionCreateBulk) Exec(ctx context.Context) error {
	_, err := vcb.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (vcb *VersionCreateBulk) ExecX(ctx context.Context) {
	if err := vcb.Exec(ctx); err != nil {
		panic(err)
	}
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.Version.CreateBulk(builders...).
//		OnConflict(
//			// Update the row with the new values
//			// the was proposed for insertion.
//			sql.ResolveWithNewValues(),
//		).
//		// Override some of the fields with custom
//		// update values.
//		Update(func(u *ent.VersionUpsert) {
//			SetChannel(v+v).
//		}).
//		Exec(ctx)
func (vcb *VersionCreateBulk) OnConflict(opts ...sql.ConflictOption) *VersionUpsertBulk {
	vcb.conflict = opts
	return &VersionUpsertBulk{
		create: vcb,
	}
}

// OnConflictColumns calls `OnConflict` and configures the columns
// as conflict target. Using this option is equivalent to using:
//
//	client.Version.Create().
//		OnConflict(sql.ConflictColumns(columns...)).
//		Exec(ctx)
func (vcb *VersionCreateBulk) OnConflictColumns(columns ...string) *VersionUpsertBulk {
	vcb.conflict = append(vcb.conflict, sql.ConflictColumns(columns...))
	return &VersionUpsertBulk{
		create: vcb,
	}
}

// VersionUpsertBulk is the builder for "upsert"-ing
// a bulk of Version nodes.
type VersionUpsertBulk struct {
	create *VersionCreateBulk
}

// UpdateNewValues updates the mutable fields using the new values that
// were set on create. Using this option is equivalent to using:
//
//	client.Version.Create().
//		OnConflict(
//			sql.ResolveWithNewValues(),
//		).
//		Exec(ctx)
func (u *VersionUpsertBulk) UpdateNewValues() *VersionUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithNewValues())
	return u
}

// Ignore sets each column to itself in case of conflict.
// Using this option is equivalent to using:
//
//	client.Version.Create().
//		OnConflict(sql.ResolveWithIgnore()).
//		Exec(ctx)
func (u *VersionUpsertBulk) Ignore() *VersionUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING`.
// Supported only by SQLite and PostgreSQL.
func (u *VersionUpsertBulk) DoNothing() *VersionUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.DoNothing())
	return u
}

// Update allows overriding fields `UPDATE` values. See the VersionCreateBulk.OnConflict
// documentation for more info.
func (u *VersionUpsertBulk) Update(set func(*VersionUpsert)) *VersionUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&VersionUpsert{UpdateSet: update})
	}))
	return u
}

// SetChannel sets the "channel" field.
func (u *VersionUpsertBulk) SetChannel(v version.Channel) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.SetChannel(v)
	})
}

// UpdateChannel sets the "channel" field to the value that was provided on create.
func (u *VersionUpsertBulk) UpdateChannel() *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateChannel()
	})
}

// SetName sets the "name" field.
func (u *VersionUpsertBulk) SetName(v string) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.SetName(v)
	})
}

// UpdateName sets the "name" field to the value that was provided on create.
func (u *VersionUpsertBulk) UpdateName() *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateName()
	})
}

// SetNumber sets the "number" field.
func (u *VersionUpsertBulk) SetNumber(v uint64) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.SetNumber(v)
	})
}

// AddNumber adds v to the "number" field.
func (u *VersionUpsertBulk) AddNumber(v uint64) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.AddNumber(v)
	})
}

// UpdateNumber sets the "number" field to the value that was provided on create.
func (u *VersionUpsertBulk) UpdateNumber() *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateNumber()
	})
}

// SetReleaseNote sets the "release_note" field.
func (u *VersionUpsertBulk) SetReleaseNote(v string) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.SetReleaseNote(v)
	})
}

// UpdateReleaseNote sets the "release_note" field to the value that was provided on create.
func (u *VersionUpsertBulk) UpdateReleaseNote() *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateReleaseNote()
	})
}

// SetCustomData sets the "custom_data" field.
func (u *VersionUpsertBulk) SetCustomData(v string) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.SetCustomData(v)
	})
}

// UpdateCustomData sets the "custom_data" field to the value that was provided on create.
func (u *VersionUpsertBulk) UpdateCustomData() *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateCustomData()
	})
}

// SetCreatedAt sets the "created_at" field.
func (u *VersionUpsertBulk) SetCreatedAt(v time.Time) *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.SetCreatedAt(v)
	})
}

// UpdateCreatedAt sets the "created_at" field to the value that was provided on create.
func (u *VersionUpsertBulk) UpdateCreatedAt() *VersionUpsertBulk {
	return u.Update(func(s *VersionUpsert) {
		s.UpdateCreatedAt()
	})
}

// Exec executes the query.
func (u *VersionUpsertBulk) Exec(ctx context.Context) error {
	if u.create.err != nil {
		return u.create.err
	}
	for i, b := range u.create.builders {
		if len(b.conflict) != 0 {
			return fmt.Errorf("ent: OnConflict was set for builder %d. Set it on the VersionCreateBulk instead", i)
		}
	}
	if len(u.create.conflict) == 0 {
		return errors.New("ent: missing options for VersionCreateBulk.OnConflict")
	}
	return u.create.Exec(ctx)
}

// ExecX is like Exec, but panics if an error occurs.
func (u *VersionUpsertBulk) ExecX(ctx context.Context) {
	if err := u.create.Exec(ctx); err != nil {
		panic(err)
	}
}

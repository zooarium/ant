package attribute

import (
	"context"
	"fmt"

	"ant/ent"
	entattribute "ant/ent/attribute"
	"ant/ent/attributeoption"
	"ant/ent/productattribute"
)

type attributeRepository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *attributeRepository {
	return &attributeRepository{client: client}
}

func (r *attributeRepository) Create(ctx context.Context, item Attribute) (Attribute, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Attribute{}, fmt.Errorf("begin tx: %w", err)
	}
	e, err := tx.Attribute.
		Create().
		SetAppID(item.AppID).
		SetUserID(item.UserID).
		SetDivisionID(item.DivisionID).
		SetName(item.Name).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Attribute{}, rollback(tx, fmt.Errorf("create attribute: %w", err))
	}
	if len(item.Options) > 0 {
		bulk := make([]*ent.AttributeOptionCreate, len(item.Options))
		for i, o := range item.Options {
			bulk[i] = tx.AttributeOption.
				Create().
				SetAttributeID(e.ID).
				SetValue(o.Value)
		}
		if _, err := tx.AttributeOption.CreateBulk(bulk...).Save(ctx); err != nil {
			return Attribute{}, rollback(tx, fmt.Errorf("create attribute options: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return Attribute{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, item.AppID, item.UserID, item.DivisionID, e.ID)
}

func (r *attributeRepository) List(ctx context.Context, appID, userID, divisionID, limit, offset int, status *int8) ([]Attribute, error) {
	q := r.client.Attribute.
		Query().
		Where(entattribute.AppID(appID), entattribute.DivisionID(divisionID))
	if status != nil {
		q = q.Where(entattribute.Status(*status))
	}
	es, err := q.
		WithOptions(func(oq *ent.AttributeOptionQuery) {
			oq.Order(ent.Asc(attributeoption.FieldID))
		}).
		Order(ent.Asc(entattribute.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list attributes: %w", err)
	}
	items := make([]Attribute, len(es))
	for i, e := range es {
		items[i] = r.mapToModel(e)
	}
	return items, nil
}

func (r *attributeRepository) GetByID(ctx context.Context, appID, userID, divisionID, id int) (Attribute, error) {
	e, err := r.client.Attribute.
		Query().
		Where(entattribute.ID(id), entattribute.AppID(appID), entattribute.DivisionID(divisionID)).
		WithOptions(func(oq *ent.AttributeOptionQuery) {
			oq.Order(ent.Asc(attributeoption.FieldID))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return Attribute{}, ErrAttributeNotFound
		}
		return Attribute{}, fmt.Errorf("get attribute by id: %w", err)
	}
	return r.mapToModel(e), nil
}

// Update atomically updates the attribute and syncs its options in one
// transaction: payload options with an id update the existing option, ones
// without an id are created, and existing options absent from the payload are
// deleted.
func (r *attributeRepository) Update(ctx context.Context, appID, userID, divisionID, id int, item Attribute, options []SyncOptionRequest) (Attribute, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return Attribute{}, fmt.Errorf("begin tx: %w", err)
	}
	count, err := tx.Attribute.
		Update().
		Where(entattribute.ID(id), entattribute.AppID(appID), entattribute.DivisionID(divisionID)).
		SetName(item.Name).
		SetStatus(item.Status).
		Save(ctx)
	if err != nil {
		return Attribute{}, rollback(tx, fmt.Errorf("update attribute: %w", err))
	}
	if count == 0 {
		return Attribute{}, rollback(tx, ErrAttributeNotFound)
	}

	existing, err := tx.AttributeOption.
		Query().
		Where(attributeoption.AttributeID(id)).
		Select(attributeoption.FieldID).
		Ints(ctx)
	if err != nil {
		return Attribute{}, rollback(tx, fmt.Errorf("list attribute options: %w", err))
	}
	existingIDs := make(map[int]struct{}, len(existing))
	for _, oid := range existing {
		existingIDs[oid] = struct{}{}
	}

	keep := make(map[int]struct{}, len(options))
	for _, o := range options {
		if o.ID == 0 {
			if _, err := tx.AttributeOption.
				Create().
				SetAttributeID(id).
				SetValue(o.Value).
				Save(ctx); err != nil {
				return Attribute{}, rollback(tx, fmt.Errorf("create attribute option: %w", err))
			}
			continue
		}
		if _, ok := existingIDs[o.ID]; !ok {
			return Attribute{}, rollback(tx, ErrOptionNotFound)
		}
		if _, err := tx.AttributeOption.
			Update().
			Where(attributeoption.ID(o.ID), attributeoption.AttributeID(id)).
			SetValue(o.Value).
			Save(ctx); err != nil {
			return Attribute{}, rollback(tx, fmt.Errorf("update attribute option: %w", err))
		}
		keep[o.ID] = struct{}{}
	}

	remove := make([]int, 0, len(existing))
	for _, oid := range existing {
		if _, ok := keep[oid]; !ok {
			remove = append(remove, oid)
		}
	}
	if len(remove) > 0 {
		if _, err := tx.AttributeOption.
			Delete().
			Where(attributeoption.IDIn(remove...), attributeoption.AttributeID(id)).
			Exec(ctx); err != nil {
			return Attribute{}, rollback(tx, fmt.Errorf("delete attribute options: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return Attribute{}, fmt.Errorf("commit tx: %w", err)
	}
	return r.GetByID(ctx, appID, userID, divisionID, id)
}

func (r *attributeRepository) Delete(ctx context.Context, appID, userID, divisionID, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	exists, err := tx.Attribute.
		Query().
		Where(entattribute.ID(id), entattribute.AppID(appID), entattribute.DivisionID(divisionID)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check attribute exists: %w", err))
	}
	if !exists {
		return rollback(tx, ErrAttributeNotFound)
	}
	assigned, err := tx.ProductAttribute.
		Query().
		Where(productattribute.AttributeID(id)).
		Exist(ctx)
	if err != nil {
		return rollback(tx, fmt.Errorf("check attribute in use: %w", err))
	}
	if assigned {
		return rollback(tx, ErrAttributeInUse)
	}
	if _, err := tx.AttributeOption.
		Delete().
		Where(attributeoption.AttributeID(id)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete attribute options: %w", err))
	}
	if _, err := tx.Attribute.
		Delete().
		Where(entattribute.ID(id), entattribute.AppID(appID), entattribute.DivisionID(divisionID)).
		Exec(ctx); err != nil {
		return rollback(tx, fmt.Errorf("delete attribute: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *attributeRepository) mapToModel(e *ent.Attribute) Attribute {
	options := make([]Option, len(e.Edges.Options))
	for i, o := range e.Edges.Options {
		options[i] = Option{ID: o.ID, Value: o.Value}
	}
	return Attribute{
		ID:         e.ID,
		AppID:      e.AppID,
		UserID:     e.UserID,
		DivisionID: e.DivisionID,
		Name:       e.Name,
		Status:     e.Status,
		Options:    options,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return fmt.Errorf("%w: rollback: %v", err, rerr)
	}
	return err
}

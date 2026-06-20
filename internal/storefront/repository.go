package storefront

import (
	"context"
	"fmt"
	"log/slog"

	"ant/ent"
	entstorefront "ant/ent/storefront"
)

type storefrontRepository struct {
	client *ent.Client
}

// NewRepository creates a new storefront repository.
func NewRepository(client *ent.Client) *storefrontRepository {
	return &storefrontRepository{client: client}
}

// Get returns the storefront for a tenant scope, or ErrStorefrontNotFound.
func (r *storefrontRepository) Get(ctx context.Context, appID, divisionID int) (*Storefront, error) {
	row, err := r.client.Storefront.Query().
		Where(
			entstorefront.AppIDEQ(appID),
			entstorefront.DivisionIDEQ(divisionID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrStorefrontNotFound
		}
		slog.Error("database error: failed to get storefront", "app_id", appID, "division_id", divisionID, "error", err)
		return nil, err
	}
	return mapToModel(row), nil
}

// Upsert creates the storefront for a tenant scope or replaces the existing
// one, inside a single transaction. The unique (app_id, division_id) index
// keeps it race-safe: a losing concurrent create is retried as an update.
func (r *storefrontRepository) Upsert(ctx context.Context, sf Storefront) (*Storefront, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		slog.Error("database error: failed to begin transaction for storefront upsert", "error", err)
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	result, err := r.upsertTx(ctx, tx, sf)
	if err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			slog.Error("database error: failed to rollback storefront upsert", "error", rerr)
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		slog.Error("database error: failed to commit storefront upsert", "error", err)
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return result, nil
}

func (r *storefrontRepository) upsertTx(ctx context.Context, tx *ent.Tx, sf Storefront) (*Storefront, error) {
	existing, err := tx.Storefront.Query().
		Where(
			entstorefront.AppIDEQ(sf.AppID),
			entstorefront.DivisionIDEQ(sf.DivisionID),
		).
		Only(ctx)
	switch {
	case err == nil:
		return r.update(ctx, tx, existing.ID, sf)
	case ent.IsNotFound(err):
		created, cerr := tx.Storefront.Create().
			SetAppID(sf.AppID).
			SetDivisionID(sf.DivisionID).
			SetHeroImage(sf.HeroImage).
			SetAssessments(sf.Assessments).
			SetGallery(sf.Gallery).
			SetFoodTags(sf.FoodTags).
			SetStatus(sf.Status).
			Save(ctx)
		if cerr != nil {
			// Lost a concurrent create race: the row now exists — update it.
			if ent.IsConstraintError(cerr) {
				row, qerr := tx.Storefront.Query().
					Where(
						entstorefront.AppIDEQ(sf.AppID),
						entstorefront.DivisionIDEQ(sf.DivisionID),
					).
					Only(ctx)
				if qerr != nil {
					return nil, qerr
				}
				return r.update(ctx, tx, row.ID, sf)
			}
			slog.Error("database error: failed to create storefront", "app_id", sf.AppID, "error", cerr)
			return nil, cerr
		}
		return mapToModel(created), nil
	default:
		slog.Error("database error: failed to load storefront for upsert", "app_id", sf.AppID, "error", err)
		return nil, err
	}
}

func (r *storefrontRepository) update(ctx context.Context, tx *ent.Tx, id int, sf Storefront) (*Storefront, error) {
	updated, err := tx.Storefront.UpdateOneID(id).
		SetHeroImage(sf.HeroImage).
		SetAssessments(sf.Assessments).
		SetGallery(sf.Gallery).
		SetFoodTags(sf.FoodTags).
		SetStatus(sf.Status).
		Save(ctx)
	if err != nil {
		slog.Error("database error: failed to update storefront", "id", id, "error", err)
		return nil, err
	}
	return mapToModel(updated), nil
}

func mapToModel(s *ent.Storefront) *Storefront {
	return &Storefront{
		ID:          s.ID,
		AppID:       s.AppID,
		DivisionID:  s.DivisionID,
		HeroImage:   s.HeroImage,
		Assessments: s.Assessments,
		Gallery:     s.Gallery,
		FoodTags:    s.FoodTags,
		Status:      s.Status,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

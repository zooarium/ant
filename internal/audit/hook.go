// Package audit writes one JSON line per entity mutation (create/update/
// delete) to a dedicated log file, via a single Ent client-level hook.
// Repositories/services/handlers are untouched — new entities are covered
// automatically as soon as they go through the client.
package audit

import (
	"context"
	"log/slog"
	"reflect"

	"ant/ent"

	"keeper/pkg/auth"
)

// identifiable is satisfied by every generated *XMutation's ID() method.
// Not part of the generic ent.Mutation interface (ID types vary by schema),
// so it's asserted per-mutation instead.
type identifiable interface {
	ID() (int, bool)
}

type appScoped interface {
	AppID() (int, bool)
}

type divisionScoped interface {
	DivisionID() (int, bool)
}

// Hook returns a client-level Ent hook that logs one line per mutation to
// logger. Logging failures never fail the caller's request — audit is
// best-effort observability here, not a hard compliance guarantee.
func Hook(logger *slog.Logger) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			value, err := next.Mutate(ctx, m)
			if err != nil {
				return value, err
			}

			attrs := []any{
				"action", m.Op().String(),
				"entity_type", m.Type(),
			}

			if id, exists := entityID(m); exists {
				attrs = append(attrs, "entity_id", id)
			} else if id := createdID(value); id != 0 {
				attrs = append(attrs, "entity_id", id)
			}

			if claims, ok := auth.GetClaimsFromContext(ctx); ok {
				attrs = append(attrs, "actor_id", claims.UserID)
			}

			if as, ok := m.(appScoped); ok {
				if appID, exists := as.AppID(); exists {
					attrs = append(attrs, "app_id", appID)
				}
			}
			if ds, ok := m.(divisionScoped); ok {
				if divisionID, exists := ds.DivisionID(); exists {
					attrs = append(attrs, "division_id", divisionID)
				}
			}

			logger.Info("mutation", attrs...)
			return value, nil
		})
	}
}

func entityID(m ent.Mutation) (int, bool) {
	idm, ok := m.(identifiable)
	if !ok {
		return 0, false
	}
	return idm.ID()
}

// createdID extracts the ID field via reflection for create mutations,
// which have no ID until after Save — there's no common typed interface
// across generated entities to read it back through instead.
func createdID(v ent.Value) int {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return 0
	}
	f := rv.FieldByName("ID")
	if !f.IsValid() || f.Kind() != reflect.Int {
		return 0
	}
	return int(f.Int())
}

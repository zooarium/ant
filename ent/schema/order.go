package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Order struct {
	ent.Schema
}

func (Order) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_order"},
	}
}

func (Order) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.Int("app_id"),
		field.Int("user_id"),
		field.Int("division_id"),
		// group_id is the order group (tab) this order belongs to. Every order
		// belongs to exactly one group (minted on create when not supplied).
		field.Int("group_id"),
		field.String("customer_name").
			NotEmpty(),
		field.String("customer_contact").
			NotEmpty(),
		// ordered_at is the business order date, settable by the caller
		// (defaults to now). Distinct from the immutable created_at audit
		// timestamp from TimeMixin.
		field.Time("ordered_at").
			Default(time.Now),
		// 1=pending, 2=confirmed, 3=completed, 4=cancelled, 5=paid
		field.Int8("status").
			Default(1),
		// tax_percent is the tax rate applied to the order, stored as a
		// percentage value (e.g. 18.5 = 18.5%). Range 0–100.
		field.Float("tax_percent").
			Min(0).
			Max(100).
			Default(0),
		// total is the denormalized pre-tax order amount: sum over items of
		// (base price + chosen option deltas) * quantity. Maintained inside the
		// same transaction on every item change (create/update) so list reads
		// avoid per-row aggregation. Detail reads recompute from items as the
		// authoritative value.
		field.Float("total").
			Min(0).
			Default(0),
		// ip_address is the client IP captured server-side at order creation
		// (audit / abuse signal). Optional; holds IPv4 or IPv6 (max 45 chars).
		field.String("ip_address").
			MaxLen(45).
			Optional(),
		// device_id is a client-generated identifier (persistent UUID, with an
		// optional fingerprint component) sent by the order-intake page to
		// recognise a returning customer and surface their order history. Soft
		// recognition only — not an identity/auth signal.
		field.String("device_id").
			MaxLen(64).
			Optional(),
	}
}

func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("products", OrderProduct.Type),
		edge.From("group", OrderGroup.Type).
			Ref("orders").
			Field("group_id").
			Unique().
			Required(),
	}
}

func (Order) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("app_id", "status"),
		index.Fields("group_id"),
		// Recognition lookup on order-intake: orders for a device within a
		// tenant scope. Always queried by app_id + division_id + device_id.
		index.Fields("app_id", "division_id", "device_id"),
	}
}

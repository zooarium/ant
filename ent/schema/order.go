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
		// 1=pending, 2=confirmed, 3=completed, 4=cancelled
		field.Int8("status").
			Default(1),
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
	}
}

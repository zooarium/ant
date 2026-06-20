package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OrderGroup is a "tab": several orders placed over time (e.g. a restaurant
// table) clubbed under one group so they can be settled together. Each order
// keeps its own status; the group carries the shared settlement lifecycle.
type OrderGroup struct {
	ent.Schema
}

func (OrderGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_order_group"},
	}
}

func (OrderGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (OrderGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int("app_id"),
		field.Int("user_id"),
		field.Int("division_id"),
		// token is a generated unique identifier for the group, usable as a
		// QR/short code in the UI to attach orders to the same tab.
		field.String("token").
			NotEmpty(),
		// label is an optional human name for the tab (e.g. "Table 5").
		field.String("label").
			Optional(),
		// 1=open, 2=closed, 3=paid, 4=cancelled
		field.Int8("status").
			Default(1),
	}
}

func (OrderGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("orders", Order.Type),
	}
}

func (OrderGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("app_id", "token").
			Unique(),
		index.Fields("app_id", "division_id", "status"),
	}
}

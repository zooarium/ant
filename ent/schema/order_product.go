package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OrderItemAttribute is the denormalized snapshot of a chosen attribute
// value stored with an order item. It survives later catalogue changes.
type OrderItemAttribute struct {
	AttributeID   int     `json:"attribute_id"`
	AttributeName string  `json:"attribute_name"`
	OptionID      int     `json:"option_id"`
	OptionValue   string  `json:"option_value"`
	PriceDelta    float64 `json:"price_delta"`
}

type OrderProduct struct {
	ent.Schema
}

func (OrderProduct) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_order_product"},
	}
}

func (OrderProduct) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (OrderProduct) Fields() []ent.Field {
	return []ent.Field{
		field.Int("order_id"),
		// product_id is a plain reference, intentionally NOT a foreign key:
		// orders must not depend on the product table.
		field.Int("product_id"),
		field.String("product_name").
			NotEmpty(),
		field.Float("price"),
		field.Int("quantity").
			Positive(),
		field.JSON("attributes", []OrderItemAttribute{}).
			Default([]OrderItemAttribute{}),
	}
}

func (OrderProduct) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("products").
			Field("order_id").
			Unique().
			Required(),
	}
}

func (OrderProduct) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
		index.Fields("product_id"),
	}
}

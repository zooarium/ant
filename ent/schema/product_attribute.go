package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProductAttributeOption is one option a product allows for an assigned
// attribute, with the price adjustment (positive or negative) applied to the
// product's base price when a customer picks it. Stored as a JSON list on the
// product-attribute row: the allowed subset is product-specific, the option
// catalogue is shared. option_id is validated in-app against the attribute's
// live options (no FK).
type ProductAttributeOption struct {
	OptionID   int     `json:"option_id"`
	PriceDelta float64 `json:"price_delta"`
}

type ProductAttribute struct {
	ent.Schema
}

func (ProductAttribute) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_product_attribute"},
	}
}

func (ProductAttribute) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (ProductAttribute) Fields() []ent.Field {
	return []ent.Field{
		field.Int("product_id"),
		field.Int("attribute_id"),
		field.Bool("is_mandatory").
			Default(false),
		// options is the product-specific allowed subset of the attribute's
		// options plus the per-option price delta.
		field.JSON("options", []ProductAttributeOption{}).
			Default([]ProductAttributeOption{}),
	}
}

func (ProductAttribute) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("product", Product.Type).
			Ref("attributes").
			Field("product_id").
			Unique().
			Required(),
		edge.From("attribute", Attribute.Type).
			Ref("product_attributes").
			Field("attribute_id").
			Unique().
			Required(),
	}
}

func (ProductAttribute) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("product_id", "attribute_id").
			Unique(),
		index.Fields("attribute_id"),
	}
}

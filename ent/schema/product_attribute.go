package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

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

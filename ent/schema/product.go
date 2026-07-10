package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Product struct {
	ent.Schema
}

func (Product) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_product"},
	}
}

func (Product) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (Product) Fields() []ent.Field {
	return []ent.Field{
		field.Int("app_id"),
		field.Int("user_id"),
		// division_id is the branch (keeper division) the product belongs to.
		// Master data is per-branch: products are created and queried within a
		// single division scope.
		field.Int("division_id"),
		field.String("name").
			NotEmpty(),
		field.Float("price").
			Default(0),
		field.Int8("status").
			Default(1),
		// featured marks a product for prominent placement; featured products
		// sort before others in all listings (admin and public).
		field.Bool("featured").
			Default(false),
		field.Int("category_id").
			Optional().
			Nillable(),
	}
}

func (Product) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("attributes", ProductAttribute.Type),
		edge.From("category", Category.Type).
			Ref("products").
			Unique().
			Field("category_id"),
	}
}

func (Product) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("app_id", "division_id"),
		index.Fields("app_id", "division_id", "category_id"),
	}
}

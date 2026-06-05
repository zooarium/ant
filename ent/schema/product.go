package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
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
		field.String("name").
			NotEmpty(),
		field.Int8("status").
			Default(1),
		// Add domain-specific fields here
	}
}

func (Product) Edges() []ent.Edge {
	return nil
}

func (Product) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("app_id"),
	}
}

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Attribute struct {
	ent.Schema
}

func (Attribute) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_attribute"},
	}
}

func (Attribute) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (Attribute) Fields() []ent.Field {
	return []ent.Field{
		field.Int("app_id"),
		field.Int("user_id"),
		field.String("name").
			NotEmpty(),
		field.Int8("status").
			Default(1),
	}
}

func (Attribute) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("options", AttributeOption.Type),
		edge.To("product_attributes", ProductAttribute.Type),
	}
}

func (Attribute) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("app_id"),
	}
}

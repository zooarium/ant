package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AttributeOption struct {
	ent.Schema
}

func (AttributeOption) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_attribute_option"},
	}
}

func (AttributeOption) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (AttributeOption) Fields() []ent.Field {
	return []ent.Field{
		field.Int("attribute_id"),
		field.String("value").
			NotEmpty(),
	}
}

func (AttributeOption) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("attribute", Attribute.Type).
			Ref("options").
			Field("attribute_id").
			Unique().
			Required(),
	}
}

func (AttributeOption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("attribute_id"),
	}
}

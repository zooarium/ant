package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Category is a hierarchical product category. Hierarchy is stored as a
// materialized path of ancestor+self ids (e.g. "/5/8/", self last), mirroring
// keeper's Division so the same subtree/move semantics apply.
type Category struct {
	ent.Schema
}

func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_category"},
	}
}

func (Category) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.Int("app_id"),
		// division_id is the branch (keeper division) the category belongs to.
		// Categories are per-branch master data, scoped within one division.
		field.Int("division_id"),
		field.Int("parent_id").
			Optional().
			Nillable(),
		field.String("name").
			NotEmpty(),
		// path is the materialized path of ancestor+self ids, e.g. "/5/8/".
		field.String("path"),
		// depth is the number of ancestors (root = 0), derived from path.
		field.Int8("depth").
			Default(0),
		field.Int8("status").
			Default(1),
	}
}

func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Category.Type),
		edge.From("parent", Category.Type).
			Ref("children").
			Unique().
			Field("parent_id"),
		edge.To("products", Product.Type),
	}
}

func (Category) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("path"),
		index.Fields("app_id", "division_id", "parent_id"),
	}
}

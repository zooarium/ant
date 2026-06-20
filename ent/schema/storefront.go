package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Assessment is one external platform's public reputation (e.g. Google,
// Swiggy, Zomato): an aggregate rating plus a small set of featured reviews.
// Stored as a JSON list on the storefront row so new platforms can be added
// without a schema migration. The per-platform review count cap (3) is
// enforced in the service layer, not the schema.
type Assessment struct {
	Name    string   `json:"name"`
	Rating  float64  `json:"rating"`
	Reviews []Review `json:"reviews"`
}

// Review is a single featured customer review shown under an assessment.
type Review struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

// GalleryImage is one photo in the storefront gallery. Sort gives the display
// order (ascending); the UI manages the full list and saves it as a whole.
type GalleryImage struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
	Sort    int    `json:"sort"`
}

// FoodTag is a manageable label applied to the storefront/menu (e.g. spicy,
// veg, gf, signature). Slug is the stable machine value, Label the display text.
type FoodTag struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

// Storefront is the per-tenant public presentation config: one row per
// (app_id, division_id). It holds branding, gallery, food tags, and external
// platform assessments rendered on the public storefront page.
type Storefront struct {
	ent.Schema
}

func (Storefront) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ant_storefront"},
	}
}

func (Storefront) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (Storefront) Fields() []ent.Field {
	return []ent.Field{
		field.Int("app_id"),
		field.Int("division_id"),
		// hero_image is the branding hero image URL. Optional; scheme is
		// restricted to http/https in the service layer.
		field.String("hero_image").
			Optional(),
		// assessments holds external platform ratings + featured reviews,
		// keyed by platform name within each element.
		field.JSON("assessments", []Assessment{}).
			Default([]Assessment{}),
		// gallery holds the storefront photo gallery (URL + caption + sort).
		field.JSON("gallery", []GalleryImage{}).
			Default([]GalleryImage{}),
		// food_tags holds the manageable label set (spicy/veg/gf/signature…).
		field.JSON("food_tags", []FoodTag{}).
			Default([]FoodTag{}),
		field.Int8("status").
			Default(1),
	}
}

func (Storefront) Indexes() []ent.Index {
	return []ent.Index{
		// One storefront per tenant scope; also makes the upsert race-safe.
		index.Fields("app_id", "division_id").
			Unique(),
	}
}

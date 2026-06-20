package storefront

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrStorefrontNotFound = errors.New("storefront not found")
	// ErrInvalid wraps all request-content validation failures (mapped to 400).
	ErrInvalid = errors.New("invalid storefront")
)

// Content limits. Caps bound the stored JSON blob size and the public payload;
// they are deliberately generous but finite.
const (
	maxHeroURLLen      = 2048
	maxAssessments     = 20
	maxReviewsPerAssmt = 3
	maxNameLen         = 50
	maxAuthorLen       = 100
	maxReviewTextLen   = 1000
	maxRating          = 5.0
	maxGallery         = 50
	maxGalleryURLLen   = 2048
	maxCaptionLen      = 200
	maxFoodTags        = 50
	maxSlugLen         = 50
	maxLabelLen        = 100
)

// Repository is the data-access contract for the storefront singleton.
type Repository interface {
	Get(ctx context.Context, appID, divisionID int) (*Storefront, error)
	Upsert(ctx context.Context, sf Storefront) (*Storefront, error)
}

// Cache is the in-memory cache contract (satisfied by pkg/cache.TTLCache).
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any)
	Delete(key string)
}

// Service is the business-logic contract for the storefront.
type Service interface {
	Get(ctx context.Context, appID, divisionID int) (*Storefront, error)
	Upsert(ctx context.Context, appID, divisionID int, req UpsertStorefrontRequest) (*Storefront, error)
}

type service struct {
	repo  Repository
	cache Cache
}

// NewService creates a new storefront service.
func NewService(repo Repository, cache Cache) Service {
	return &service{repo: repo, cache: cache}
}

func cacheKey(appID, divisionID int) string {
	return strconv.Itoa(appID) + ":" + strconv.Itoa(divisionID)
}

// Get returns the storefront for a tenant scope, reading through the cache.
// When none has been saved yet it returns an empty active storefront so the
// UI always receives a stable shape (no 404 special-casing).
func (s *service) Get(ctx context.Context, appID, divisionID int) (*Storefront, error) {
	key := cacheKey(appID, divisionID)
	if v, ok := s.cache.Get(key); ok {
		return v.(*Storefront), nil
	}

	sf, err := s.repo.Get(ctx, appID, divisionID)
	if err != nil {
		if errors.Is(err, ErrStorefrontNotFound) {
			sf = &Storefront{
				AppID:       appID,
				DivisionID:  divisionID,
				Assessments: []Assessment{},
				Gallery:     []GalleryImage{},
				FoodTags:    []FoodTag{},
				Status:      1,
			}
		} else {
			return nil, err
		}
	}

	s.cache.Set(key, sf)
	return sf, nil
}

// Upsert validates and persists the whole storefront, then refreshes the cache.
func (s *service) Upsert(ctx context.Context, appID, divisionID int, req UpsertStorefrontRequest) (*Storefront, error) {
	if err := validate(req); err != nil {
		return nil, err
	}

	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}

	updated, err := s.repo.Upsert(ctx, Storefront{
		AppID:       appID,
		DivisionID:  divisionID,
		HeroImage:   strings.TrimSpace(req.HeroImage),
		Assessments: normalizeAssessments(req.Assessments),
		Gallery:     normalizeGallery(req.Gallery),
		FoodTags:    normalizeFoodTags(req.FoodTags),
		Status:      status,
	})
	if err != nil {
		slog.Error("failed to upsert storefront", "app_id", appID, "division_id", divisionID, "error", err)
		return nil, err
	}

	s.cache.Set(cacheKey(appID, divisionID), updated)
	slog.Info("storefront upserted", "id", updated.ID, "app_id", appID, "division_id", divisionID)
	return updated, nil
}

// validate enforces all content rules. Returns ErrInvalid wrapping a precise
// message so the handler can render a 400.
func validate(req UpsertStorefrontRequest) error {
	if len(req.HeroImage) > maxHeroURLLen {
		return invalid("hero_image exceeds %d chars", maxHeroURLLen)
	}
	if req.HeroImage != "" && !isHTTPURL(req.HeroImage) {
		return invalid("hero_image must be an http(s) URL")
	}

	if len(req.Assessments) > maxAssessments {
		return invalid("at most %d assessments allowed", maxAssessments)
	}
	for i, a := range req.Assessments {
		if strings.TrimSpace(a.Name) == "" {
			return invalid("assessments[%d].name is required", i)
		}
		if len(a.Name) > maxNameLen {
			return invalid("assessments[%d].name exceeds %d chars", i, maxNameLen)
		}
		if a.Rating < 0 || a.Rating > maxRating {
			return invalid("assessments[%d].rating must be between 0 and %g", i, maxRating)
		}
		if len(a.Reviews) > maxReviewsPerAssmt {
			return invalid("assessments[%d] has more than %d reviews", i, maxReviewsPerAssmt)
		}
		for j, rv := range a.Reviews {
			if len(rv.Author) > maxAuthorLen {
				return invalid("assessments[%d].reviews[%d].author exceeds %d chars", i, j, maxAuthorLen)
			}
			if strings.TrimSpace(rv.Text) == "" {
				return invalid("assessments[%d].reviews[%d].text is required", i, j)
			}
			if len(rv.Text) > maxReviewTextLen {
				return invalid("assessments[%d].reviews[%d].text exceeds %d chars", i, j, maxReviewTextLen)
			}
		}
	}

	if len(req.Gallery) > maxGallery {
		return invalid("at most %d gallery images allowed", maxGallery)
	}
	for i, g := range req.Gallery {
		if len(g.URL) > maxGalleryURLLen {
			return invalid("gallery[%d].url exceeds %d chars", i, maxGalleryURLLen)
		}
		if !isHTTPURL(g.URL) {
			return invalid("gallery[%d].url must be an http(s) URL", i)
		}
		if len(g.Caption) > maxCaptionLen {
			return invalid("gallery[%d].caption exceeds %d chars", i, maxCaptionLen)
		}
	}

	if len(req.FoodTags) > maxFoodTags {
		return invalid("at most %d food tags allowed", maxFoodTags)
	}
	for i, t := range req.FoodTags {
		if strings.TrimSpace(t.Slug) == "" {
			return invalid("food_tags[%d].slug is required", i)
		}
		if len(t.Slug) > maxSlugLen {
			return invalid("food_tags[%d].slug exceeds %d chars", i, maxSlugLen)
		}
		if strings.TrimSpace(t.Label) == "" {
			return invalid("food_tags[%d].label is required", i)
		}
		if len(t.Label) > maxLabelLen {
			return invalid("food_tags[%d].label exceeds %d chars", i, maxLabelLen)
		}
	}

	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// isHTTPURL reports whether raw is an absolute http/https URL with a host.
// Restricting the scheme blocks javascript:/data: vectors in rendered links.
func isHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// normalize* guarantee non-nil slices (so JSON renders [] not null) and trim
// surrounding whitespace on free-text fields.
func normalizeAssessments(in []Assessment) []Assessment {
	out := make([]Assessment, 0, len(in))
	for _, a := range in {
		reviews := make([]Review, 0, len(a.Reviews))
		for _, rv := range a.Reviews {
			reviews = append(reviews, Review{
				Author: strings.TrimSpace(rv.Author),
				Text:   strings.TrimSpace(rv.Text),
			})
		}
		out = append(out, Assessment{
			Name:    strings.TrimSpace(a.Name),
			Rating:  a.Rating,
			Reviews: reviews,
		})
	}
	return out
}

func normalizeGallery(in []GalleryImage) []GalleryImage {
	out := make([]GalleryImage, 0, len(in))
	for _, g := range in {
		out = append(out, GalleryImage{
			URL:     strings.TrimSpace(g.URL),
			Caption: strings.TrimSpace(g.Caption),
			Sort:    g.Sort,
		})
	}
	return out
}

func normalizeFoodTags(in []FoodTag) []FoodTag {
	out := make([]FoodTag, 0, len(in))
	for _, t := range in {
		out = append(out, FoodTag{
			Slug:  strings.TrimSpace(t.Slug),
			Label: strings.TrimSpace(t.Label),
		})
	}
	return out
}

package template

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// QueryService is the read/query layer for Template search and pagination.
// UI must not filter or page Template lists client-side.
type QueryService struct {
	templates Repository
}

// NewQueryService constructs a TemplateQueryService.
func NewQueryService(templates Repository) *QueryService {
	return &QueryService{templates: templates}
}

// NormalizeListFilter applies LanguageResolver and category parsing to list filters.
func NormalizeListFilter(filter ListFilter) ListFilter {
	filter.Search = strings.TrimSpace(filter.Search)
	if lang := strings.TrimSpace(filter.Language); lang != "" && !strings.EqualFold(lang, "all") {
		filter.Language = DefaultLanguageResolver().NormalizeForFilter(lang)
	} else {
		filter.Language = ""
	}
	if cat := strings.TrimSpace(filter.Category); cat != "" && !strings.EqualFold(cat, "all") {
		if parsed, ok := ParseCategory(cat); ok {
			filter.Category = string(parsed)
		} else {
			filter.Category = strings.ToUpper(cat)
		}
	} else {
		filter.Category = ""
	}
	return filter
}

// Search lists templates with normalized filters and server-side pagination.
func (q *QueryService) Search(
	ctx context.Context,
	teamID uuid.UUID,
	filter ListFilter,
	page repository.PageParams,
) (repository.Page[Template], error) {
	if q == nil || q.templates == nil {
		return repository.Page[Template]{Items: []Template{}, Pagination: repository.NewMeta(page.Page, page.PageSize, 0)}, nil
	}
	return q.templates.List(ctx, teamID, NormalizeListFilter(filter), page)
}

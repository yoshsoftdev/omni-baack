package repository

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// PageParams matches API v1 list query parameters.
type PageParams struct {
	Page     int
	PageSize int
}

// Normalize applies defaults and caps page_size.
func (p PageParams) Normalize() PageParams {
	if p.Page < 1 {
		p.Page = DefaultPage
	}
	if p.PageSize <= 0 {
		p.PageSize = DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		p.PageSize = MaxPageSize
	}
	return p
}

// Offset returns SQL OFFSET value.
func (p PageParams) Offset() int {
	n := p.Normalize()
	return (n.Page - 1) * n.PageSize
}

// Limit returns SQL LIMIT value.
func (p PageParams) Limit() int {
	return p.Normalize().PageSize
}

// Meta is pagination metadata for list responses.
type Meta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewMeta builds pagination meta from total row count.
func NewMeta(page, pageSize int, total int64) Meta {
	totalPages := 0
	if pageSize > 0 && total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return Meta{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

// Page is a paginated result set.
type Page[T any] struct {
	Items      []T  `json:"items"`
	Pagination Meta `json:"pagination"`
}

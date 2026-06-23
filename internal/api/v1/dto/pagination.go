package dto

import "github.com/omnichannel/omni-chan-v2/pkg/repository"

// PageResponse wraps paginated list data for API v1.
type PageResponse[T any] struct {
	Items      []T             `json:"items"`
	Pagination repository.Meta `json:"pagination"`
}

func NewPageResponse[T any](page repository.Page[T]) PageResponse[T] {
	return PageResponse[T]{
		Items:      page.Items,
		Pagination: page.Pagination,
	}
}

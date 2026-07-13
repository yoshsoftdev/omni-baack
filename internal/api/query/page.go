package query

import "github.com/gofiber/fiber/v2"

// PageSize reads list page size from page_size or limit query params.
func PageSize(c *fiber.Ctx) int {
	if size := c.QueryInt("page_size", 0); size > 0 {
		return size
	}
	if size := c.QueryInt("limit", 0); size > 0 {
		return size
	}
	return 50
}

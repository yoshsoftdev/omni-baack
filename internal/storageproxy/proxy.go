package storageproxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

// Handler forwards /api/v1/storage/* requests to the storage-service.
func Handler(targetURL string) fiber.Handler {
	target, err := url.Parse(targetURL)
	if err != nil {
		panic("invalid STORAGE_SERVICE_URL: " + err.Error())
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
	}
	return adaptor.HTTPHandler(proxy)
}

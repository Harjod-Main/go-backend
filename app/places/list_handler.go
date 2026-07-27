package places

import (
	"log/slog"
	"net/http"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

// List returns non-blacklisted places for the map drawer (public read).
// Responses are served from a short in-memory TTL cache and advertise
// Cache-Control + ETag so clients can short-circuit with If-None-Match.
func (h *Handler) List(c *gin.Context) {
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch != "" {
		if _, etag, ok := h.listCache.peek(); ok && etagMatches(ifNoneMatch, etag) {
			c.Header("Cache-Control", listMapPlacesCacheControl)
			c.Header("ETag", etag)
			c.Header("Vary", "Accept-Encoding")
			c.Status(http.StatusNotModified)
			return
		}
	}

	places, etag, err := h.listCache.getOrLoad(c.Request.Context(), h.repo.ListMapPlaces)
	if err != nil {
		slog.Error("places list failed", "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	c.Header("Cache-Control", listMapPlacesCacheControl)
	c.Header("ETag", etag)
	c.Header("Vary", "Accept-Encoding")

	if etagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &places,
	})
}

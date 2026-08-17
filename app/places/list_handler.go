package places

import (
	"context"
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
	bounds, err := parseMapBounds(c.Query("west"), c.Query("south"), c.Query("east"), c.Query("north"))
	if err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if bounds != nil {
		h.respondMapPlaces(c, func(ctx context.Context) ([]Place, error) {
			return h.repo.ListMapPlaces(ctx, bounds)
		}, false)
		return
	}

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

	places, etag, err := h.listCache.getOrLoad(c.Request.Context(), func(ctx context.Context) ([]Place, error) {
		return h.repo.ListMapPlaces(ctx, nil)
	})
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

func (h *Handler) respondMapPlaces(c *gin.Context, load func(context.Context) ([]Place, error), cacheable bool) {
	places, err := load(c.Request.Context())
	if err != nil {
		slog.Error("places list failed", "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if cacheable {
		c.Header("Cache-Control", listMapPlacesCacheControl)
	} else {
		c.Header("Cache-Control", "public, max-age=15")
	}
	wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &places,
	})
}

func (h *Handler) invalidateMapList(ctx context.Context) {
	h.listCache.invalidate()
	if err := h.repo.RefreshMapPlacePins(ctx); err != nil {
		slog.Error("refresh map place pins failed", "error", err)
	}
}

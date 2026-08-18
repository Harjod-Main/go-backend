package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const listMapPlacesSQL = `
SELECT COALESCE(json_agg(row_to_json(p) ORDER BY p.name_th), '[]'::json)
FROM (
	SELECT
		pin.place_id::text AS place_id,
		pin.name_th,
		pin.name_en,
		pin.place_type::text AS place_type,
		pin.latitude::float8 AS latitude,
		pin.longitude::float8 AS longitude,
		pin.address_th,
		pin.district_th,
		pin.province_th,
		pin.postal_code,
		pin.photo_url,
		pin.avg_rating,
		pin.review_count,
		pin.has_ev_charging,
		pin.has_valet,
		pin.has_cover,
		pin.transit_access,
		pin.transit_access_type,
		pin.total_spaces,
		pin.free_minutes,
		pin.min_hourly_rate,
		CASE WHEN h.open_time IS NULL THEN NULL ELSE to_char(h.open_time, 'HH24:MI:SS') END AS today_open_time,
		CASE WHEN h.close_time IS NULL THEN NULL ELSE to_char(h.close_time, 'HH24:MI:SS') END AS today_close_time,
		h.is_closed AS today_is_closed,
		ent.entrance_latitude,
		ent.entrance_longitude
	FROM map_place_pins pin
	LEFT JOIN LATERAL (
		SELECT e.latitude::float8 AS entrance_latitude, e.longitude::float8 AS entrance_longitude
		FROM entrance_exit e
		WHERE e.parking_area_id = pin.parking_area_id
			AND e.latitude IS NOT NULL
			AND e.longitude IS NOT NULL
			AND e.direction::text IN ('entry', 'both')
		ORDER BY e.entrance_id
		LIMIT 1
	) ent ON true
	LEFT JOIN hours h ON h.parking_area_id = pin.parking_area_id
		AND h.day_of_week = (ARRAY['SUN','MON','TUE','WED','THU','FRI','SAT']::day_of_week_enum[])[
			EXTRACT(DOW FROM (NOW() AT TIME ZONE 'Asia/Bangkok'))::int + 1
		]
	WHERE CASE
		WHEN $1::float8 IS NULL THEN true
		ELSE pin.geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)::geography
	END
) p
`

func (r *postgresRepo) ListMapPlaces(ctx context.Context, bounds *MapBounds) ([]Place, error) {
	var west, south, east, north any
	if bounds != nil {
		west, south, east, north = bounds.West, bounds.South, bounds.East, bounds.North
	}
	var raw []byte
	if err := r.pool.QueryRow(ctx, listMapPlacesSQL, west, south, east, north).Scan(&raw); err != nil {
		return nil, fmt.Errorf("list map places: %w", err)
	}

	places := make([]Place, 0)
	if err := json.Unmarshal(raw, &places); err != nil {
		return nil, fmt.Errorf("decode map places: %w", err)
	}
	return places, nil
}

func (r *postgresRepo) RefreshMapPlacePins(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `SELECT public.refresh_map_place_pins()`); err != nil {
		return fmt.Errorf("refresh map place pins: %w", err)
	}
	return nil
}

const getMapPlaceCardSQL = `
SELECT json_build_object(
	'place_id', pl.place_id::text,
	'photo_urls', COALESCE((
		SELECT json_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
		FROM place_images img
		WHERE img.entity_type = 'place'
			AND img.entity_id = pl.place_id::text
			AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
	), COALESCE((
		SELECT to_json(ps.photo_urls)
		FROM place_submissions ps
		WHERE ps.place_id = pl.place_id
			AND ps.status = 'approved'
			AND COALESCE(cardinality(ps.photo_urls), 0) > 0
		ORDER BY ps.created_at DESC
		LIMIT 1
	), '[]'::json)),
	'hours', COALESCE((
		SELECT json_agg(
			json_build_object(
				'day_of_week', h.day_of_week::text,
				'open_time', CASE WHEN h.open_time IS NULL THEN NULL ELSE to_char(h.open_time, 'HH24:MI:SS') END,
				'close_time', CASE WHEN h.close_time IS NULL THEN NULL ELSE to_char(h.close_time, 'HH24:MI:SS') END,
				'is_closed', h.is_closed
			)
			ORDER BY h.day_of_week
		)
		FROM hours h
		WHERE h.parking_area_id = (
			SELECT pa.parking_area_id
			FROM parking_area pa
			WHERE pa.place_id = pl.place_id
			ORDER BY pa.parking_area_id
			LIMIT 1
		)
	), '[]'::json),
	'entrances', COALESCE((
		SELECT json_agg(
			json_build_object(
				'latitude', e.latitude::float8,
				'longitude', e.longitude::float8,
				'direction', e.direction::text
			)
			ORDER BY e.entrance_id
		)
		FROM entrance_exit e
		INNER JOIN parking_area pa ON pa.parking_area_id = e.parking_area_id
		WHERE pa.place_id = pl.place_id
			AND e.latitude IS NOT NULL
			AND e.longitude IS NOT NULL
	), '[]'::json)
)
FROM places pl
WHERE pl.place_id = $1::uuid
	AND COALESCE(pl.is_blacklisted, false) = false
`

func (r *postgresRepo) GetMapPlaceCard(ctx context.Context, placeID string) (*MapPlaceCard, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, getMapPlaceCardSQL, placeID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get map place card: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var card MapPlaceCard
	if err := json.Unmarshal(raw, &card); err != nil {
		return nil, fmt.Errorf("decode map place card: %w", err)
	}
	if card.PhotoURLs == nil {
		card.PhotoURLs = []string{}
	}
	if card.Hours == nil {
		card.Hours = []Hour{}
	}
	if card.Entrances == nil {
		card.Entrances = []MapPlaceEntrance{}
	}
	return &card, nil
}

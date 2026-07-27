package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listMapPlacesSQL = `
SELECT COALESCE(json_agg(row_to_json(p) ORDER BY p.name_th), '[]'::json)
FROM (
	SELECT
		pl.place_id::text AS place_id,
		pl.name_th,
		pl.name_en,
		pl.place_type::text AS place_type,
		pl.latitude::float8 AS latitude,
		pl.longitude::float8 AS longitude,
		pl.address_th,
		pl.district_th,
		pl.province_th,
		pl.postal_code,
		COALESCE((
			SELECT json_agg(area_obj)
			FROM (
				SELECT json_build_object(
					'total_spaces', pa.total_spaces,
					'has_ev_charging', pa.has_ev_charging,
					'has_valet', pa.has_valet,
					'has_cover', pa.has_cover,
					'transit_access', pa.transit_access,
					'transit_access_type', pa.transit_access_type,
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
						WHERE h.parking_area_id = pa.parking_area_id
					), '[]'::json),
					'rate', COALESCE((
						SELECT json_agg(
							json_build_object(
								'free_minutes', r.free_minutes,
								'rate_tier', COALESCE((
									SELECT json_agg(
										json_build_object(
											'tier_order', rt.tier_order,
											'price', rt.price::float8,
											'unit', rt.unit::text
										)
										ORDER BY rt.tier_order
									)
									FROM rate_tier rt
									WHERE rt.rate_id = r.rate_id
								), '[]'::json)
							)
						)
						FROM rate r
						WHERE r.parking_area_id = pa.parking_area_id
					), '[]'::json)
				) AS area_obj
				FROM parking_area pa
				WHERE pa.place_id = pl.place_id
			) areas
		), '[]'::json) AS parking_area
	FROM places pl
	WHERE COALESCE(pl.is_blacklisted, false) = false
) p
`

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

func (r *postgresRepo) ListMapPlaces(ctx context.Context) ([]Place, error) {
	var raw []byte
	if err := r.pool.QueryRow(ctx, listMapPlacesSQL).Scan(&raw); err != nil {
		return nil, fmt.Errorf("list map places: %w", err)
	}

	places := make([]Place, 0)
	if err := json.Unmarshal(raw, &places); err != nil {
		return nil, fmt.Errorf("decode map places: %w", err)
	}
	return places, nil
}

const getPlaceRateSQL = `
SELECT row_to_json(rate_obj)
FROM (
	SELECT
		r.free_minutes,
		r.daily_max::float8 AS daily_max,
		r.lost_ticket_fee::float8 AS lost_ticket_fee,
		r.night_rate::float8 AS night_rate,
		CASE WHEN r.night_start_time IS NULL THEN NULL ELSE to_char(r.night_start_time, 'HH24:MI:SS') END AS night_start_time,
		CASE WHEN r.night_end_time IS NULL THEN NULL ELSE to_char(r.night_end_time, 'HH24:MI:SS') END AS night_end_time,
		r.currency,
		r.notes,
		COALESCE((
			SELECT json_agg(
				json_build_object(
					'tier_order', rt.tier_order,
					'price', rt.price::float8,
					'unit', rt.unit::text,
					'from_hour', rt.from_hour::float8,
					'to_hour', CASE WHEN rt.to_hour IS NULL THEN NULL ELSE rt.to_hour::float8 END
				)
				ORDER BY rt.tier_order
			)
			FROM rate_tier rt
			WHERE rt.rate_id = r.rate_id
		), '[]'::json) AS rate_tier
	FROM places pl
	INNER JOIN parking_area pa ON pa.place_id = pl.place_id
	INNER JOIN rate r ON r.parking_area_id = pa.parking_area_id
	WHERE pl.place_id = $1::uuid
		AND COALESCE(pl.is_blacklisted, false) = false
	ORDER BY pa.parking_area_id
	LIMIT 1
) rate_obj
`

func (r *postgresRepo) GetPlaceRate(ctx context.Context, placeID string) (*PlaceRateDetail, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, getPlaceRateSQL, placeID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get place rate: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var rate PlaceRateDetail
	if err := json.Unmarshal(raw, &rate); err != nil {
		return nil, fmt.Errorf("decode place rate: %w", err)
	}
	return &rate, nil
}

package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
			SELECT array_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
			FROM place_images img
			WHERE img.entity_type = 'place'
				AND img.entity_id = pl.place_id::text
				AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
		), COALESCE((
			SELECT ps.photo_urls
			FROM place_submissions ps
			WHERE ps.place_id = pl.place_id
				AND ps.status = 'approved'
				AND COALESCE(cardinality(ps.photo_urls), 0) > 0
			ORDER BY ps.created_at DESC
			LIMIT 1
		), '{}'::text[])) AS photo_urls,
		rev.avg_rating,
		COALESCE(rev.review_count, 0) AS review_count,
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
	LEFT JOIN (
		SELECT
			place_id,
			ROUND(AVG(rating)::numeric, 1)::float8 AS avg_rating,
			COUNT(*)::int AS review_count
		FROM reviews
		GROUP BY place_id
	) rev ON rev.place_id = pl.place_id
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

const getPlaceRatesSQL = `
SELECT
	picked.place_id::text,
	json_build_object(
		'free_minutes', picked.free_minutes,
		'daily_max', picked.daily_max,
		'lost_ticket_fee', picked.lost_ticket_fee,
		'night_rate', picked.night_rate,
		'night_start_time', picked.night_start_time,
		'night_end_time', picked.night_end_time,
		'currency', picked.currency,
		'notes', picked.notes,
		'rate_tier', COALESCE((
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
			WHERE rt.rate_id = picked.rate_id
		), '[]'::json)
	)
FROM (
	SELECT DISTINCT ON (pl.place_id)
		pl.place_id,
		r.rate_id,
		r.free_minutes,
		r.daily_max::float8 AS daily_max,
		r.lost_ticket_fee::float8 AS lost_ticket_fee,
		r.night_rate::float8 AS night_rate,
		CASE WHEN r.night_start_time IS NULL THEN NULL ELSE to_char(r.night_start_time, 'HH24:MI:SS') END AS night_start_time,
		CASE WHEN r.night_end_time IS NULL THEN NULL ELSE to_char(r.night_end_time, 'HH24:MI:SS') END AS night_end_time,
		r.currency,
		r.notes
	FROM places pl
	INNER JOIN parking_area pa ON pa.place_id = pl.place_id
	INNER JOIN rate r ON r.parking_area_id = pa.parking_area_id
	WHERE pl.place_id = ANY($1::uuid[])
		AND COALESCE(pl.is_blacklisted, false) = false
	ORDER BY pl.place_id, pa.parking_area_id
) picked
`

func (r *postgresRepo) GetPlaceRates(ctx context.Context, placeIDs []string) (map[string]*PlaceRateDetail, error) {
	out := make(map[string]*PlaceRateDetail, len(placeIDs))
	if len(placeIDs) == 0 {
		return out, nil
	}

	rows, err := r.pool.Query(ctx, getPlaceRatesSQL, placeIDs)
	if err != nil {
		return nil, fmt.Errorf("get place rates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var placeID string
		var raw []byte
		if err := rows.Scan(&placeID, &raw); err != nil {
			return nil, fmt.Errorf("scan place rates: %w", err)
		}
		if raw == nil || string(raw) == "null" {
			continue
		}
		var rate PlaceRateDetail
		if err := json.Unmarshal(raw, &rate); err != nil {
			return nil, fmt.Errorf("decode place rate %s: %w", placeID, err)
		}
		out[placeID] = &rate
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate place rates: %w", err)
	}
	return out, nil
}

const getPlacePrivilegesSQL = `
SELECT json_build_object(
	'validation_parking', COALESCE((
		SELECT json_agg(
			json_build_object(
				'validation', json_build_object(
					'validation_id', v.validation_id::text,
					'validation_type', v.validation_type::text,
					'condition_description', COALESCE(v.condition_description, ''),
					'validation_location', v.validation_location,
					'notes', v.notes,
					'program_other', v.program_other,
					'program', CASE WHEN prog.program_id IS NULL THEN NULL ELSE json_build_object(
						'name', prog.name,
						'provider', prog.provider,
						'category', prog.category::text
					) END,
					'validation_tier', COALESCE((
						SELECT json_agg(
							json_build_object(
								'tier_order', vt.tier_order,
								'min_spend', vt.min_spend::float8,
								'free_minutes', vt.free_minutes
							)
							ORDER BY vt.tier_order
						)
						FROM validation_tier vt
						WHERE vt.validation_id = v.validation_id
					), '[]'::json)
				)
			)
			ORDER BY v.validation_id
		)
		FROM validation_parking vp
		INNER JOIN validation v ON v.validation_id = vp.validation_id
		LEFT JOIN program prog ON prog.program_id = v.program_id
		WHERE vp.place_id = pl.place_id
	), '[]'::json),
	'parking_area', COALESCE((
		SELECT json_agg(area_obj)
		FROM (
			SELECT json_build_object(
				'reserved', COALESCE((
					SELECT json_agg(
						json_build_object(
							'reserved_id', res.reserved_id::text,
							'reservation_type', res.reservation_type::text,
							'reservation_type_other', res.reservation_type_other,
							'program_other', res.program_other,
							'floor', res.floor,
							'conditions', res.conditions,
							'spots_count', res.spots_count,
							'additional_benefits', res.additional_benefits,
							'program', CASE WHEN rprog.program_id IS NULL THEN NULL ELSE json_build_object(
								'name', rprog.name,
								'provider', rprog.provider,
								'category', rprog.category::text
							) END
						)
						ORDER BY res.reserved_id
					)
					FROM reserved res
					LEFT JOIN program rprog ON rprog.program_id = res.program_id
					WHERE res.parking_area_id = pa.parking_area_id
				), '[]'::json),
				'ev_charger', COALESCE((
					SELECT json_agg(
						json_build_object(
							'ev_charger_id', ev.ev_charger_id::text,
							'floor', ev.floor,
							'ev_provider', CASE WHEN ep.ev_provider_id IS NULL THEN NULL ELSE json_build_object(
								'name', ep.name
							) END,
							'ev_connector', COALESCE((
								SELECT json_agg(
									json_build_object(
										'connector_type', ec.connector_type::text
									)
									ORDER BY ec.ev_connector_id
								)
								FROM ev_connector ec
								WHERE ec.ev_charger_id = ev.ev_charger_id
							), '[]'::json)
						)
						ORDER BY ev.ev_charger_id
					)
					FROM ev_charger ev
					LEFT JOIN ev_provider ep ON ep.ev_provider_id = ev.ev_provider_id
					WHERE ev.parking_area_id = pa.parking_area_id
				), '[]'::json)
			) AS area_obj
			FROM parking_area pa
			WHERE pa.place_id = pl.place_id
		) areas
	), '[]'::json)
)
FROM places pl
WHERE pl.place_id = $1::uuid
	AND COALESCE(pl.is_blacklisted, false) = false
`

func (r *postgresRepo) GetPlacePrivileges(ctx context.Context, placeID string) (*PlacePrivileges, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, getPlacePrivilegesSQL, placeID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get place privileges: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var privileges PlacePrivileges
	if err := json.Unmarshal(raw, &privileges); err != nil {
		return nil, fmt.Errorf("decode place privileges: %w", err)
	}
	if privileges.ValidationParking == nil {
		privileges.ValidationParking = []ValidationParking{}
	}
	if privileges.ParkingArea == nil {
		privileges.ParkingArea = []PrivilegeArea{}
	}
	return &privileges, nil
}

const getValidationSQL = `
SELECT json_build_object(
	'validation_id', v.validation_id::text,
	'validation_type', v.validation_type::text,
	'condition_description', COALESCE(v.condition_description, ''),
	'validation_location', v.validation_location,
	'notes', v.notes,
	'program_other', v.program_other,
	'program', CASE WHEN prog.program_id IS NULL THEN NULL ELSE json_build_object(
		'name', prog.name,
		'provider', prog.provider,
		'category', prog.category::text
	) END,
	'validation_tier', COALESCE((
		SELECT json_agg(
			json_build_object(
				'tier_order', vt.tier_order,
				'min_spend', vt.min_spend::float8,
				'free_minutes', vt.free_minutes
			)
			ORDER BY vt.tier_order
		)
		FROM validation_tier vt
		WHERE vt.validation_id = v.validation_id
	), '[]'::json)
)
FROM validation v
LEFT JOIN program prog ON prog.program_id = v.program_id
WHERE v.validation_id = $1::uuid
`

func (r *postgresRepo) GetValidation(ctx context.Context, validationID string) (*Validation, error) {
	return scanJSON[Validation](ctx, r.pool, getValidationSQL, validationID, "validation")
}

const updateValidationSQL = `
UPDATE validation
SET validation_type = $2::validation_type_enum,
    condition_description = $3,
    notes = $4,
    validation_location = $5
WHERE validation_id = $1::uuid
`

const hasValidationCorrectionSQL = `
SELECT EXISTS(
  SELECT 1 FROM audit_log
  WHERE entity_type = 'validation'
    AND entity_id = $1
    AND action = 'correct'
    AND changed_by = $2
)
`

const insertValidationAuditSQL = `
INSERT INTO audit_log (entity_type, entity_id, action, old_data, changed_by, created_at)
VALUES ('validation', $1, 'correct', $2::jsonb, $3, NOW())
`

func (r *postgresRepo) UpdateValidation(
	ctx context.Context,
	validationID string,
	in UpdateValidationInput,
) (*Validation, bool, error) {
	existing, err := r.GetValidation(ctx, validationID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, nil
	}

	oldData, err := json.Marshal(existing)
	if err != nil {
		return nil, false, fmt.Errorf("encode validation audit: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin validation update: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, updateValidationSQL,
		validationID,
		in.ValidationType,
		in.ConditionDescription,
		in.Notes,
		in.ValidationLocation,
	)
	if err != nil {
		return nil, false, fmt.Errorf("update validation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, false, nil
	}

	var alreadyCorrected bool
	if err := tx.QueryRow(ctx, hasValidationCorrectionSQL, validationID, in.ChangedBy).Scan(&alreadyCorrected); err != nil {
		return nil, false, fmt.Errorf("check validation correction: %w", err)
	}

	// pgx encodes []byte as bytea; pass a string so $2::jsonb gets valid JSON text.
	if _, err := tx.Exec(ctx, insertValidationAuditSQL, validationID, string(oldData), in.ChangedBy); err != nil {
		return nil, false, fmt.Errorf("insert validation audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit validation update: %w", err)
	}

	updated, err := r.GetValidation(ctx, validationID)
	if err != nil {
		return nil, false, err
	}
	return updated, !alreadyCorrected, nil
}

const getReservedSQL = `
SELECT json_build_object(
	'reserved_id', res.reserved_id::text,
	'reservation_type', res.reservation_type::text,
	'reservation_type_other', res.reservation_type_other,
	'program_other', res.program_other,
	'floor', res.floor,
	'conditions', res.conditions,
	'spots_count', res.spots_count,
	'additional_benefits', res.additional_benefits,
	'program', CASE WHEN prog.program_id IS NULL THEN NULL ELSE json_build_object(
		'name', prog.name,
		'provider', prog.provider,
		'category', prog.category::text
	) END
)
FROM reserved res
LEFT JOIN program prog ON prog.program_id = res.program_id
WHERE res.reserved_id = $1::uuid
`

func (r *postgresRepo) GetReserved(ctx context.Context, reservedID string) (*Reserved, error) {
	return scanJSON[Reserved](ctx, r.pool, getReservedSQL, reservedID, "reserved")
}

const getEVChargerSQL = `
SELECT json_build_object(
	'ev_charger_id', ev.ev_charger_id::text,
	'floor', ev.floor,
	'ev_provider', CASE WHEN ep.ev_provider_id IS NULL THEN NULL ELSE json_build_object(
		'name', ep.name
	) END,
	'ev_connector', COALESCE((
		SELECT json_agg(
			json_build_object(
				'connector_type', ec.connector_type::text
			)
			ORDER BY ec.ev_connector_id
		)
		FROM ev_connector ec
		WHERE ec.ev_charger_id = ev.ev_charger_id
	), '[]'::json)
)
FROM ev_charger ev
LEFT JOIN ev_provider ep ON ep.ev_provider_id = ev.ev_provider_id
WHERE ev.ev_charger_id = $1::uuid
`

func (r *postgresRepo) GetEVCharger(ctx context.Context, evChargerID string) (*EVCharger, error) {
	return scanJSON[EVCharger](ctx, r.pool, getEVChargerSQL, evChargerID, "ev charger")
}

func (r *postgresRepo) PlaceExists(ctx context.Context, placeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM places WHERE place_id = $1::uuid)`, placeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check place exists: %w", err)
	}
	return exists, nil
}

const placeReactionCountsSQL = `
SELECT
  COALESCE(SUM(CASE WHEN reaction = 'like' THEN 1 ELSE 0 END), 0)::int,
  COALESCE(SUM(CASE WHEN reaction = 'unlike' THEN 1 ELSE 0 END), 0)::int
FROM place_reactions
WHERE place_id = $1::uuid
`

const myPlaceReactionSQL = `
SELECT reaction
FROM place_reactions
WHERE place_id = $1::uuid AND user_id = $2::uuid
`

func (r *postgresRepo) loadPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error) {
	out := &PlaceReactionResponse{PlaceID: placeID}
	if err := r.pool.QueryRow(ctx, placeReactionCountsSQL, placeID).Scan(&out.LikeCount, &out.UnlikeCount); err != nil {
		return nil, fmt.Errorf("count place reactions: %w", err)
	}
	if strings.TrimSpace(userID) != "" {
		var reaction string
		err := r.pool.QueryRow(ctx, myPlaceReactionSQL, placeID, userID).Scan(&reaction)
		if err == nil {
			kind := PlaceReactionKind(reaction)
			out.MyReaction = &kind
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get my place reaction: %w", err)
		}
	}
	return out, nil
}

func (r *postgresRepo) GetPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error) {
	return r.loadPlaceReaction(ctx, placeID, userID)
}

const upsertPlaceReactionSQL = `
INSERT INTO place_reactions (place_id, user_id, reaction, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $4)
ON CONFLICT (place_id, user_id) DO UPDATE
SET reaction = EXCLUDED.reaction,
    updated_at = EXCLUDED.updated_at
`

func (r *postgresRepo) SetPlaceReaction(ctx context.Context, placeID, userID string, reaction PlaceReactionKind) (*PlaceReactionResponse, error) {
	now := time.Now().UTC()
	if _, err := r.pool.Exec(ctx, upsertPlaceReactionSQL, placeID, userID, string(reaction), now); err != nil {
		return nil, fmt.Errorf("set place reaction: %w", err)
	}
	return r.loadPlaceReaction(ctx, placeID, userID)
}

const clearPlaceReactionSQL = `
DELETE FROM place_reactions
WHERE place_id = $1::uuid AND user_id = $2::uuid
`

func (r *postgresRepo) ClearPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error) {
	if _, err := r.pool.Exec(ctx, clearPlaceReactionSQL, placeID, userID); err != nil {
		return nil, fmt.Errorf("clear place reaction: %w", err)
	}
	return r.loadPlaceReaction(ctx, placeID, userID)
}

func scanJSON[T any](ctx context.Context, pool *pgxpool.Pool, sql string, id string, label string) (*T, error) {
	var raw []byte
	err := pool.QueryRow(ctx, sql, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get %s: %w", label, err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	return &value, nil
}

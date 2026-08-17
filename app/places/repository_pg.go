package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RinTanth/go-backend/app/submissions"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		h.is_closed AS today_is_closed
	FROM map_place_pins pin
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

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

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
	return &card, nil
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
					), '[]'::json),
					'signage_photos', COALESCE((
						SELECT json_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
						FROM place_images img
						WHERE img.entity_type = 'validation'
							AND img.entity_id = v.validation_id::text
							AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
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
							) END,
							'signage_photos', COALESCE((
								SELECT json_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
								FROM place_images img
								WHERE img.entity_type = 'reserved'
									AND img.entity_id = res.reserved_id::text
									AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
							), '[]'::json)
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
							'conditions', ev.conditions,
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
	'place_id', vp.place_id::text,
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
	), '[]'::json),
	'signage_photos', COALESCE((
		SELECT json_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
		FROM place_images img
		WHERE img.entity_type = 'validation'
			AND img.entity_id = v.validation_id::text
			AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
	), '[]'::json)
)
FROM validation v
INNER JOIN validation_parking vp ON vp.validation_id = v.validation_id
LEFT JOIN program prog ON prog.program_id = v.program_id
WHERE v.validation_id = $1::uuid
`

func (r *postgresRepo) GetValidation(ctx context.Context, validationID string) (*Validation, error) {
	v, err := scanJSON[Validation](ctx, r.pool, getValidationSQL, validationID, "validation")
	if v != nil && v.SignagePhotos == nil {
		v.SignagePhotos = []string{}
	}
	return v, err
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

	if in.SignagePhotos != nil {
		if err := replaceEntityImages(ctx, tx, "validation", validationID, *in.SignagePhotos, in.ChangedBy); err != nil {
			return nil, false, err
		}
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
	'place_id', pa.place_id::text,
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
	) END,
	'signage_photos', COALESCE((
		SELECT json_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
		FROM place_images img
		WHERE img.entity_type = 'reserved'
			AND img.entity_id = res.reserved_id::text
			AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
	), '[]'::json)
)
FROM reserved res
INNER JOIN parking_area pa ON pa.parking_area_id = res.parking_area_id
LEFT JOIN program prog ON prog.program_id = res.program_id
WHERE res.reserved_id = $1::uuid
`

func (r *postgresRepo) GetReserved(ctx context.Context, reservedID string) (*Reserved, error) {
	v, err := scanJSON[Reserved](ctx, r.pool, getReservedSQL, reservedID, "reserved")
	if v != nil && v.SignagePhotos == nil {
		v.SignagePhotos = []string{}
	}
	return v, err
}

const updateReservedSQL = `
UPDATE reserved
SET reservation_type = $2::reservation_type_enum,
    program_id = NULL,
    program_other = $3,
    conditions = $4,
    floor = $5
WHERE reserved_id = $1::uuid
`

const hasReservedCorrectionSQL = `
SELECT EXISTS(
  SELECT 1 FROM audit_log
  WHERE entity_type = 'reserved'
    AND entity_id = $1
    AND action = 'correct'
    AND changed_by = $2
)
`

const insertReservedAuditSQL = `
INSERT INTO audit_log (entity_type, entity_id, action, old_data, changed_by, created_at)
VALUES ('reserved', $1, 'correct', $2::jsonb, $3, NOW())
`

func (r *postgresRepo) UpdateReserved(
	ctx context.Context,
	reservedID string,
	in UpdateReservedInput,
) (*Reserved, bool, error) {
	existing, err := r.GetReserved(ctx, reservedID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, nil
	}

	oldData, err := json.Marshal(existing)
	if err != nil {
		return nil, false, fmt.Errorf("encode reserved audit: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin reserved update: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, updateReservedSQL,
		reservedID,
		in.ReservationType,
		in.ProgramOther,
		in.Conditions,
		in.Floor,
	)
	if err != nil {
		return nil, false, fmt.Errorf("update reserved: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, false, nil
	}

	var alreadyCorrected bool
	if err := tx.QueryRow(ctx, hasReservedCorrectionSQL, reservedID, in.ChangedBy).Scan(&alreadyCorrected); err != nil {
		return nil, false, fmt.Errorf("check reserved correction: %w", err)
	}

	if _, err := tx.Exec(ctx, insertReservedAuditSQL, reservedID, string(oldData), in.ChangedBy); err != nil {
		return nil, false, fmt.Errorf("insert reserved audit: %w", err)
	}

	if in.SignagePhotos != nil {
		if err := replaceEntityImages(ctx, tx, "reserved", reservedID, *in.SignagePhotos, in.ChangedBy); err != nil {
			return nil, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit reserved update: %w", err)
	}

	updated, err := r.GetReserved(ctx, reservedID)
	if err != nil {
		return nil, false, err
	}
	return updated, !alreadyCorrected, nil
}

const getRateIDSQL = `
SELECT r.rate_id::text
FROM rate r
INNER JOIN parking_area pa ON pa.parking_area_id = r.parking_area_id
WHERE pa.place_id = $1::uuid
ORDER BY pa.parking_area_id
LIMIT 1
`

const updateRateSQL = `
UPDATE rate
SET free_minutes = $2,
    lost_ticket_fee = $3,
    night_rate = $4,
    notes = $5
WHERE rate_id = $1::uuid
`

const deleteRateTiersSQL = `
DELETE FROM rate_tier
WHERE rate_id = $1::uuid
`

const insertRateTierSQL = `
INSERT INTO rate_tier (
	rate_id, tier_order, price, unit, from_hour, to_hour
) VALUES (
	$1::uuid, $2, $3, $4::rate_unit_enum, $5, $6
)
`

const hasRateCorrectionSQL = `
SELECT EXISTS(
  SELECT 1 FROM audit_log
  WHERE entity_type = 'rate'
    AND entity_id = $1
    AND action = 'correct'
    AND changed_by = $2
)
`

const insertRateAuditSQL = `
INSERT INTO audit_log (entity_type, entity_id, action, old_data, changed_by, created_at)
VALUES ('rate', $1, 'correct', $2::jsonb, $3, NOW())
`

func (r *postgresRepo) UpdateRate(
	ctx context.Context,
	placeID string,
	in UpdateRateInput,
) (*PlaceRateDetail, bool, error) {
	existing, err := r.GetPlaceRate(ctx, placeID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, nil
	}

	var rateID string
	if err := r.pool.QueryRow(ctx, getRateIDSQL, placeID).Scan(&rateID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lookup rate id: %w", err)
	}

	oldData, err := json.Marshal(existing)
	if err != nil {
		return nil, false, fmt.Errorf("encode rate audit: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin rate update: %w", err)
	}
	defer tx.Rollback(ctx)

	var freeMinutes any = nil
	if in.FreeMinutes != nil {
		freeMinutes = *in.FreeMinutes
	}
	var lostTicketFee any = nil
	if in.LostTicketFee != nil {
		lostTicketFee = *in.LostTicketFee
	}
	var overnightFee any = nil
	if in.OvernightFee != nil {
		overnightFee = *in.OvernightFee
	}
	var notes any = nil
	if in.Notes != nil {
		notes = *in.Notes
	}

	tag, err := tx.Exec(ctx, updateRateSQL,
		rateID,
		freeMinutes,
		lostTicketFee,
		overnightFee,
		notes,
	)
	if err != nil {
		return nil, false, fmt.Errorf("update rate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, false, nil
	}

	if _, err := tx.Exec(ctx, deleteRateTiersSQL, rateID); err != nil {
		return nil, false, fmt.Errorf("delete rate tiers: %w", err)
	}

	for i, tier := range in.RateTiers {
		var toHour any = nil
		if tier.ToHour != nil {
			toHour = *tier.ToHour
		}
		if _, err := tx.Exec(ctx, insertRateTierSQL,
			rateID,
			i+1,
			tier.Price,
			tier.Unit,
			tier.FromHour,
			toHour,
		); err != nil {
			return nil, false, fmt.Errorf("insert rate tier: %w", err)
		}
	}

	var alreadyCorrected bool
	if err := tx.QueryRow(ctx, hasRateCorrectionSQL, rateID, in.ChangedBy).Scan(&alreadyCorrected); err != nil {
		return nil, false, fmt.Errorf("check rate correction: %w", err)
	}

	// Audit after successful change, but before commit — same semantics as
	// UpdateValidation/UpdateReserved.
	if _, err := tx.Exec(ctx, insertRateAuditSQL, rateID, string(oldData), in.ChangedBy); err != nil {
		return nil, false, fmt.Errorf("insert rate audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit rate update: %w", err)
	}

	updated, err := r.GetPlaceRate(ctx, placeID)
	if err != nil {
		return nil, false, err
	}
	return updated, !alreadyCorrected, nil
}

const getParkingAreaAmenitiesSQL = `
SELECT
	pa.parking_area_id::text,
	pa.total_spaces,
	pa.has_ev_charging,
	pa.has_cover,
	pa.has_valet,
	pa.transit_access,
	pa.transit_access_type
FROM parking_area pa
WHERE pa.place_id = $1::uuid
ORDER BY pa.parking_area_id
LIMIT 1
`

type parkingAreaAmenitiesRow struct {
	ParkingAreaID     string
	TotalSpaces       *int
	HasEVCharging     *bool
	HasCover          *bool
	HasValet          *bool
	TransitAccess     *bool
	TransitAccessType *string
}

const updateParkingAreaAmenitiesSQL = `
UPDATE parking_area
SET
	has_ev_charging = $2,
	has_cover = $3,
	has_valet = $4,
	total_spaces = $5,
	transit_access = $6,
	transit_access_type = $7
WHERE parking_area_id = $1::uuid
`

const hasParkingAreaAmenitiesCorrectionSQL = `
SELECT EXISTS(
  SELECT 1 FROM audit_log
  WHERE entity_type = 'parking_area'
    AND entity_id = $1
    AND action = 'correct'
    AND changed_by = $2
)
`

const insertParkingAreaAmenitiesAuditSQL = `
INSERT INTO audit_log (entity_type, entity_id, action, old_data, changed_by, created_at)
VALUES ('parking_area', $1, 'correct', $2::jsonb, $3, NOW())
`

func (r *postgresRepo) UpdateParkingAmenities(
	ctx context.Context,
	placeID string,
	in UpdateParkingAmenitiesInput,
) (*ParkingAmenitiesCorrectionResult, bool, error) {
	var row parkingAreaAmenitiesRow
	if err := r.pool.QueryRow(ctx, getParkingAreaAmenitiesSQL, placeID).Scan(
		&row.ParkingAreaID,
		&row.TotalSpaces,
		&row.HasEVCharging,
		&row.HasCover,
		&row.HasValet,
		&row.TransitAccess,
		&row.TransitAccessType,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get parking area amenities: %w", err)
	}

	oldData, err := json.Marshal(row)
	if err != nil {
		return nil, false, fmt.Errorf("encode parking area amenities audit: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin parking amenities update: %w", err)
	}
	defer tx.Rollback(ctx)

	alreadyCorrected := false
	if err := tx.QueryRow(ctx, hasParkingAreaAmenitiesCorrectionSQL, row.ParkingAreaID, in.ChangedBy).Scan(&alreadyCorrected); err != nil {
		return nil, false, fmt.Errorf("check parking amenities correction: %w", err)
	}

	// Normalize values: parking_area columns are non-null in practice, but use
	// null-safe pointers to match existing schema.
	hasEV := in.HasEvCharging
	hasCover := in.HasCover
	hasValet := in.HasValet
	transitAccess := in.TransitAccess
	transitType := any(in.TransitAccessType)

	tag, err := tx.Exec(ctx, updateParkingAreaAmenitiesSQL,
		row.ParkingAreaID,
		hasEV,
		hasCover,
		hasValet,
		in.TotalSpaces,
		transitAccess,
		transitType,
	)
	if err != nil {
		return nil, false, fmt.Errorf("update parking amenities: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, false, nil
	}

	if _, err := tx.Exec(ctx, insertParkingAreaAmenitiesAuditSQL,
		row.ParkingAreaID,
		string(oldData),
		in.ChangedBy,
	); err != nil {
		return nil, false, fmt.Errorf("insert parking amenities audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit parking amenities update: %w", err)
	}

	// No updated value needed for the client; only points + firstCorrection.
	return &ParkingAmenitiesCorrectionResult{}, !alreadyCorrected, nil
}

const getEVChargerSQL = `
SELECT json_build_object(
	'ev_charger_id', ev.ev_charger_id::text,
	'place_id', COALESCE(ev.place_id::text, pa.place_id::text, ''),
	'floor', ev.floor,
	'conditions', ev.conditions,
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
	), '[]'::json),
	'signage_photos', COALESCE((
		SELECT json_agg(img.storage_path ORDER BY img.is_primary DESC, img.created_at)
		FROM place_images img
		WHERE img.entity_type = 'ev_charger'
			AND img.entity_id = ev.ev_charger_id::text
			AND NULLIF(BTRIM(img.storage_path), '') IS NOT NULL
	), '[]'::json)
)
FROM ev_charger ev
LEFT JOIN ev_provider ep ON ep.ev_provider_id = ev.ev_provider_id
LEFT JOIN parking_area pa ON pa.parking_area_id = ev.parking_area_id
WHERE ev.ev_charger_id = $1::uuid
`

func (r *postgresRepo) GetEVCharger(ctx context.Context, evChargerID string) (*EVCharger, error) {
	v, err := scanJSON[EVCharger](ctx, r.pool, getEVChargerSQL, evChargerID, "ev charger")
	if v != nil && v.SignagePhotos == nil {
		v.SignagePhotos = []string{}
	}
	if v != nil && v.EVConnector == nil {
		v.EVConnector = []EVConnector{}
	}
	return v, err
}

const updateEVChargerSQL = `
UPDATE ev_charger
SET ev_provider_id = $2::uuid,
    floor = $3,
    conditions = $4
WHERE ev_charger_id = $1::uuid
`

const deleteEVConnectorsSQL = `
DELETE FROM ev_connector
WHERE ev_charger_id = $1::uuid
`

const insertEVConnectorSQL = `
INSERT INTO ev_connector (
	ev_charger_id, connector_type, power_type, power_kw, is_operational
) VALUES (
	$1::uuid, $2::connector_type_enum, $3::power_type_enum, $4, true
)
`

const findEVProviderSQL = `
SELECT ev_provider_id::text
FROM ev_provider
WHERE name = $1
LIMIT 1
`

const insertEVProviderSQL = `
INSERT INTO ev_provider (name)
VALUES ($1)
RETURNING ev_provider_id::text
`

const hasEVCorrectionSQL = `
SELECT EXISTS(
  SELECT 1 FROM audit_log
  WHERE entity_type = 'ev_charger'
    AND entity_id = $1
    AND action = 'correct'
    AND changed_by = $2
)
`

const insertEVAuditSQL = `
INSERT INTO audit_log (entity_type, entity_id, action, old_data, changed_by, created_at)
VALUES ('ev_charger', $1, 'correct', $2::jsonb, $3, NOW())
`

func (r *postgresRepo) ensureEVProviderID(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var providerID string
	err := tx.QueryRow(ctx, findEVProviderSQL, name).Scan(&providerID)
	if err == nil {
		return providerID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find ev provider: %w", err)
	}
	if err := tx.QueryRow(ctx, insertEVProviderSQL, name).Scan(&providerID); err != nil {
		return "", fmt.Errorf("insert ev provider: %w", err)
	}
	return providerID, nil
}

func (r *postgresRepo) UpdateEVCharger(
	ctx context.Context,
	evChargerID string,
	in UpdateEVInput,
) (*EVCharger, bool, error) {
	existing, err := r.GetEVCharger(ctx, evChargerID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, nil
	}

	oldData, err := json.Marshal(existing)
	if err != nil {
		return nil, false, fmt.Errorf("encode ev charger audit: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin ev charger update: %w", err)
	}
	defer tx.Rollback(ctx)

	providerID, err := r.ensureEVProviderID(ctx, tx, in.ProviderName)
	if err != nil {
		return nil, false, err
	}

	tag, err := tx.Exec(ctx, updateEVChargerSQL, evChargerID, providerID, in.Floor, in.Conditions)
	if err != nil {
		return nil, false, fmt.Errorf("update ev charger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, false, nil
	}

	if _, err := tx.Exec(ctx, deleteEVConnectorsSQL, evChargerID); err != nil {
		return nil, false, fmt.Errorf("delete ev connectors: %w", err)
	}
	for _, connector := range in.Connectors {
		if _, err := tx.Exec(ctx, insertEVConnectorSQL,
			evChargerID,
			connector.ConnectorType,
			connector.PowerType,
			connector.PowerKW,
		); err != nil {
			return nil, false, fmt.Errorf("insert ev connector: %w", err)
		}
	}

	var alreadyCorrected bool
	if err := tx.QueryRow(ctx, hasEVCorrectionSQL, evChargerID, in.ChangedBy).Scan(&alreadyCorrected); err != nil {
		return nil, false, fmt.Errorf("check ev correction: %w", err)
	}

	if _, err := tx.Exec(ctx, insertEVAuditSQL, evChargerID, string(oldData), in.ChangedBy); err != nil {
		return nil, false, fmt.Errorf("insert ev charger audit: %w", err)
	}

	if in.SignagePhotos != nil {
		if err := replaceEntityImages(ctx, tx, "ev_charger", evChargerID, *in.SignagePhotos, in.ChangedBy); err != nil {
			return nil, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit ev charger update: %w", err)
	}

	updated, err := r.GetEVCharger(ctx, evChargerID)
	if err != nil {
		return nil, false, err
	}
	return updated, !alreadyCorrected, nil
}

const getParkingAreaForPlaceSQL = `
SELECT parking_area_id::text, latitude::float8, longitude::float8
FROM parking_area
WHERE place_id = $1::uuid
ORDER BY parking_area_id
LIMIT 1
`

func (r *postgresRepo) GetParkingAreaForPlace(ctx context.Context, placeID string) (*ParkingAreaRef, error) {
	var out ParkingAreaRef
	err := r.pool.QueryRow(ctx, getParkingAreaForPlaceSQL, placeID).Scan(
		&out.ParkingAreaID,
		&out.Latitude,
		&out.Longitude,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get parking area for place: %w", err)
	}
	return &out, nil
}

func (r *postgresRepo) CreatePrivilege(ctx context.Context, in CreatePrivilegeInput) error {
	userID := in.UserID
	return submissions.ContributePrivilege(ctx, r.pool, submissions.ContributeInput{
		PlaceID:       in.PlaceID,
		ParkingAreaID: in.ParkingAreaID,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
		UserID:        &userID,
		Kind:          in.Kind,
		Value:         in.Value,
	})
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

const deleteEntityImagesSQL = `
DELETE FROM place_images
WHERE entity_type = $1 AND entity_id = $2
`

const insertPrivilegeImageSQL = `
INSERT INTO place_images (
	entity_type, entity_id, storage_path, is_primary, is_verified, uploaded_by
) VALUES (
	$1, $2, $3, $4, false, $5::uuid
)
`

func replaceEntityImages(
	ctx context.Context,
	tx pgx.Tx,
	entityType string,
	entityID string,
	urls []string,
	uploadedBy string,
) error {
	if _, err := tx.Exec(ctx, deleteEntityImagesSQL, entityType, entityID); err != nil {
		return fmt.Errorf("delete %s images: %w", entityType, err)
	}
	for i, photoURL := range urls {
		trimmed := strings.TrimSpace(photoURL)
		if trimmed == "" {
			continue
		}
		if _, err := tx.Exec(ctx, insertPrivilegeImageSQL,
			entityType,
			entityID,
			trimmed,
			i == 0,
			uploadedBy,
		); err != nil {
			return fmt.Errorf("insert %s image: %w", entityType, err)
		}
	}
	return nil
}

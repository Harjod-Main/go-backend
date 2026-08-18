package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

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

package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RinTanth/go-backend/app/submissions"
	"github.com/jackc/pgx/v5"
)

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

const walkInStampFreeMinutesSQL = `
SELECT
	vp.place_id::text,
	CASE
		WHEN BOOL_OR(vt.free_minutes = -1) THEN -1
		ELSE COALESCE(MAX(vt.free_minutes) FILTER (WHERE vt.free_minutes > 0), 0)
	END
FROM validation_parking vp
INNER JOIN validation_tier vt ON vt.validation_id = vp.validation_id
WHERE vp.place_id = ANY($1::uuid[])
	AND COALESCE(vt.min_spend, 0) <= 0
GROUP BY vp.place_id
`

func (r *postgresRepo) WalkInStampFreeMinutes(ctx context.Context, placeIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(placeIDs))
	if len(placeIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, walkInStampFreeMinutesSQL, placeIDs)
	if err != nil {
		return nil, fmt.Errorf("walk-in stamp free minutes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var placeID string
		var minutes int
		if err := rows.Scan(&placeID, &minutes); err != nil {
			return nil, fmt.Errorf("scan walk-in stamp free minutes: %w", err)
		}
		out[placeID] = minutes
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate walk-in stamp free minutes: %w", err)
	}
	return out, nil
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
INNER JOIN places pl ON pl.place_id = pa.place_id
LEFT JOIN program prog ON prog.program_id = res.program_id
WHERE res.reserved_id = $1::uuid
	AND COALESCE(pl.is_blacklisted, false) = false
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
  AND EXISTS (
    SELECT 1
    FROM parking_area pa
    INNER JOIN places pl ON pl.place_id = pa.place_id
    WHERE pa.parking_area_id = reserved.parking_area_id
      AND COALESCE(pl.is_blacklisted, false) = false
  )
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

const getParkingAreaForPlaceSQL = `
SELECT pa.parking_area_id::text, pa.latitude::float8, pa.longitude::float8
FROM parking_area pa
INNER JOIN places pl ON pl.place_id = pa.place_id
WHERE pa.place_id = $1::uuid
	AND COALESCE(pl.is_blacklisted, false) = false
ORDER BY pa.parking_area_id
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
	err := submissions.ContributePrivilege(ctx, r.pool, submissions.ContributeInput{
		PlaceID:       in.PlaceID,
		ParkingAreaID: in.ParkingAreaID,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
		UserID:        &userID,
		Kind:          in.Kind,
		Value:         in.Value,
	})
	if errors.Is(err, submissions.ErrPlaceNotFound) {
		return ErrPlaceNotFound
	}
	return err
}

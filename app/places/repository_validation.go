package places

import (
	"context"
	"encoding/json"
	"fmt"
)

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

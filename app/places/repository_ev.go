package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

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

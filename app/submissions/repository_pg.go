package submissions

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

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const insertPlaceSQL = `
INSERT INTO places (
	name_en, name_th, place_type, latitude, longitude,
	address_en, address_th,
	subdistrict_en, subdistrict_th, district_en, district_th,
	province_en, province_th, postal_code,
	is_verified, confidence, collection_method,
	data_source_record_id, geom
) VALUES (
	$1, $2, $3::place_type_enum, $4, $5,
	$6, $7,
	$8, $9, $10, $11,
	$12, $13, $14,
	false, 'medium'::confidence_enum, 'community_contribution'::collection_method_enum,
	$15, ST_SetSRID(ST_MakePoint($5, $4), 4326)::geography
)
RETURNING place_id::text
`

const insertParkingAreaSQL = `
INSERT INTO parking_area (
	place_id, latitude, longitude,
	has_ev_charging, has_cover, transit_access, transit_access_type
) VALUES (
	$1::uuid, $2, $3, $4, $5, $6, $7
)
RETURNING parking_area_id::text
`

const insertHoursSQL = `
INSERT INTO hours (parking_area_id, day_of_week, open_time, close_time, is_closed)
VALUES ($1::uuid, $2::day_of_week_enum, $3::time, $4::time, $5)
`

const insertRateSQL = `
INSERT INTO rate (
	name_en, parking_area_id, free_minutes, lost_ticket_fee, night_rate, currency, notes
) VALUES (
	$1, $2::uuid, $3, $4, $5, 'THB', $6
)
RETURNING rate_id::text
`

const insertRateTierSQL = `
INSERT INTO rate_tier (rate_id, tier_order, price, unit, from_hour, to_hour)
VALUES ($1::uuid, $2, $3, $4::rate_unit_enum, $5, $6)
`

const insertEntityImageSQL = `
INSERT INTO place_images (
	entity_type, entity_id, storage_path, is_primary, is_verified, uploaded_by
) VALUES (
	$1, $2, $3, $4, false, $5::uuid
)
`

const insertProgramSQL = `
INSERT INTO program (name, provider, category)
VALUES ($1, $2, $3::program_category_enum)
RETURNING program_id::text
`

const findProgramSQL = `
SELECT program_id::text
FROM program
WHERE name = $1 AND provider = $2 AND category = $3::program_category_enum
LIMIT 1
`

const insertValidationSQL = `
INSERT INTO validation (
	validation_type, program_id, program_other, condition_description, validation_location, notes
) VALUES (
	$1::validation_type_enum, $2::uuid, $3, $4, $5, $6
)
RETURNING validation_id::text
`

const insertValidationParkingSQL = `
INSERT INTO validation_parking (validation_id, place_id)
VALUES ($1::uuid, $2::uuid)
`

const insertValidationTierSQL = `
INSERT INTO validation_tier (validation_id, tier_order, min_spend, free_minutes)
VALUES ($1::uuid, $2, $3, $4)
`

const insertReservedSQL = `
INSERT INTO reserved (
	parking_area_id, reservation_type, program_other, conditions, floor, spots_count
) VALUES (
	$1::uuid, $2::reservation_type_enum, $3, $4, $5, 1
)
RETURNING reserved_id::text
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

const insertEVChargerSQL = `
INSERT INTO ev_charger (
	place_id, parking_area_id, ev_provider_id, floor, latitude, longitude, geom
) VALUES (
	$1::uuid, $2::uuid, $3::uuid, $4, $5, $6,
	ST_SetSRID(ST_MakePoint($6, $5), 4326)::geography
)
RETURNING ev_charger_id::text
`

const insertEVConnectorSQL = `
INSERT INTO ev_connector (
	ev_charger_id, connector_type, power_type, power_kw, is_operational
) VALUES (
	$1::uuid, $2::connector_type_enum, $3::power_type_enum, $4, true
)
`

const markParkingAreaHasEVSQL = `
UPDATE parking_area
SET has_ev_charging = true
WHERE parking_area_id = $1::uuid
`

const insertSubmissionSQL = `
INSERT INTO place_submissions (
	user_id, name, name_th, name_en, google_place_id,
	address, address_th, address_en,
	subdistrict_th, subdistrict_en, district_th, district_en,
	province_th, province_en, postal_code,
	latitude, longitude, place_type,
	amenities, photo_urls, rate_photo_urls, lost_ticket_fee, overnight_fee,
	free_minutes, opening_hours, rate_tiers, special_conditions,
	parking_stamps, parking_reserved, parking_ev_charges,
	status, place_id, created_at, updated_at
) VALUES (
	$1::uuid, $2, $3, $4, $5,
	$6, $7, $8,
	$9, $10, $11, $12,
	$13, $14, $15,
	$16, $17, $18,
	$19, $20, $21, $22, $23,
	$24, $25::jsonb, $26::jsonb, $27,
	$28::jsonb, $29::jsonb, $30::jsonb,
	'approved', $31::uuid, $32, $32
)
RETURNING submission_id::text, status, created_at
`

func (r *postgresRepo) Create(ctx context.Context, s *Submission) error {
	now := time.Now()
	s.CreatedAt = now

	amenities := s.Amenities
	if amenities == nil {
		amenities = []string{}
	}
	photos := s.PhotoURLs
	if photos == nil {
		photos = []string{}
	}
	ratePhotos := s.RatePhotoURLs
	if ratePhotos == nil {
		ratePhotos = []string{}
	}
	special := s.SpecialConditions
	if special == nil {
		special = []string{}
	}

	openingHoursJSON := jsonOrEmptyObject(s.OpeningHours)
	rateTiersJSON := jsonOrEmptyArray(s.RateTiers)
	stamps := jsonOrEmptyArray(s.ParkingStamps)
	reserved := jsonOrEmptyArray(s.ParkingReserved)
	ev := jsonOrEmptyArray(s.ParkingEvCharges)

	nameTh := derefOr(s.NameTh, s.Name)
	nameEn := derefOr(s.NameEn, s.Name)
	addressTh := derefOr(s.AddressTh, derefOr(s.Address, s.Name))
	addressEn := derefOr(s.AddressEn, derefOr(s.Address, s.Name))
	provinceTh := derefOr(s.ProvinceTh, "กรุงเทพมหานคร")
	provinceEn := derefOr(s.ProvinceEn, "Bangkok")
	placeType := mapPlaceType(s.PlaceType)
	hasEV, hasTransit, hasCover, transitType := amenitiesFlags(amenities)
	hours := parseOpeningHours(s.OpeningHours)
	tiers := parseRateTiers(s.RateTiers)
	freeMinutes := 0
	if s.FreeMinutes != nil {
		freeMinutes = *s.FreeMinutes
	}
	lostTicket := parseMoney(s.LostTicketFee)
	nightRate := parseMoney(s.OvernightFee)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin publish tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var placeID string
	if err := tx.QueryRow(ctx, insertPlaceSQL,
		nameEn,
		nameTh,
		placeType,
		s.Latitude,
		s.Longitude,
		nullIfEmpty(addressEn),
		nullIfEmpty(addressTh),
		nullIfEmptyPtr(s.SubdistrictEn),
		nullIfEmptyPtr(s.SubdistrictTh),
		nullIfEmptyPtr(s.DistrictEn),
		nullIfEmptyPtr(s.DistrictTh),
		provinceEn,
		provinceTh,
		nullIfEmptyPtr(s.PostalCode),
		nullIfEmptyPtr(s.GooglePlaceID),
	).Scan(&placeID); err != nil {
		return fmt.Errorf("insert place: %w", err)
	}

	var parkingAreaID string
	if err := tx.QueryRow(ctx, insertParkingAreaSQL,
		placeID,
		s.Latitude,
		s.Longitude,
		hasEV,
		hasCover,
		hasTransit,
		transitType,
	).Scan(&parkingAreaID); err != nil {
		return fmt.Errorf("insert parking area: %w", err)
	}

	for _, hour := range hours {
		if _, err := tx.Exec(ctx, insertHoursSQL,
			parkingAreaID,
			hour.DayOfWeek,
			hour.OpenTime,
			hour.CloseTime,
			hour.IsClosed,
		); err != nil {
			return fmt.Errorf("insert hours: %w", err)
		}
	}

	var rateID string
	if err := tx.QueryRow(ctx, insertRateSQL,
		"Standard rate",
		parkingAreaID,
		freeMinutes,
		lostTicket,
		nightRate,
		joinSpecialConditions(special),
	).Scan(&rateID); err != nil {
		return fmt.Errorf("insert rate: %w", err)
	}

	for _, tier := range tiers {
		if _, err := tx.Exec(ctx, insertRateTierSQL,
			rateID,
			tier.TierOrder,
			tier.Price,
			tier.Unit,
			tier.FromHour,
			tier.ToHour,
		); err != nil {
			return fmt.Errorf("insert rate tier: %w", err)
		}
	}

	if err := insertEntityImages(ctx, tx, "place", placeID, photos, s.UserID); err != nil {
		return err
	}
	if err := insertEntityImages(ctx, tx, "rate", rateID, ratePhotos, s.UserID); err != nil {
		return err
	}

	stampsPublished := parseStampEntries(stamps)
	reservedPublished := parseReservedEntries(reserved)
	evPublished := parseEVEntries(ev)

	if err := publishStamps(ctx, tx, placeID, s.UserID, stampsPublished); err != nil {
		return err
	}
	if err := publishReserved(ctx, tx, parkingAreaID, s.UserID, reservedPublished); err != nil {
		return err
	}
	if err := publishEVCharges(ctx, tx, placeID, parkingAreaID, s.Latitude, s.Longitude, s.UserID, evPublished); err != nil {
		return err
	}
	if len(evPublished) > 0 || hasEV {
		if _, err := tx.Exec(ctx, markParkingAreaHasEVSQL, parkingAreaID); err != nil {
			return fmt.Errorf("mark parking area has EV: %w", err)
		}
	}

	if err := tx.QueryRow(ctx, insertSubmissionSQL,
		s.UserID,
		s.Name,
		s.NameTh,
		s.NameEn,
		s.GooglePlaceID,
		s.Address,
		s.AddressTh,
		s.AddressEn,
		s.SubdistrictTh,
		s.SubdistrictEn,
		s.DistrictTh,
		s.DistrictEn,
		s.ProvinceTh,
		s.ProvinceEn,
		s.PostalCode,
		s.Latitude,
		s.Longitude,
		s.PlaceType,
		amenities,
		photos,
		ratePhotos,
		s.LostTicketFee,
		s.OvernightFee,
		s.FreeMinutes,
		string(openingHoursJSON),
		string(rateTiersJSON),
		special,
		string(stamps),
		string(reserved),
		string(ev),
		placeID,
		now,
	).Scan(&s.SubmissionID, &s.Status, &s.CreatedAt); err != nil {
		return fmt.Errorf("insert place submission: %w", err)
	}

	s.PlaceID = &placeID

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit publish tx: %w", err)
	}
	return nil
}

func jsonOrEmptyObject(raw json.RawMessage) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	return raw
}

func jsonOrEmptyArray(raw json.RawMessage) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("[]")
	}
	return raw
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullIfEmptyPtr(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return *value
}

func insertEntityImages(
	ctx context.Context,
	tx pgx.Tx,
	entityType string,
	entityID string,
	urls []string,
	uploadedBy *string,
) error {
	for i, photoURL := range urls {
		trimmed := strings.TrimSpace(photoURL)
		if trimmed == "" {
			continue
		}
		if _, err := tx.Exec(ctx, insertEntityImageSQL,
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

func ensureProgramID(
	ctx context.Context,
	tx pgx.Tx,
	name string,
	provider string,
	category string,
) (string, error) {
	var programID string
	err := tx.QueryRow(ctx, findProgramSQL, name, provider, category).Scan(&programID)
	if err == nil {
		return programID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find program: %w", err)
	}
	if err := tx.QueryRow(ctx, insertProgramSQL, name, provider, category).Scan(&programID); err != nil {
		return "", fmt.Errorf("insert program: %w", err)
	}
	return programID, nil
}

func ensureEVProviderID(ctx context.Context, tx pgx.Tx, name string) (string, error) {
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

func publishStamps(
	ctx context.Context,
	tx pgx.Tx,
	placeID string,
	uploadedBy *string,
	stamps []publishedStamp,
) error {
	for _, stamp := range stamps {
		var programID any
		if stamp.ProgramName != nil && stamp.ProgramProvider != nil && stamp.ProgramCategory != nil {
			id, err := ensureProgramID(ctx, tx, *stamp.ProgramName, *stamp.ProgramProvider, *stamp.ProgramCategory)
			if err != nil {
				return err
			}
			programID = id
		}

		var validationID string
		if err := tx.QueryRow(ctx, insertValidationSQL,
			stamp.ValidationType,
			programID,
			stamp.ProgramOther,
			stamp.ConditionDescription,
			stamp.ValidationLocation,
			stamp.Notes,
		).Scan(&validationID); err != nil {
			return fmt.Errorf("insert validation: %w", err)
		}

		if _, err := tx.Exec(ctx, insertValidationParkingSQL, validationID, placeID); err != nil {
			return fmt.Errorf("insert validation_parking: %w", err)
		}

		if stamp.FreeMinutes != nil || stamp.MinSpend > 0 {
			freeMinutes := 0
			if stamp.FreeMinutes != nil {
				freeMinutes = *stamp.FreeMinutes
			}
			if _, err := tx.Exec(ctx, insertValidationTierSQL,
				validationID,
				1,
				stamp.MinSpend,
				freeMinutes,
			); err != nil {
				return fmt.Errorf("insert validation_tier: %w", err)
			}
		}

		if err := insertEntityImages(ctx, tx, "validation", validationID, stamp.SignagePhotos, uploadedBy); err != nil {
			return err
		}
	}
	return nil
}

func publishReserved(
	ctx context.Context,
	tx pgx.Tx,
	parkingAreaID string,
	uploadedBy *string,
	items []publishedReserved,
) error {
	for _, item := range items {
		var reservedID string
		if err := tx.QueryRow(ctx, insertReservedSQL,
			parkingAreaID,
			item.ReservationType,
			item.ProgramOther,
			item.Conditions,
			item.Floor,
		).Scan(&reservedID); err != nil {
			return fmt.Errorf("insert reserved: %w", err)
		}
		if err := insertEntityImages(ctx, tx, "reserved", reservedID, item.SignagePhotos, uploadedBy); err != nil {
			return err
		}
	}
	return nil
}

func publishEVCharges(
	ctx context.Context,
	tx pgx.Tx,
	placeID string,
	parkingAreaID string,
	latitude float64,
	longitude float64,
	uploadedBy *string,
	items []publishedEV,
) error {
	for _, item := range items {
		providerID, err := ensureEVProviderID(ctx, tx, item.ProviderName)
		if err != nil {
			return err
		}

		var chargerID string
		if err := tx.QueryRow(ctx, insertEVChargerSQL,
			placeID,
			parkingAreaID,
			providerID,
			item.Floor,
			latitude,
			longitude,
		).Scan(&chargerID); err != nil {
			return fmt.Errorf("insert ev charger: %w", err)
		}

		for _, connector := range item.Connectors {
			if _, err := tx.Exec(ctx, insertEVConnectorSQL,
				chargerID,
				connector.ConnectorType,
				connector.PowerType,
				connector.PowerKW,
			); err != nil {
				return fmt.Errorf("insert ev connector: %w", err)
			}
		}

		if err := insertEntityImages(ctx, tx, "ev_charger", chargerID, item.SignagePhotos, uploadedBy); err != nil {
			return err
		}
	}
	return nil
}
package submissions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const insertSQL = `
INSERT INTO place_submissions (
	user_id, name, address, latitude, longitude, place_type,
	amenities, photo_urls, rate_photo_urls, lost_ticket_fee, overnight_fee,
	free_minutes, opening_hours, rate_tiers, special_conditions,
	parking_stamps, parking_reserved, parking_ev_charges,
	status, created_at, updated_at
) VALUES (
	$1::uuid, $2, $3, $4, $5, $6,
	$7, $8, $9, $10, $11,
	$12, $13::jsonb, $14::jsonb, $15,
	$16::jsonb, $17::jsonb, $18::jsonb,
	'pending', $19, $19
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

	openingHours := jsonOrEmptyObject(s.OpeningHours)
	rateTiers := jsonOrEmptyArray(s.RateTiers)
	stamps := jsonOrEmptyArray(s.ParkingStamps)
	reserved := jsonOrEmptyArray(s.ParkingReserved)
	ev := jsonOrEmptyArray(s.ParkingEvCharges)

	// Pass JSON as text (not []byte) — pgx encodes []byte as bytea, which fails ::jsonb.
	err := r.pool.QueryRow(ctx, insertSQL,
		s.UserID,
		s.Name,
		s.Address,
		s.Latitude,
		s.Longitude,
		s.PlaceType,
		amenities,
		photos,
		ratePhotos,
		s.LostTicketFee,
		s.OvernightFee,
		s.FreeMinutes,
		string(openingHours),
		string(rateTiers),
		special,
		string(stamps),
		string(reserved),
		string(ev),
		now,
	).Scan(&s.SubmissionID, &s.Status, &s.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert place submission: %w", err)
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

package profile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,30}$`)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const getByUserIDSQL = `
SELECT user_id::text, display_name, username, avatar_url, credit_points, created_at, updated_at
FROM profiles
WHERE user_id = $1::uuid
`

func (r *postgresRepo) GetByUserID(ctx context.Context, userID string) (*Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, getByUserIDSQL, userID).Scan(
		&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreditPoints, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}

const ensureProfileSQL = `
INSERT INTO profiles (user_id, display_name, username, avatar_url, created_at, updated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $5)
ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
RETURNING user_id::text, display_name, username, avatar_url, credit_points, created_at, updated_at
`

func (r *postgresRepo) Ensure(ctx context.Context, userID, email string, seed OAuthSeed) (*Profile, error) {
	existing, err := r.GetByUserID(ctx, userID)
	if err == nil {
		return r.maybeBackfillOAuth(ctx, userID, email, existing, seed)
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	display := strings.TrimSpace(seed.DisplayName)
	if isGenericDisplayName(display) {
		display = defaultDisplayName(email)
	}
	base := defaultUsername(display)
	if isGenericUsername(base) {
		base = defaultUsername(email)
	}
	now := time.Now()

	for attempt := 0; attempt < 8; attempt++ {
		username := base
		if attempt > 0 {
			username = fmt.Sprintf("%s%d", base, attempt+1)
			if len(username) > 30 {
				username = username[:30]
			}
		}

		var p Profile
		err := r.pool.QueryRow(ctx, ensureProfileSQL, userID, display, username, seed.AvatarURL, now).Scan(
			&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreditPoints, &p.CreatedAt, &p.UpdatedAt,
		)
		if err == nil {
			return &p, nil
		}
		if isUniqueViolation(err) {
			// Race: another request created the profile, or username collision.
			if existing, getErr := r.GetByUserID(ctx, userID); getErr == nil {
				return r.maybeBackfillOAuth(ctx, userID, email, existing, seed)
			}
			continue
		}
		return nil, fmt.Errorf("ensure profile: %w", err)
	}
	return nil, fmt.Errorf("ensure profile: could not allocate username")
}

const backfillOAuthSQL = `
UPDATE profiles
SET
	display_name = COALESCE($2, display_name),
	avatar_url = COALESCE($3, avatar_url),
	updated_at = $4
WHERE user_id = $1::uuid
RETURNING user_id::text, display_name, username, avatar_url, credit_points, created_at, updated_at
`

func (r *postgresRepo) SyncFromOAuth(ctx context.Context, userID, email string, seed OAuthSeed) (*Profile, error) {
	existing, err := r.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return r.maybeBackfillOAuth(ctx, userID, email, existing, seed)
}

func (r *postgresRepo) maybeBackfillOAuth(ctx context.Context, userID string, email string, existing *Profile, seed OAuthSeed) (*Profile, error) {
	displayName := strings.TrimSpace(seed.DisplayName)
	needsDisplay := shouldBackfillDisplayName(existing.DisplayName, email, displayName)
	needsAvatar := existing.AvatarURL == nil && seed.AvatarURL != nil

	if !needsDisplay && !needsAvatar {
		return existing, nil
	}

	var displayPtr *string
	if needsDisplay {
		displayPtr = &displayName
	}
	var avatarPtr *string
	if needsAvatar {
		avatarPtr = seed.AvatarURL
	}

	now := time.Now()
	var p Profile
	err := r.pool.QueryRow(ctx, backfillOAuthSQL, userID, displayPtr, avatarPtr, now).Scan(
		&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreditPoints, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		slog.Error("oauth backfill update failed", "user_id", userID, "error", err)
		return existing, nil
	}
	return &p, nil
}

const updateProfileSQL = `
UPDATE profiles
SET
	display_name = COALESCE($2, display_name),
	username = COALESCE($3, username),
	avatar_url = CASE
		WHEN $5::boolean THEN NULL
		WHEN $4::text IS NOT NULL THEN $4
		ELSE avatar_url
	END,
	updated_at = $6
WHERE user_id = $1::uuid
RETURNING user_id::text, display_name, username, avatar_url, credit_points, created_at, updated_at
`

func (r *postgresRepo) Update(ctx context.Context, userID string, displayName, username *string, avatarURL *string, clearAvatar bool) (*Profile, error) {
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if trimmed == "" || len(trimmed) > 80 {
			return nil, ErrInvalidDisplayName
		}
		displayName = &trimmed
	}
	if username != nil {
		trimmed := strings.TrimSpace(*username)
		if !usernamePattern.MatchString(trimmed) {
			return nil, ErrInvalidUsername
		}
		username = &trimmed
	}
	if avatarURL != nil {
		trimmed := strings.TrimSpace(*avatarURL)
		if trimmed == "" {
			clearAvatar = true
			avatarURL = nil
		} else if !mediaurl.ValidAvatarValue(trimmed) {
			return nil, ErrInvalidAvatarURL
		} else {
			avatarURL = &trimmed
		}
	}

	now := time.Now()
	var p Profile
	err := r.pool.QueryRow(ctx, updateProfileSQL, userID, displayName, username, avatarURL, clearAvatar, now).Scan(
		&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreditPoints, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return &p, nil
}

const addCreditPointsSQL = `
UPDATE profiles
SET credit_points = credit_points + $2,
    updated_at = $3
WHERE user_id = $1::uuid
RETURNING credit_points
`

func (r *postgresRepo) AddCreditPoints(ctx context.Context, userID string, amount int) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("add credit points: amount must be positive")
	}
	var total int
	err := r.pool.QueryRow(ctx, addCreditPointsSQL, userID, amount, time.Now().UTC()).Scan(&total)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("add credit points: %w", err)
	}
	return total, nil
}

const listLeaderboardSQL = `
SELECT
  user_id::text,
  display_name,
  username,
  avatar_url,
  credit_points
FROM profiles
ORDER BY credit_points DESC, updated_at ASC, user_id ASC
LIMIT $1
`

func (r *postgresRepo) ListLeaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, listLeaderboardSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("list leaderboard: %w", err)
	}
	defer rows.Close()

	out := make([]LeaderboardEntry, 0, limit)
	rank := 0
	for rows.Next() {
		rank++
		var entry LeaderboardEntry
		if err := rows.Scan(
			&entry.UserID,
			&entry.DisplayName,
			&entry.Username,
			&entry.AvatarURL,
			&entry.CreditPoints,
		); err != nil {
			return nil, fmt.Errorf("scan leaderboard: %w", err)
		}
		entry.Rank = rank
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate leaderboard: %w", err)
	}
	return out, nil
}

const leaderboardRankSQL = `
SELECT
  credit_points,
  1 + (
    SELECT COUNT(*)::int
    FROM profiles p2
    WHERE p2.credit_points > p.credit_points
       OR (p2.credit_points = p.credit_points AND (
            p2.updated_at < p.updated_at
            OR (p2.updated_at = p.updated_at AND p2.user_id < p.user_id)
          ))
  ) AS rank
FROM profiles p
WHERE p.user_id = $1::uuid
`

func (r *postgresRepo) LeaderboardRank(ctx context.Context, userID string) (int, int, error) {
	var creditPoints, rank int
	err := r.pool.QueryRow(ctx, leaderboardRankSQL, userID).Scan(&creditPoints, &rank)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, ErrNotFound
		}
		return 0, 0, fmt.Errorf("leaderboard rank: %w", err)
	}
	return rank, creditPoints, nil
}

func defaultDisplayName(email string) string {
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	if email != "" {
		return email
	}
	return "User"
}

func defaultUsername(email string) string {
	raw := defaultDisplayName(email)
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) < 3 {
		out = "user"
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

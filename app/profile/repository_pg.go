package profile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

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
SELECT user_id::text, display_name, username, avatar_url, created_at, updated_at
FROM profiles
WHERE user_id = $1::uuid
`

func (r *postgresRepo) GetByUserID(ctx context.Context, userID string) (*Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, getByUserIDSQL, userID).Scan(
		&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreatedAt, &p.UpdatedAt,
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
VALUES ($1::uuid, $2, $3, NULL, $4, $4)
ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
RETURNING user_id::text, display_name, username, avatar_url, created_at, updated_at
`

func (r *postgresRepo) Ensure(ctx context.Context, userID, email string) (*Profile, error) {
	existing, err := r.GetByUserID(ctx, userID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	base := defaultUsername(email)
	display := defaultDisplayName(email)
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
		err := r.pool.QueryRow(ctx, ensureProfileSQL, userID, display, username, now).Scan(
			&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreatedAt, &p.UpdatedAt,
		)
		if err == nil {
			return &p, nil
		}
		if isUniqueViolation(err) {
			// Race: another request created the profile, or username collision.
			if existing, getErr := r.GetByUserID(ctx, userID); getErr == nil {
				return existing, nil
			}
			continue
		}
		return nil, fmt.Errorf("ensure profile: %w", err)
	}
	return nil, fmt.Errorf("ensure profile: could not allocate username")
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
RETURNING user_id::text, display_name, username, avatar_url, created_at, updated_at
`

func (r *postgresRepo) Update(ctx context.Context, userID string, displayName, username *string, avatarURL *string, clearAvatar bool) (*Profile, error) {
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if trimmed == "" || len(trimmed) > 80 {
			return nil, fmt.Errorf("invalid display name")
		}
		displayName = &trimmed
	}
	if username != nil {
		trimmed := strings.TrimSpace(*username)
		if !usernamePattern.MatchString(trimmed) {
			return nil, fmt.Errorf("invalid username")
		}
		username = &trimmed
	}
	if avatarURL != nil {
		trimmed := strings.TrimSpace(*avatarURL)
		if trimmed == "" {
			clearAvatar = true
			avatarURL = nil
		} else if len(trimmed) > 2048 {
			return nil, fmt.Errorf("invalid avatar url")
		} else {
			avatarURL = &trimmed
		}
	}

	now := time.Now()
	var p Profile
	err := r.pool.QueryRow(ctx, updateProfileSQL, userID, displayName, username, avatarURL, clearAvatar, now).Scan(
		&p.UserID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.CreatedAt, &p.UpdatedAt,
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

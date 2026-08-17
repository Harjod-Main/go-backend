package notifications

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

func NewPostgresRepo(pool *pgxpool.Pool) *postgresRepo {
	return &postgresRepo{pool: pool}
}

func (r *postgresRepo) UpsertPreferences(ctx context.Context, userID string, req NotificationPreferencesRequest) error {
	const sql = `
		INSERT INTO notification_preferences (
			user_id,
			notifications_enabled,
			in_app_alerts_enabled,
			in_app_sounds_enabled
		)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			notifications_enabled = EXCLUDED.notifications_enabled,
			in_app_alerts_enabled = EXCLUDED.in_app_alerts_enabled,
			in_app_sounds_enabled = EXCLUDED.in_app_sounds_enabled,
			updated_at = now()
	`
	_, err := r.pool.Exec(ctx, sql,
		userID,
		req.NotificationsEnabled,
		req.InAppAlertsEnabled,
		req.InAppSoundsEnabled,
	)
	if err != nil {
		return fmt.Errorf("upsert notification preferences: %w", err)
	}
	return nil
}

func (r *postgresRepo) GetPreferences(ctx context.Context, userID string) (*NotificationPreferences, error) {
	const sql = `
		SELECT
			notifications_enabled,
			in_app_alerts_enabled,
			in_app_sounds_enabled
		FROM notification_preferences
		WHERE user_id = $1::uuid
	`

	var prefs NotificationPreferences
	err := r.pool.QueryRow(ctx, sql, userID).Scan(
		&prefs.NotificationsEnabled,
		&prefs.InAppAlertsEnabled,
		&prefs.InAppSoundsEnabled,
	)
	if err != nil {
		// No preferences row yet => defaults.
		return &NotificationPreferences{
			NotificationsEnabled: false,
			InAppAlertsEnabled:   true,
			InAppSoundsEnabled:   false,
		}, nil
	}
	return &prefs, nil
}

func (r *postgresRepo) UpsertWebPushSubscription(ctx context.Context, userID string, req WebPushSubscriptionRequest) error {
	rawKeys, err := json.Marshal(req.Keys)
	if err != nil {
		return fmt.Errorf("marshal web push keys: %w", err)
	}

	const sql = `
		INSERT INTO web_push_subscriptions (user_id, endpoint, keys)
		VALUES ($1::uuid, $2, $3::jsonb)
		ON CONFLICT (user_id, endpoint) DO UPDATE SET
			keys = EXCLUDED.keys,
			updated_at = now()
	`

	_, err = r.pool.Exec(ctx, sql, userID, req.Endpoint, string(rawKeys))
	if err != nil {
		return fmt.Errorf("upsert web push subscription: %w", err)
	}
	return nil
}

func (r *postgresRepo) ListWebPushSubscriptions(ctx context.Context, userID string) ([]WebPushSubscriptionRequest, error) {
	const sql = `
		SELECT
			endpoint,
			keys->>'p256dh' AS p256dh,
			keys->>'auth' AS auth
		FROM web_push_subscriptions
		WHERE user_id = $1::uuid
	`

	rows, err := r.pool.Query(ctx, sql, userID)
	if err != nil {
		return nil, fmt.Errorf("list web push subscriptions: %w", err)
	}
	defer rows.Close()

	out := make([]WebPushSubscriptionRequest, 0)
	for rows.Next() {
		var endpoint string
		var p256dh, auth string
		if err := rows.Scan(&endpoint, &p256dh, &auth); err != nil {
			return nil, fmt.Errorf("scan web push subscription: %w", err)
		}
		out = append(out, WebPushSubscriptionRequest{
			Endpoint: endpoint,
			Keys: WebPushSubscriptionKeys{
				P256dh: p256dh,
				Auth:   auth,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list web push subscriptions rows: %w", err)
	}
	return out, nil
}

func (r *postgresRepo) DeleteWebPushSubscription(ctx context.Context, userID string, endpoint string) error {
	const sql = `
		DELETE FROM web_push_subscriptions
		WHERE user_id = $1::uuid AND endpoint = $2
	`
	_, err := r.pool.Exec(ctx, sql, userID, endpoint)
	if err != nil {
		return fmt.Errorf("delete web push subscription: %w", err)
	}
	return nil
}

func (r *postgresRepo) UpsertIOSPushToken(ctx context.Context, userID string, req IOSPushTokenRequest) error {
	const sql = `
		INSERT INTO ios_push_tokens (user_id, token)
		VALUES ($1::uuid, $2)
		ON CONFLICT (user_id, token) DO UPDATE SET
			updated_at = now()
	`
	_, err := r.pool.Exec(ctx, sql, userID, req.Token)
	if err != nil {
		return fmt.Errorf("upsert iOS push token: %w", err)
	}
	return nil
}

func (r *postgresRepo) ListIOSPushTokens(ctx context.Context, userID string) ([]IOSPushTokenRequest, error) {
	const sql = `
		SELECT token
		FROM ios_push_tokens
		WHERE user_id = $1::uuid
	`
	rows, err := r.pool.Query(ctx, sql, userID)
	if err != nil {
		return nil, fmt.Errorf("list ios push tokens: %w", err)
	}
	defer rows.Close()

	out := make([]IOSPushTokenRequest, 0)
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, fmt.Errorf("scan ios push token: %w", err)
		}
		out = append(out, IOSPushTokenRequest{Token: token})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ios push tokens rows: %w", err)
	}
	return out, nil
}

func (r *postgresRepo) DeleteIOSPushToken(ctx context.Context, userID string, token string) error {
	const sql = `
		DELETE FROM ios_push_tokens
		WHERE user_id = $1::uuid AND token = $2
	`
	_, err := r.pool.Exec(ctx, sql, userID, token)
	if err != nil {
		return fmt.Errorf("delete iOS push token: %w", err)
	}
	return nil
}

const enqueueNotificationJobSQL = `
INSERT INTO notification_jobs (user_id, payload)
VALUES ($1::uuid, $2::jsonb)
`

func (r *postgresRepo) Enqueue(ctx context.Context, userID string, evt NotificationEvent) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal notification job: %w", err)
	}
	if _, err := r.pool.Exec(ctx, enqueueNotificationJobSQL, userID, payload); err != nil {
		return fmt.Errorf("enqueue notification job: %w", err)
	}
	return nil
}

const claimNotificationJobSQL = `
WITH next_job AS (
  SELECT job_id
  FROM notification_jobs
  WHERE status = 'pending'
    AND next_attempt_at <= now()
  ORDER BY next_attempt_at ASC, created_at ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE notification_jobs j
SET status = 'processing',
    updated_at = now()
FROM next_job
WHERE j.job_id = next_job.job_id
RETURNING j.job_id::text, j.user_id::text, j.payload, j.attempts, j.max_attempts
`

func (r *postgresRepo) Claim(ctx context.Context) (*NotificationJob, error) {
	var job NotificationJob
	var payload []byte
	err := r.pool.QueryRow(ctx, claimNotificationJobSQL).Scan(
		&job.JobID, &job.UserID, &payload, &job.Attempts, &job.MaxAttempts,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim notification job: %w", err)
	}
	if err := json.Unmarshal(payload, &job.Event); err != nil {
		return nil, fmt.Errorf("decode notification job payload: %w", err)
	}
	return &job, nil
}

func (r *postgresRepo) Complete(ctx context.Context, jobID string) error {
	const sql = `
		UPDATE notification_jobs
		SET status = 'done',
		    last_error = NULL,
		    updated_at = now()
		WHERE job_id = $1::uuid
	`
	if _, err := r.pool.Exec(ctx, sql, jobID); err != nil {
		return fmt.Errorf("complete notification job: %w", err)
	}
	return nil
}

func (r *postgresRepo) Retry(ctx context.Context, jobID string, attempts int, nextAttemptAt time.Time, lastError string) error {
	const sql = `
		UPDATE notification_jobs
		SET status = 'pending',
		    attempts = $2,
		    next_attempt_at = $3,
		    last_error = $4,
		    updated_at = now()
		WHERE job_id = $1::uuid
	`
	if _, err := r.pool.Exec(ctx, sql, jobID, attempts, nextAttemptAt, truncateErr(lastError)); err != nil {
		return fmt.Errorf("retry notification job: %w", err)
	}
	return nil
}

func (r *postgresRepo) Fail(ctx context.Context, jobID string, lastError string) error {
	const sql = `
		UPDATE notification_jobs
		SET status = 'failed',
		    last_error = $2,
		    updated_at = now()
		WHERE job_id = $1::uuid
	`
	if _, err := r.pool.Exec(ctx, sql, jobID, truncateErr(lastError)); err != nil {
		return fmt.Errorf("fail notification job: %w", err)
	}
	return nil
}

func (r *postgresRepo) ReclaimStale(ctx context.Context, staleAfter time.Duration) error {
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	const sql = `
		UPDATE notification_jobs
		SET status = 'pending',
		    updated_at = now()
		WHERE status = 'processing'
		  AND updated_at < now() - ($1::double precision * interval '1 second')
	`
	if _, err := r.pool.Exec(ctx, sql, staleAfter.Seconds()); err != nil {
		return fmt.Errorf("reclaim stale notification jobs: %w", err)
	}
	return nil
}

func truncateErr(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= 1000 {
		return msg
	}
	return msg[:1000]
}

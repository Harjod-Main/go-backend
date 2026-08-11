package notifications

import (
	"context"
	"encoding/json"
	"fmt"

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


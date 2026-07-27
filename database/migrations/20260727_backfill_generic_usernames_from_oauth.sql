-- One-shot backfill: replace generic usernames ('', 'user') with OAuth-derived
-- unique usernames. Set-based (not per-row EXISTS loops) so cost stays ~O(n log n)
-- as profiles grow. No rollback — backfill is idempotent for already-filled rows.

WITH targets AS (
    SELECT
        p.user_id,
        CASE
            WHEN length(cleaned) < 3 THEN 'user'
            ELSE left(cleaned, 30)
        END AS base_username
    FROM public.profiles p
    JOIN auth.users u ON u.id = p.user_id
    CROSS JOIN LATERAL (
        SELECT regexp_replace(
            lower(
                COALESCE(
                    NULLIF(TRIM(u.raw_user_meta_data ->> 'full_name'), ''),
                    NULLIF(TRIM(u.raw_user_meta_data ->> 'name'), ''),
                    NULLIF(TRIM(u.raw_user_meta_data ->> 'display_name'), ''),
                    NULLIF(TRIM(u.raw_user_meta_data ->> 'nickname'), ''),
                    NULLIF(TRIM(p.display_name), ''),
                    'user'
                )
            ),
            '[^a-z0-9._-]+',
            '',
            'g'
        ) AS cleaned
    ) src
    WHERE lower(trim(COALESCE(p.username, ''))) IN ('', 'user')
),
reserved AS (
    -- Usernames already claimed by rows we are not rewriting.
    SELECT lower(p.username) AS username
    FROM public.profiles p
    WHERE lower(trim(COALESCE(p.username, ''))) NOT IN ('', 'user')
),
base_max_suffix AS (
    -- Highest numeric suffix already taken for each base (base itself counts as 1).
    SELECT
        t.base_username,
        COALESCE(
            MAX(
                CASE
                    WHEN r.username = lower(t.base_username) THEN 1
                    WHEN r.username LIKE lower(t.base_username) || '%'
                        AND substring(r.username FROM length(t.base_username) + 1) ~ '^[0-9]+$'
                    THEN substring(r.username FROM length(t.base_username) + 1)::int
                    ELSE NULL
                END
            ),
            0
        ) AS max_suffix
    FROM (SELECT DISTINCT base_username FROM targets) t
    LEFT JOIN reserved r
        ON r.username = lower(t.base_username)
        OR (
            r.username LIKE lower(t.base_username) || '%'
            AND substring(r.username FROM length(t.base_username) + 1) ~ '^[0-9]+$'
        )
    GROUP BY t.base_username
),
ranked AS (
    SELECT
        t.user_id,
        t.base_username,
        COALESCE(b.max_suffix, 0)
            + row_number() OVER (
                PARTITION BY t.base_username
                ORDER BY t.user_id
            ) AS suffix_num
    FROM targets t
    LEFT JOIN base_max_suffix b ON b.base_username = t.base_username
),
assigned AS (
    SELECT
        user_id,
        CASE
            WHEN suffix_num = 1 THEN base_username
            ELSE left(base_username, 30 - length(suffix_num::text)) || suffix_num::text
        END AS username
    FROM ranked
)
UPDATE public.profiles p
SET
    username = a.username,
    updated_at = NOW()
FROM assigned a
WHERE p.user_id = a.user_id
  AND lower(trim(COALESCE(p.username, ''))) IN ('', 'user');

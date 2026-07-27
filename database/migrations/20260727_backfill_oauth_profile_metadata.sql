WITH candidates AS (
    SELECT
        p.user_id,
        NULLIF(
            TRIM(
                COALESCE(
                    u.raw_user_meta_data ->> 'full_name',
                    u.raw_user_meta_data ->> 'name',
                    u.raw_user_meta_data ->> 'display_name',
                    u.raw_user_meta_data ->> 'nickname',
                    ''
                )
            ),
            ''
        ) AS oauth_display_name,
        NULLIF(
            TRIM(
                COALESCE(
                    u.raw_user_meta_data ->> 'avatar_url',
                    u.raw_user_meta_data ->> 'picture',
                    u.raw_user_meta_data ->> 'avatar',
                    ''
                )
            ),
            ''
        ) AS oauth_avatar_url
    FROM public.profiles p
    JOIN auth.users u ON u.id = p.user_id
)
UPDATE public.profiles p
SET
    display_name = CASE
        WHEN LOWER(TRIM(p.display_name)) IN ('', 'user') AND c.oauth_display_name IS NOT NULL
            THEN c.oauth_display_name
        ELSE p.display_name
    END,
    avatar_url = CASE
        WHEN p.avatar_url IS NULL AND c.oauth_avatar_url IS NOT NULL
            THEN c.oauth_avatar_url
        ELSE p.avatar_url
    END,
    updated_at = CASE
        WHEN (
            LOWER(TRIM(p.display_name)) IN ('', 'user') AND c.oauth_display_name IS NOT NULL
        ) OR (
            p.avatar_url IS NULL AND c.oauth_avatar_url IS NOT NULL
        )
            THEN NOW()
        ELSE p.updated_at
    END
FROM candidates c
WHERE p.user_id = c.user_id
  AND (
      (LOWER(TRIM(p.display_name)) IN ('', 'user') AND c.oauth_display_name IS NOT NULL)
      OR (p.avatar_url IS NULL AND c.oauth_avatar_url IS NOT NULL)
  );

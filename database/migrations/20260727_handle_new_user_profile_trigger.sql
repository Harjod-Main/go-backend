-- Create a profile row when a new auth.users row is inserted.
-- Seeds display_name / avatar_url from OAuth metadata (LINE, Google, etc.)
-- and allocates a unique username (mirrors app/profile Ensure logic).

CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $$
DECLARE
    v_display_name text;
    v_avatar_url text;
    v_base_username text;
    v_username text;
    v_suffix int := 1;
BEGIN
    v_display_name := COALESCE(
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'full_name'), ''),
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'name'), ''),
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'display_name'), ''),
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'nickname'), '')
    );

    IF v_display_name IS NULL OR LOWER(TRIM(v_display_name)) IN ('', 'user') THEN
        IF NEW.email IS NOT NULL AND POSITION('@' IN NEW.email) > 1 THEN
            v_display_name := SPLIT_PART(NEW.email, '@', 1);
        ELSIF NEW.email IS NOT NULL AND NEW.email <> '' THEN
            v_display_name := NEW.email;
        ELSE
            v_display_name := 'User';
        END IF;
    END IF;

    v_avatar_url := COALESCE(
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'avatar_url'), ''),
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'picture'), ''),
        NULLIF(TRIM(NEW.raw_user_meta_data ->> 'avatar'), '')
    );

    v_base_username := LOWER(REGEXP_REPLACE(v_display_name, '[^a-z0-9._-]+', '', 'g'));
    IF LENGTH(v_base_username) < 3 THEN
        IF NEW.email IS NOT NULL AND POSITION('@' IN NEW.email) > 1 THEN
            v_base_username := LOWER(REGEXP_REPLACE(SPLIT_PART(NEW.email, '@', 1), '[^a-z0-9._-]+', '', 'g'));
        END IF;
    END IF;
    IF LENGTH(v_base_username) < 3 THEN
        v_base_username := 'user';
    END IF;
    IF LENGTH(v_base_username) > 30 THEN
        v_base_username := LEFT(v_base_username, 30);
    END IF;

    v_username := v_base_username;
    WHILE EXISTS (
        SELECT 1
        FROM public.profiles p
        WHERE LOWER(p.username) = LOWER(v_username)
    ) LOOP
        v_suffix := v_suffix + 1;
        v_username := LEFT(v_base_username, 30 - LENGTH(v_suffix::text)) || v_suffix::text;
    END LOOP;

    INSERT INTO public.profiles (user_id, display_name, username, avatar_url)
    VALUES (NEW.id, v_display_name, v_username, v_avatar_url)
    ON CONFLICT (user_id) DO NOTHING;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS on_auth_user_created ON auth.users;

CREATE TRIGGER on_auth_user_created
    AFTER INSERT ON auth.users
    FOR EACH ROW
    EXECUTE FUNCTION public.handle_new_user();

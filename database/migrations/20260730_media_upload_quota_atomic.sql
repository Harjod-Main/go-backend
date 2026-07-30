-- Atomic media upload quota reservation (used by media-upload-url Edge Function).
-- Serializes per-user checks with a transaction-scoped advisory lock so concurrent
-- requests cannot all observe the same count and exceed the soft caps.

CREATE OR REPLACE FUNCTION public.reserve_media_upload(
  p_user_id uuid,
  p_object_path text,
  p_folder text,
  p_max_per_hour integer DEFAULT 30,
  p_max_per_day integer DEFAULT 100
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_hour_count integer;
  v_day_count integer;
  v_event_id uuid;
BEGIN
  IF p_user_id IS NULL OR p_object_path IS NULL OR length(trim(p_object_path)) = 0 THEN
    RETURN jsonb_build_object('ok', false, 'error', 'invalid_input');
  END IF;
  IF p_folder IS NULL OR length(trim(p_folder)) = 0 THEN
    RETURN jsonb_build_object('ok', false, 'error', 'invalid_input');
  END IF;
  IF p_max_per_hour IS NULL OR p_max_per_hour < 1 OR p_max_per_day IS NULL OR p_max_per_day < 1 THEN
    RETURN jsonb_build_object('ok', false, 'error', 'invalid_input');
  END IF;

  -- Namespace 872014 = media upload quota; second key hashes the user id.
  PERFORM pg_advisory_xact_lock(872014, hashtext(p_user_id::text));

  SELECT count(*)::integer
  INTO v_hour_count
  FROM public.media_upload_events
  WHERE user_id = p_user_id
    AND created_at >= now() - interval '1 hour';

  IF v_hour_count >= p_max_per_hour THEN
    RETURN jsonb_build_object(
      'ok', false,
      'error', 'rate_limited_hour',
      'hourCount', v_hour_count,
      'maxPerHour', p_max_per_hour
    );
  END IF;

  SELECT count(*)::integer
  INTO v_day_count
  FROM public.media_upload_events
  WHERE user_id = p_user_id
    AND created_at >= now() - interval '1 day';

  IF v_day_count >= p_max_per_day THEN
    RETURN jsonb_build_object(
      'ok', false,
      'error', 'rate_limited_day',
      'dayCount', v_day_count,
      'maxPerDay', p_max_per_day
    );
  END IF;

  INSERT INTO public.media_upload_events (user_id, object_path, folder)
  VALUES (p_user_id, p_object_path, p_folder)
  RETURNING id INTO v_event_id;

  RETURN jsonb_build_object(
    'ok', true,
    'eventId', v_event_id,
    'hourCount', v_hour_count + 1,
    'dayCount', v_day_count + 1
  );
END;
$$;

CREATE OR REPLACE FUNCTION public.release_media_upload(
  p_event_id uuid,
  p_user_id uuid
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_deleted integer;
BEGIN
  IF p_event_id IS NULL OR p_user_id IS NULL THEN
    RETURN false;
  END IF;

  DELETE FROM public.media_upload_events
  WHERE id = p_event_id
    AND user_id = p_user_id;

  GET DIAGNOSTICS v_deleted = ROW_COUNT;
  RETURN v_deleted > 0;
END;
$$;

REVOKE ALL ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.release_media_upload(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) TO service_role;
GRANT EXECUTE ON FUNCTION public.release_media_upload(uuid, uuid) TO service_role;

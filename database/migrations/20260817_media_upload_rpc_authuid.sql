-- Lock media upload quota RPCs to the calling user.
-- Previous migrations revoked PUBLIC and granted service_role only, but live
-- ACLs still had anon/authenticated EXECUTE (Supabase default / drift).
-- Edge Function must call these with the end-user JWT (not service_role).

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
  v_caller uuid := auth.uid();
BEGIN
  IF v_caller IS NULL OR v_caller IS DISTINCT FROM p_user_id THEN
    RETURN jsonb_build_object('ok', false, 'error', 'forbidden');
  END IF;

  IF p_user_id IS NULL OR p_object_path IS NULL OR length(trim(p_object_path)) = 0 THEN
    RETURN jsonb_build_object('ok', false, 'error', 'invalid_input');
  END IF;
  IF p_folder IS NULL OR length(trim(p_folder)) = 0 THEN
    RETURN jsonb_build_object('ok', false, 'error', 'invalid_input');
  END IF;
  IF p_max_per_hour IS NULL OR p_max_per_hour < 1 OR p_max_per_day IS NULL OR p_max_per_day < 1 THEN
    RETURN jsonb_build_object('ok', false, 'error', 'invalid_input');
  END IF;

  -- Reservations may only target the caller's own storage prefix.
  IF left(trim(p_object_path), length(p_user_id::text) + 1)
       IS DISTINCT FROM (p_user_id::text || '/') THEN
    RETURN jsonb_build_object('ok', false, 'error', 'forbidden_path');
  END IF;

  PERFORM pg_advisory_xact_lock(872014, hashtext(p_user_id::text));

  UPDATE public.media_upload_events
  SET reservation_status = 'expired'
  WHERE user_id = p_user_id
    AND reservation_status = 'reserved'
    AND reserved_until <= now();

  SELECT count(*)::integer
  INTO v_hour_count
  FROM public.media_upload_events
  WHERE user_id = p_user_id
    AND created_at >= now() - interval '1 hour'
    AND reservation_status IN ('reserved', 'completed');

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
    AND created_at >= now() - interval '1 day'
    AND reservation_status IN ('reserved', 'completed');

  IF v_day_count >= p_max_per_day THEN
    RETURN jsonb_build_object(
      'ok', false,
      'error', 'rate_limited_day',
      'dayCount', v_day_count,
      'maxPerDay', p_max_per_day
    );
  END IF;

  INSERT INTO public.media_upload_events (
    user_id, object_path, folder, reservation_status, reserved_until
  )
  VALUES (
    p_user_id, p_object_path, p_folder, 'reserved', now() + interval '2 hours'
  )
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
  v_caller uuid := auth.uid();
BEGIN
  IF v_caller IS NULL OR v_caller IS DISTINCT FROM p_user_id THEN
    RETURN false;
  END IF;

  IF p_event_id IS NULL OR p_user_id IS NULL THEN
    RETURN false;
  END IF;

  DELETE FROM public.media_upload_events
  WHERE id = p_event_id
    AND user_id = p_user_id
    AND reservation_status = 'reserved';

  GET DIAGNOSTICS v_deleted = ROW_COUNT;
  RETURN v_deleted > 0;
END;
$$;

-- Clients must not call these via PostgREST; Edge Function uses the user JWT.
REVOKE ALL ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) FROM anon;
REVOKE ALL ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) FROM authenticated;
REVOKE ALL ON FUNCTION public.release_media_upload(uuid, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.release_media_upload(uuid, uuid) FROM anon;
REVOKE ALL ON FUNCTION public.release_media_upload(uuid, uuid) FROM authenticated;

GRANT EXECUTE ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) TO authenticated;
GRANT EXECUTE ON FUNCTION public.release_media_upload(uuid, uuid) TO authenticated;

-- Trigger-only helpers: keep callable by the trigger owner, not by API roles.
REVOKE ALL ON FUNCTION public.mark_media_upload_abandoned() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.mark_media_upload_abandoned() FROM anon;
REVOKE ALL ON FUNCTION public.mark_media_upload_abandoned() FROM authenticated;
REVOKE ALL ON FUNCTION public.mark_media_upload_completed() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.mark_media_upload_completed() FROM anon;
REVOKE ALL ON FUNCTION public.mark_media_upload_completed() FROM authenticated;

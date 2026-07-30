-- Track whether a media-upload quota reservation produced a real Storage object.
-- The storage trigger below marks reservations complete server-side when the
-- signed upload actually creates storage.objects. Abandoned reservations are
-- excluded after their expiry window.

ALTER TABLE public.media_upload_events
  ADD COLUMN IF NOT EXISTS reservation_status text NOT NULL DEFAULT 'reserved',
  ADD COLUMN IF NOT EXISTS reserved_until timestamptz NOT NULL DEFAULT (now() + interval '2 hours'),
  ADD COLUMN IF NOT EXISTS completed_at timestamptz;

DO $$
BEGIN
  ALTER TABLE public.media_upload_events
    ADD CONSTRAINT media_upload_events_reservation_status_check
    CHECK (reservation_status IN ('reserved', 'completed', 'expired'));
EXCEPTION
  WHEN duplicate_object THEN NULL;
END;
$$;

CREATE INDEX IF NOT EXISTS media_upload_events_active_reservation_idx
  ON public.media_upload_events (user_id, created_at DESC)
  WHERE reservation_status IN ('reserved', 'completed');

-- Storage is the source of truth for completion. This prevents clients from
-- claiming completion without actually uploading an object.
CREATE OR REPLACE FUNCTION public.mark_media_upload_completed()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NEW.bucket_id = 'media' THEN
    UPDATE public.media_upload_events
    SET reservation_status = 'completed',
        completed_at = COALESCE(completed_at, now())
    WHERE object_path = NEW.name
      AND reservation_status IN ('reserved', 'expired');
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS media_upload_events_mark_completed ON storage.objects;
CREATE TRIGGER media_upload_events_mark_completed
  AFTER INSERT ON storage.objects
  FOR EACH ROW
  WHEN (NEW.bucket_id = 'media')
  EXECUTE FUNCTION public.mark_media_upload_completed();

REVOKE ALL ON FUNCTION public.mark_media_upload_completed() FROM PUBLIC;

-- Replace the reservation function so abandoned reservations stop counting
-- after two hours, while completed uploads continue to count for the normal
-- hourly/daily quota windows.
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

  PERFORM pg_advisory_xact_lock(872014, hashtext(p_user_id::text));

  -- Keep the audit row, but stop abandoned reservations from consuming quota.
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

-- Only an outstanding reservation may be released after signed-URL creation
-- fails. Completed uploads remain in the quota ledger.
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
    AND user_id = p_user_id
    AND reservation_status = 'reserved';

  GET DIAGNOSTICS v_deleted = ROW_COUNT;
  RETURN v_deleted > 0;
END;
$$;

REVOKE ALL ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.release_media_upload(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.reserve_media_upload(uuid, text, text, integer, integer) TO service_role;
GRANT EXECUTE ON FUNCTION public.release_media_upload(uuid, uuid) TO service_role;

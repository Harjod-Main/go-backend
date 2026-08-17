-- Bound notification_jobs growth. Successful rows are dropped after 14 days;
-- failed rows stay as a dead-letter for 90 days, then are purged in batches.
-- Claim/reclaim already use partial indexes on pending and processing.

COMMENT ON TABLE public.notification_jobs IS
  'Push outbox. done purged after 14 days; failed kept 90 days as dead-letter.';

CREATE INDEX IF NOT EXISTS notification_jobs_done_updated_idx
  ON public.notification_jobs (updated_at)
  WHERE status = 'done';

CREATE INDEX IF NOT EXISTS notification_jobs_failed_updated_idx
  ON public.notification_jobs (updated_at)
  WHERE status = 'failed';

CREATE OR REPLACE FUNCTION public.purge_notification_jobs(
  p_done_days integer DEFAULT 14,
  p_failed_days integer DEFAULT 90,
  p_batch_size integer DEFAULT 1000
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_done_days integer := least(greatest(coalesce(p_done_days, 14), 1), 90);
  v_failed_days integer := least(greatest(coalesce(p_failed_days, 90), 1), 365);
  v_batch integer := least(greatest(coalesce(p_batch_size, 1000), 1), 5000);
  v_cap integer := 20000;
  v_deleted_done integer := 0;
  v_deleted_failed integer := 0;
  v_n integer;
BEGIN
  LOOP
    WITH doomed AS (
      SELECT job_id
      FROM public.notification_jobs
      WHERE status = 'done'
        AND updated_at < now() - make_interval(days => v_done_days)
      ORDER BY updated_at ASC
      LIMIT v_batch
    )
    DELETE FROM public.notification_jobs j
    USING doomed
    WHERE j.job_id = doomed.job_id;
    GET DIAGNOSTICS v_n = ROW_COUNT;
    v_deleted_done := v_deleted_done + v_n;
    EXIT WHEN v_n = 0 OR v_deleted_done >= v_cap;
  END LOOP;

  LOOP
    WITH doomed AS (
      SELECT job_id
      FROM public.notification_jobs
      WHERE status = 'failed'
        AND updated_at < now() - make_interval(days => v_failed_days)
      ORDER BY updated_at ASC
      LIMIT v_batch
    )
    DELETE FROM public.notification_jobs j
    USING doomed
    WHERE j.job_id = doomed.job_id;
    GET DIAGNOSTICS v_n = ROW_COUNT;
    v_deleted_failed := v_deleted_failed + v_n;
    EXIT WHEN v_n = 0 OR v_deleted_failed >= v_cap;
  END LOOP;

  RAISE LOG 'notification_jobs purge deleted_done=% deleted_failed=%',
    v_deleted_done, v_deleted_failed;

  RETURN jsonb_build_object(
    'deleted_done', v_deleted_done,
    'deleted_failed', v_deleted_failed
  );
END;
$$;

CREATE OR REPLACE FUNCTION public.notification_job_stats()
RETURNS jsonb
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT jsonb_build_object(
    'pending', (
      SELECT count(*)::bigint
      FROM public.notification_jobs
      WHERE status = 'pending'
    ),
    'processing', (
      SELECT count(*)::bigint
      FROM public.notification_jobs
      WHERE status = 'processing'
    ),
    'done', (
      SELECT count(*)::bigint
      FROM public.notification_jobs
      WHERE status = 'done'
    ),
    'failed', (
      SELECT count(*)::bigint
      FROM public.notification_jobs
      WHERE status = 'failed'
    ),
    'retried', (
      SELECT count(*)::bigint
      FROM public.notification_jobs
      WHERE status = 'pending'
        AND attempts > 0
    ),
    'retry_attempts', (
      SELECT coalesce(sum(attempts), 0)::bigint
      FROM public.notification_jobs
      WHERE status IN ('pending', 'processing')
    ),
    'oldest_pending_at', (
      SELECT min(next_attempt_at)
      FROM public.notification_jobs
      WHERE status = 'pending'
    )
  );
$$;

REVOKE ALL ON FUNCTION public.purge_notification_jobs(integer, integer, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.purge_notification_jobs(integer, integer, integer) FROM anon;
REVOKE ALL ON FUNCTION public.purge_notification_jobs(integer, integer, integer) FROM authenticated;
GRANT EXECUTE ON FUNCTION public.purge_notification_jobs(integer, integer, integer) TO postgres;
GRANT EXECUTE ON FUNCTION public.purge_notification_jobs(integer, integer, integer) TO service_role;

REVOKE ALL ON FUNCTION public.notification_job_stats() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.notification_job_stats() FROM anon;
REVOKE ALL ON FUNCTION public.notification_job_stats() FROM authenticated;
GRANT EXECUTE ON FUNCTION public.notification_job_stats() TO postgres;
GRANT EXECUTE ON FUNCTION public.notification_job_stats() TO service_role;

CREATE EXTENSION IF NOT EXISTS pg_cron WITH SCHEMA pg_catalog;
GRANT USAGE ON SCHEMA cron TO postgres;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM cron.job WHERE jobname = 'purge-notification-jobs'
  ) THEN
    PERFORM cron.unschedule('purge-notification-jobs');
  END IF;

  PERFORM cron.schedule(
    'purge-notification-jobs',
    '*/10 * * * *',
    $cmd$SELECT public.purge_notification_jobs();$cmd$
  );
EXCEPTION
  WHEN OTHERS THEN
    RAISE NOTICE 'pg_cron schedule skipped: %', SQLERRM;
END;
$$;

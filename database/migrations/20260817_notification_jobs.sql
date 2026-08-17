-- Durable outbox for Web Push / Expo Push. Handlers insert a row and return;
-- go-backend workers claim jobs and retry independently of the HTTP request.

CREATE TABLE IF NOT EXISTS public.notification_jobs (
  job_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  payload jsonb NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL DEFAULT 5,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'processing', 'done', 'failed')),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notification_jobs_due_idx
  ON public.notification_jobs (next_attempt_at, created_at)
  WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS notification_jobs_processing_updated_idx
  ON public.notification_jobs (updated_at)
  WHERE status = 'processing';

ALTER TABLE public.notification_jobs ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON TABLE public.notification_jobs FROM anon, authenticated;
GRANT ALL ON TABLE public.notification_jobs TO service_role;

-- Ledger of credit-point awards. Writes go through go-backend (service_role / postgres).
-- RLS stays on as defense in depth; PostgREST roles have no access.

CREATE TABLE IF NOT EXISTS public.credit_point_events (
  event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  amount integer NOT NULL CHECK (amount > 0),
  reason text NOT NULL CHECK (reason IN (
    'check_in',
    'review',
    'place_submission',
    'correction',
    'referral'
  )),
  source_type text NOT NULL,
  source_id text NOT NULL,
  place_id uuid NULL REFERENCES public.places (place_id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS credit_point_events_source_uniq
  ON public.credit_point_events (user_id, reason, source_type, source_id);

CREATE INDEX IF NOT EXISTS credit_point_events_user_created_idx
  ON public.credit_point_events (user_id, created_at DESC, event_id DESC);

ALTER TABLE public.credit_point_events ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON TABLE public.credit_point_events FROM anon, authenticated;
GRANT ALL ON TABLE public.credit_point_events TO service_role;

INSERT INTO public.credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
)
SELECT
  ci.user_id,
  ci.points_awarded,
  'check_in',
  'check_in',
  ci.check_in_id::text,
  ci.place_id,
  ci.created_at
FROM public.check_ins ci
WHERE ci.points_awarded > 0
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING;

INSERT INTO public.credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
)
SELECT
  r.user_id,
  50,
  'review',
  'review',
  r.review_id::text,
  r.place_id,
  r.created_at
FROM public.reviews r
WHERE r.user_id IS NOT NULL
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING;

INSERT INTO public.credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
)
SELECT
  ps.user_id,
  50,
  'place_submission',
  'place_submission',
  ps.submission_id::text,
  ps.place_id,
  ps.created_at
FROM public.place_submissions ps
WHERE ps.user_id IS NOT NULL
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING;

INSERT INTO public.credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
)
SELECT DISTINCT ON (al.changed_by, al.entity_type, al.entity_id)
  al.changed_by::uuid,
  10,
  'correction',
  al.entity_type,
  al.entity_id,
  NULL,
  al.created_at
FROM public.audit_log al
WHERE al.action = 'correct'
  AND al.changed_by IS NOT NULL
  AND al.changed_by ~ '^[0-9a-fA-F-]{36}$'
  AND NULLIF(BTRIM(al.entity_id), '') IS NOT NULL
ORDER BY al.changed_by, al.entity_type, al.entity_id, al.created_at ASC
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING;

INSERT INTO public.credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
)
SELECT
  rf.referrer_user_id,
  rf.referrer_points,
  'referral',
  'referral',
  rf.referral_id::text || ':referrer',
  NULL,
  rf.created_at
FROM public.referrals rf
WHERE rf.referrer_points > 0
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING;

INSERT INTO public.credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
)
SELECT
  rf.referee_user_id,
  rf.referee_points,
  'referral',
  'referral',
  rf.referral_id::text || ':referee',
  NULL,
  rf.created_at
FROM public.referrals rf
WHERE rf.referee_points > 0
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING;

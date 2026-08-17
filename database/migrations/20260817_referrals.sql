-- Referral attribution: one accepted invite per new user.
-- Writes go through go-backend (service_role / postgres). RLS stays on as defense in depth.

CREATE TABLE IF NOT EXISTS public.referrals (
  referral_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  referrer_user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  referee_user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  invite_username text NOT NULL,
  referrer_points integer NOT NULL,
  referee_points integer NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT referrals_no_self CHECK (referrer_user_id <> referee_user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS referrals_referee_user_id_uniq
  ON public.referrals (referee_user_id);

CREATE INDEX IF NOT EXISTS referrals_referrer_created_idx
  ON public.referrals (referrer_user_id, created_at DESC);

ALTER TABLE public.referrals ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON TABLE public.referrals FROM anon, authenticated;
GRANT ALL ON TABLE public.referrals TO service_role;

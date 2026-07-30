-- Check-in MVP: store occupancy reports and award credit points on profiles.

ALTER TABLE public.profiles
  ADD COLUMN IF NOT EXISTS credit_points integer NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS public.check_ins (
  check_in_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES public.places (place_id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  occupancy text NOT NULL
    CHECK (occupancy IN ('full', 'crowded', 'normal', 'many_space')),
  satisfied boolean NOT NULL,
  edit_suggestion text NULL
    CHECK (
      edit_suggestion IS NULL
      OR edit_suggestion IN ('incorrect_name', 'incorrect_address', 'incorrect_hours', 'other')
    ),
  comment text NULL,
  points_awarded integer NOT NULL CHECK (points_awarded >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT check_ins_comment_len CHECK (comment IS NULL OR char_length(comment) <= 4000),
  CONSTRAINT check_ins_edit_when_unsatisfied CHECK (
    (satisfied = true AND edit_suggestion IS NULL)
    OR (satisfied = false AND edit_suggestion IS NOT NULL)
  )
);

CREATE INDEX IF NOT EXISTS check_ins_place_created_idx
  ON public.check_ins (place_id, created_at DESC);

CREATE INDEX IF NOT EXISTS check_ins_user_created_idx
  ON public.check_ins (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS check_ins_user_place_created_idx
  ON public.check_ins (user_id, place_id, created_at DESC);

ALTER TABLE public.check_ins ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON TABLE public.check_ins FROM anon, authenticated;
GRANT ALL ON TABLE public.check_ins TO service_role;
-- Go API uses the Postgres role from DATABASE_URL (typically postgres / bypass RLS).

-- Review likes (toggle helpful vote)
CREATE TABLE IF NOT EXISTS public.review_likes (
  review_id uuid NOT NULL REFERENCES public.reviews (review_id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (review_id, user_id)
);

CREATE INDEX IF NOT EXISTS review_likes_user_idx
  ON public.review_likes (user_id, created_at DESC);

ALTER TABLE public.review_likes ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.review_likes FROM anon, authenticated;
GRANT ALL ON TABLE public.review_likes TO service_role;

-- Place like / unlike (mutually exclusive per user)
CREATE TABLE IF NOT EXISTS public.place_reactions (
  place_id uuid NOT NULL REFERENCES public.places (place_id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  reaction text NOT NULL CHECK (reaction IN ('like', 'unlike')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (place_id, user_id)
);

CREATE INDEX IF NOT EXISTS place_reactions_place_idx
  ON public.place_reactions (place_id, reaction);

CREATE INDEX IF NOT EXISTS place_reactions_user_idx
  ON public.place_reactions (user_id, updated_at DESC);

ALTER TABLE public.place_reactions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.place_reactions FROM anon, authenticated;
GRANT ALL ON TABLE public.place_reactions TO service_role;

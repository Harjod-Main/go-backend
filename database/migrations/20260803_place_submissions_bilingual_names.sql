-- Bilingual names + Google place id for user-submitted parking places.
ALTER TABLE public.place_submissions
  ADD COLUMN IF NOT EXISTS name_th text,
  ADD COLUMN IF NOT EXISTS name_en text,
  ADD COLUMN IF NOT EXISTS google_place_id text;

COMMENT ON COLUMN public.place_submissions.name_th IS 'Thai place name resolved from Google Places or user input';
COMMENT ON COLUMN public.place_submissions.name_en IS 'English place name resolved from Google Places or user input';
COMMENT ON COLUMN public.place_submissions.google_place_id IS 'Google Places place_id when selected via autocomplete';

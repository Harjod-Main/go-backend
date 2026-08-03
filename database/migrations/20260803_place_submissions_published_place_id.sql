-- Link published submissions to catalog places shown on the map.

ALTER TABLE public.place_submissions
  ADD COLUMN IF NOT EXISTS place_id uuid REFERENCES public.places(place_id);

CREATE INDEX IF NOT EXISTS place_submissions_place_id_idx
  ON public.place_submissions (place_id)
  WHERE place_id IS NOT NULL;

COMMENT ON COLUMN public.place_submissions.place_id IS 'Published place row created from this submission';
COMMENT ON COLUMN public.place_submissions.status IS 'pending | approved | rejected';

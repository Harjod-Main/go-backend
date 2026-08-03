-- Structured address fields on place_submissions to mirror public.places
-- while keeping the combined UI `address` column for display / freeform edits.

ALTER TABLE public.place_submissions
  ADD COLUMN IF NOT EXISTS address_th text,
  ADD COLUMN IF NOT EXISTS address_en text,
  ADD COLUMN IF NOT EXISTS subdistrict_th text,
  ADD COLUMN IF NOT EXISTS subdistrict_en text,
  ADD COLUMN IF NOT EXISTS district_th text,
  ADD COLUMN IF NOT EXISTS district_en text,
  ADD COLUMN IF NOT EXISTS province_th text,
  ADD COLUMN IF NOT EXISTS province_en text,
  ADD COLUMN IF NOT EXISTS postal_code text;

COMMENT ON COLUMN public.place_submissions.address IS 'Combined display address from the add-parking UI';
COMMENT ON COLUMN public.place_submissions.address_th IS 'Street-level Thai address (or edited full address)';
COMMENT ON COLUMN public.place_submissions.address_en IS 'Street-level English address (or edited full address)';

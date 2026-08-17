-- Persist EV charger rules captured in create/edit privilege flows.
ALTER TABLE public.ev_charger
  ADD COLUMN IF NOT EXISTS conditions text;

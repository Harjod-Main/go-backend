-- Quota ledger for signed media uploads (Edge Function only; no client access).
CREATE TABLE IF NOT EXISTS public.media_upload_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  object_path text NOT NULL,
  folder text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS media_upload_events_user_created_idx
  ON public.media_upload_events (user_id, created_at DESC);

ALTER TABLE public.media_upload_events ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON TABLE public.media_upload_events FROM anon, authenticated;
GRANT ALL ON TABLE public.media_upload_events TO service_role;

-- Clients must not upload directly; Edge Function issues signed upload URLs after quota checks.
DROP POLICY IF EXISTS "media_public_read" ON storage.objects;
DROP POLICY IF EXISTS "media_authenticated_insert" ON storage.objects;
DROP POLICY IF EXISTS "media_authenticated_update" ON storage.objects;
DROP POLICY IF EXISTS "media_authenticated_delete" ON storage.objects;

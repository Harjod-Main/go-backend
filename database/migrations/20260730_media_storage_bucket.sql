-- Public user media bucket for reviews, reports, submissions, and avatars.
-- Applied remotely via Supabase MCP; kept here for repo history.

INSERT INTO storage.buckets (id, name, public, file_size_limit, allowed_mime_types)
VALUES (
  'media',
  'media',
  true,
  5242880,
  ARRAY['image/jpeg', 'image/png', 'image/webp', 'image/heic', 'image/heif']
)
ON CONFLICT (id) DO UPDATE SET
  public = EXCLUDED.public,
  file_size_limit = EXCLUDED.file_size_limit,
  allowed_mime_types = EXCLUDED.allowed_mime_types;

-- Public object URLs work via bucket.public=true. Do NOT add a broad SELECT
-- policy on storage.objects — that enables listing every file in the bucket.
DROP POLICY IF EXISTS "media_public_read" ON storage.objects;

DROP POLICY IF EXISTS "media_authenticated_insert" ON storage.objects;
CREATE POLICY "media_authenticated_insert"
ON storage.objects
FOR INSERT
TO authenticated
WITH CHECK (
  bucket_id = 'media'
  AND (storage.foldername(name))[1] = (select auth.uid()::text)
);

DROP POLICY IF EXISTS "media_authenticated_update" ON storage.objects;
CREATE POLICY "media_authenticated_update"
ON storage.objects
FOR UPDATE
TO authenticated
USING (
  bucket_id = 'media'
  AND (storage.foldername(name))[1] = (select auth.uid()::text)
)
WITH CHECK (
  bucket_id = 'media'
  AND (storage.foldername(name))[1] = (select auth.uid()::text)
);

DROP POLICY IF EXISTS "media_authenticated_delete" ON storage.objects;
CREATE POLICY "media_authenticated_delete"
ON storage.objects
FOR DELETE
TO authenticated
USING (
  bucket_id = 'media'
  AND (storage.foldername(name))[1] = (select auth.uid()::text)
);

-- Allow authenticated users to delete their own objects so clients can
-- roll back orphan uploads after a partial multi-file failure.
-- Signed uploads remain Edge Function–gated; this only restores DELETE.

DROP POLICY IF EXISTS "media_authenticated_delete" ON storage.objects;
CREATE POLICY "media_authenticated_delete"
ON storage.objects
FOR DELETE
TO authenticated
USING (
  bucket_id = 'media'
  AND (storage.foldername(name))[1] = (select auth.uid()::text)
);

DROP POLICY IF EXISTS "report_media_authenticated_delete" ON storage.objects;
CREATE POLICY "report_media_authenticated_delete"
ON storage.objects
FOR DELETE
TO authenticated
USING (
  bucket_id = 'report-media'
  AND (storage.foldername(name))[1] = (select auth.uid()::text)
);

-- When an object is deleted (including orphan cleanup), stop counting it
-- against upload quota. Keep the audit row but mark it expired.
CREATE OR REPLACE FUNCTION public.mark_media_upload_abandoned()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF OLD.bucket_id IN ('media', 'report-media') THEN
    UPDATE public.media_upload_events
    SET reservation_status = 'expired'
    WHERE object_path = OLD.name
      AND reservation_status IN ('reserved', 'completed');
  END IF;
  RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS media_upload_events_mark_abandoned ON storage.objects;
CREATE TRIGGER media_upload_events_mark_abandoned
  AFTER DELETE ON storage.objects
  FOR EACH ROW
  WHEN (OLD.bucket_id IN ('media', 'report-media'))
  EXECUTE FUNCTION public.mark_media_upload_abandoned();

REVOKE ALL ON FUNCTION public.mark_media_upload_abandoned() FROM PUBLIC;

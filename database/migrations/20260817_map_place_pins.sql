-- Map pin list: covering indexes, a denormalized pin snapshot, and optional
-- viewport reads. parking_area and rate are 1:1 with unique keys, so the
-- snapshot joins them directly instead of DISTINCT ON + full-table aggregates.

CREATE INDEX IF NOT EXISTS place_images_entity_primary_created_idx
  ON public.place_images (entity_type, entity_id, is_primary DESC, created_at);

CREATE INDEX IF NOT EXISTS parking_area_place_id_parking_area_id_idx
  ON public.parking_area (place_id, parking_area_id);

CREATE INDEX IF NOT EXISTS rate_parking_area_id_rate_id_idx
  ON public.rate (parking_area_id, rate_id);

CREATE INDEX IF NOT EXISTS place_submissions_place_approved_created_idx
  ON public.place_submissions (place_id, created_at DESC)
  WHERE status = 'approved';

CREATE MATERIALIZED VIEW IF NOT EXISTS public.map_place_pins AS
SELECT
  pl.place_id,
  pl.name_th,
  pl.name_en,
  pl.place_type,
  pl.latitude,
  pl.longitude,
  pl.geom,
  pl.address_th,
  pl.district_th,
  pl.province_th,
  pl.postal_code,
  COALESCE(img.photo_url, sub.photo_url) AS photo_url,
  rev.avg_rating,
  COALESCE(rev.review_count, 0) AS review_count,
  pa.parking_area_id,
  pa.has_ev_charging,
  pa.has_valet,
  pa.has_cover,
  pa.transit_access,
  pa.transit_access_type,
  pa.total_spaces,
  r.free_minutes,
  COALESCE(tier.min_hourly_rate, tier.min_flat_rate) AS min_hourly_rate
FROM public.places pl
LEFT JOIN public.parking_area pa ON pa.place_id = pl.place_id
LEFT JOIN public.rate r ON r.parking_area_id = pa.parking_area_id
LEFT JOIN LATERAL (
  SELECT
    MIN(rt.price) FILTER (WHERE rt.unit::text = 'hourly')::float8 AS min_hourly_rate,
    MIN(rt.price) FILTER (WHERE rt.unit::text = 'flat')::float8 AS min_flat_rate
  FROM public.rate_tier rt
  WHERE rt.rate_id = r.rate_id
) tier ON true
LEFT JOIN LATERAL (
  SELECT pi.storage_path AS photo_url
  FROM public.place_images pi
  WHERE pi.entity_type = 'place'
    AND pi.entity_id = pl.place_id::text
    AND NULLIF(BTRIM(pi.storage_path), '') IS NOT NULL
  ORDER BY pi.is_primary DESC, pi.created_at
  LIMIT 1
) img ON true
LEFT JOIN LATERAL (
  SELECT ps.photo_urls[1] AS photo_url
  FROM public.place_submissions ps
  WHERE img.photo_url IS NULL
    AND ps.place_id = pl.place_id
    AND ps.status = 'approved'
    AND COALESCE(cardinality(ps.photo_urls), 0) > 0
    AND NULLIF(BTRIM(ps.photo_urls[1]), '') IS NOT NULL
  ORDER BY ps.created_at DESC
  LIMIT 1
) sub ON true
LEFT JOIN (
  SELECT
    rv.place_id,
    ROUND(AVG(rv.rating)::numeric, 1)::float8 AS avg_rating,
    COUNT(*)::int AS review_count
  FROM public.reviews rv
  GROUP BY rv.place_id
) rev ON rev.place_id = pl.place_id
WHERE COALESCE(pl.is_blacklisted, false) = false
WITH NO DATA;

CREATE UNIQUE INDEX IF NOT EXISTS map_place_pins_place_id_uidx
  ON public.map_place_pins (place_id);

CREATE INDEX IF NOT EXISTS map_place_pins_geom_idx
  ON public.map_place_pins USING gist (geom);

COMMENT ON MATERIALIZED VIEW public.map_place_pins IS
  'Map pin snapshot. Hours stay live-joined; refresh via refresh_map_place_pins().';

REFRESH MATERIALIZED VIEW public.map_place_pins;

REVOKE ALL ON public.map_place_pins FROM anon, authenticated;
GRANT SELECT ON public.map_place_pins TO service_role;

CREATE OR REPLACE FUNCTION public.refresh_map_place_pins()
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  REFRESH MATERIALIZED VIEW public.map_place_pins;
END;
$$;

REVOKE ALL ON FUNCTION public.refresh_map_place_pins() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.refresh_map_place_pins() FROM anon;
REVOKE ALL ON FUNCTION public.refresh_map_place_pins() FROM authenticated;
GRANT EXECUTE ON FUNCTION public.refresh_map_place_pins() TO postgres;
GRANT EXECUTE ON FUNCTION public.refresh_map_place_pins() TO service_role;

CREATE EXTENSION IF NOT EXISTS pg_cron WITH SCHEMA pg_catalog;
GRANT USAGE ON SCHEMA cron TO postgres;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM cron.job WHERE jobname = 'refresh-map-place-pins'
  ) THEN
    PERFORM cron.unschedule('refresh-map-place-pins');
  END IF;

  PERFORM cron.schedule(
    'refresh-map-place-pins',
    '* * * * *',
    $cmd$SELECT public.refresh_map_place_pins();$cmd$
  );
EXCEPTION
  WHEN OTHERS THEN
    RAISE NOTICE 'pg_cron schedule skipped: %', SQLERRM;
END;
$$;

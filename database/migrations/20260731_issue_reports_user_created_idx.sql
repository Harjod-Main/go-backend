-- Speed up GET /api/v1/me/reports: filter by user_id, order by created_at DESC.
CREATE INDEX IF NOT EXISTS idx_issue_reports_user_created
ON public.issue_reports (user_id, created_at DESC);

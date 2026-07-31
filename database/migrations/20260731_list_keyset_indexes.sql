-- Strengthen list indexes for keyset pagination on (created_at, id).
CREATE INDEX IF NOT EXISTS idx_reviews_place_created_id
  ON public.reviews (place_id, created_at DESC, review_id DESC);

CREATE INDEX IF NOT EXISTS check_ins_user_created_id_idx
  ON public.check_ins (user_id, created_at DESC, check_in_id DESC);

CREATE INDEX IF NOT EXISTS idx_issue_reports_user_created_id
  ON public.issue_reports (user_id, created_at DESC, report_id DESC);

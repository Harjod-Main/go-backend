-- Leaderboard / credit points ordering.
CREATE INDEX IF NOT EXISTS idx_profiles_credit_points
  ON public.profiles (credit_points DESC, updated_at ASC, user_id ASC);

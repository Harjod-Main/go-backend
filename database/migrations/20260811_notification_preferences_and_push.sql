-- Notifications MVP: user preferences + web push subscriptions + iOS (Expo) push tokens.

CREATE TABLE IF NOT EXISTS public.notification_preferences (
  user_id uuid PRIMARY KEY REFERENCES auth.users (id) ON DELETE CASCADE,
  notifications_enabled boolean NOT NULL DEFAULT false,
  in_app_alerts_enabled boolean NOT NULL DEFAULT true,
  in_app_sounds_enabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE public.notification_preferences ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.notification_preferences FROM anon, authenticated;
GRANT ALL ON TABLE public.notification_preferences TO service_role;

CREATE POLICY notification_preferences_select_own
  ON public.notification_preferences
  FOR SELECT
  USING (user_id = auth.uid());

CREATE POLICY notification_preferences_insert_own
  ON public.notification_preferences
  FOR INSERT
  WITH CHECK (user_id = auth.uid());

CREATE POLICY notification_preferences_update_own
  ON public.notification_preferences
  FOR UPDATE
  USING (user_id = auth.uid())
  WITH CHECK (user_id = auth.uid());


CREATE TABLE IF NOT EXISTS public.web_push_subscriptions (
  subscription_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  endpoint text NOT NULL,
  keys jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS web_push_subscriptions_user_endpoint_uniq
  ON public.web_push_subscriptions (user_id, endpoint);

ALTER TABLE public.web_push_subscriptions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.web_push_subscriptions FROM anon, authenticated;
GRANT ALL ON TABLE public.web_push_subscriptions TO service_role;

CREATE POLICY web_push_subscriptions_select_own
  ON public.web_push_subscriptions
  FOR SELECT
  USING (user_id = auth.uid());

CREATE POLICY web_push_subscriptions_insert_own
  ON public.web_push_subscriptions
  FOR INSERT
  WITH CHECK (user_id = auth.uid());

CREATE POLICY web_push_subscriptions_update_own
  ON public.web_push_subscriptions
  FOR UPDATE
  USING (user_id = auth.uid())
  WITH CHECK (user_id = auth.uid());

CREATE POLICY web_push_subscriptions_delete_own
  ON public.web_push_subscriptions
  FOR DELETE
  USING (user_id = auth.uid());


CREATE TABLE IF NOT EXISTS public.ios_push_tokens (
  token_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
  token text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ios_push_tokens_user_token_uniq
  ON public.ios_push_tokens (user_id, token);

ALTER TABLE public.ios_push_tokens ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE public.ios_push_tokens FROM anon, authenticated;
GRANT ALL ON TABLE public.ios_push_tokens TO service_role;

CREATE POLICY ios_push_tokens_select_own
  ON public.ios_push_tokens
  FOR SELECT
  USING (user_id = auth.uid());

CREATE POLICY ios_push_tokens_insert_own
  ON public.ios_push_tokens
  FOR INSERT
  WITH CHECK (user_id = auth.uid());

CREATE POLICY ios_push_tokens_update_own
  ON public.ios_push_tokens
  FOR UPDATE
  USING (user_id = auth.uid())
  WITH CHECK (user_id = auth.uid());

CREATE POLICY ios_push_tokens_delete_own
  ON public.ios_push_tokens
  FOR DELETE
  USING (user_id = auth.uid());


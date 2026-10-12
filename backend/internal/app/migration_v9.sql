ALTER TABLE users ADD COLUMN email text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN phone text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN is_developer boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN mfa_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN deleted_at timestamptz;
ALTER TABLE users ADD CONSTRAINT developer_requires_mfa CHECK (NOT is_developer OR (role='Super Admin' AND mfa_enabled AND email<>'' AND deleted_at IS NULL));
CREATE UNIQUE INDEX one_developer_owner ON users (is_developer) WHERE is_developer;

CREATE TABLE system_issues (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 fingerprint text NOT NULL UNIQUE,
 category text NOT NULL,
 severity text NOT NULL CHECK(severity IN ('Info','Warning','Error')),
 title text NOT NULL,
 route text NOT NULL DEFAULT '',
 http_status integer NOT NULL DEFAULT 0,
 user_id bigint REFERENCES users(id),
 request_id text NOT NULL DEFAULT '',
 occurrences integer NOT NULL DEFAULT 1,
 first_seen timestamptz NOT NULL DEFAULT now(),
 last_seen timestamptz NOT NULL DEFAULT now(),
 status text NOT NULL DEFAULT 'Open' CHECK(status IN ('Open','Investigating','Resolved')),
 resolution text NOT NULL DEFAULT '',
 version integer NOT NULL DEFAULT 1
);
CREATE INDEX issue_recent ON system_issues(last_seen DESC);
ALTER TABLE system_issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE system_issues FORCE ROW LEVEL SECURITY;
CREATE POLICY issue_read ON system_issues FOR SELECT USING(erp_identity()->>'developer'='true' OR erp_service()='diagnostics');
CREATE POLICY issue_insert ON system_issues FOR INSERT WITH CHECK(erp_service()='diagnostics');
CREATE POLICY issue_update ON system_issues FOR UPDATE USING(erp_identity()->>'developer'='true' OR erp_service()='diagnostics');

CREATE TABLE notification_settings (
 id integer PRIMARY KEY CHECK(id=1),
 recipients text[] NOT NULL DEFAULT '{}',
 version integer NOT NULL DEFAULT 1
);
INSERT INTO notification_settings(id) VALUES(1);
CREATE TABLE company_notification_settings (
 company_id bigint PRIMARY KEY REFERENCES company_setups(id),
 recipients text[] NOT NULL,
 version integer NOT NULL DEFAULT 1
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['notification_settings','company_notification_settings'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY notification_read ON %I FOR SELECT USING (erp_identity()->>''developer''=''true'' OR erp_service()=''reminders'')',t);
  EXECUTE format('CREATE POLICY notification_write ON %I FOR ALL USING (erp_identity()->>''developer''=''true'') WITH CHECK (erp_identity()->>''developer''=''true'')',t);
 END LOOP;
END $$;
GRANT SELECT,INSERT,UPDATE ON system_issues,notification_settings,company_notification_settings TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE system_issues_id_seq TO euroerp_runtime;
INSERT INTO schema_versions(version) VALUES(9);

CREATE TABLE whatsapp_settings (
 id integer PRIMARY KEY CHECK(id=1), enabled boolean NOT NULL DEFAULT false,
 reminder_hour integer NOT NULL DEFAULT 9 CHECK(reminder_hour BETWEEN 0 AND 23),
 daily_cap integer NOT NULL DEFAULT 1000 CHECK(daily_cap BETWEEN 1 AND 10000),
 version integer NOT NULL DEFAULT 1
);
INSERT INTO whatsapp_settings(id) VALUES(1);
CREATE TABLE whatsapp_recipients (
 user_id bigint PRIMARY KEY REFERENCES users, enabled boolean NOT NULL DEFAULT false,
 consent_note text NOT NULL DEFAULT '', consent_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE whatsapp_templates (
 kind text PRIMARY KEY CHECK(kind IN ('assignment','update','daily','renewal','test')),
 name text NOT NULL UNIQUE, language text NOT NULL DEFAULT 'en', body text NOT NULL,
 status text NOT NULL DEFAULT 'DRAFT', meta_id text NOT NULL DEFAULT '',
 version integer NOT NULL DEFAULT 1, synced_at timestamptz
);
INSERT INTO whatsapp_templates(kind,name,body) VALUES
 ('assignment','euro_work_assignment','Hello {{1}}, Euro Suits has assigned work to your dashboard: {{2}}. Details: {{3}}. View your work: {{4}}. This is an automated notification from Euro Suits ERP.'),
 ('update','euro_work_update','Hello {{1}}, your Euro Suits work has an update: {{2}}. Current status: {{3}}. View details: {{4}}. This is an automated notification from Euro Suits ERP.'),
 ('daily','euro_daily_pending','Hello {{1}}, this is your Euro Suits daily work summary: {{2}}. Pending details: {{3}}. Review your dashboard: {{4}}. This is an automated notification from Euro Suits ERP.'),
 ('renewal','euro_document_reminder','Hello {{1}}, a Euro Suits document assigned to you needs attention: {{2}}. Renewal details: {{3}}. Review: {{4}}. This is an automated notification from Euro Suits ERP.'),
 ('test','euro_connection_test','Hello {{1}}, this is a requested Euro Suits ERP connection test: {{2}}. Test details: {{3}}. Open the ERP: {{4}}. This is an automated notification from Euro Suits ERP.');
CREATE TABLE whatsapp_observations (
 user_id bigint NOT NULL REFERENCES users, task_key text NOT NULL, fingerprint text NOT NULL,
 generation integer NOT NULL DEFAULT 1, active boolean NOT NULL DEFAULT true,
 PRIMARY KEY(user_id,task_key)
);
CREATE TABLE whatsapp_budget(day date PRIMARY KEY, attempts integer NOT NULL DEFAULT 0);
CREATE TABLE whatsapp_messages (
 id bigserial PRIMARY KEY, event_key text NOT NULL UNIQUE,
 user_id bigint NOT NULL REFERENCES users, phone text NOT NULL, kind text NOT NULL REFERENCES whatsapp_templates(kind),
 parameters jsonb NOT NULL, status text NOT NULL DEFAULT 'Queued',
 meta_id text UNIQUE, error text NOT NULL DEFAULT '', attempts integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 next_attempt timestamptz NOT NULL DEFAULT now(), sent_at timestamptz,
 CHECK(status IN ('Queued','Sending','Accepted','Sent','Delivered','Read','Failed','Unknown','Cancelled'))
);
CREATE INDEX whatsapp_queue ON whatsapp_messages(next_attempt) WHERE status='Queued';
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['whatsapp_settings','whatsapp_recipients','whatsapp_templates','whatsapp_observations','whatsapp_messages','whatsapp_budget'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY owner_notifications ON %I USING(erp_identity()->>''developer''=''true'' OR erp_service()=''reminders'' OR erp_service()=''whatsapp'')',t);
 END LOOP;
END $$;
GRANT SELECT,INSERT,UPDATE,DELETE ON whatsapp_settings,whatsapp_recipients,whatsapp_templates,whatsapp_observations,whatsapp_messages,whatsapp_budget TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE whatsapp_messages_id_seq TO euroerp_runtime;
INSERT INTO schema_versions(version) VALUES(14);

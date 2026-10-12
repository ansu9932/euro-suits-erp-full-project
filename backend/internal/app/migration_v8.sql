-- The API login inherits this non-owner role; the migration login is never
-- supplied to the running API container.
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='euroerp_runtime') THEN
  CREATE ROLE euroerp_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
 END IF;
END $$;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO euroerp_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO euroerp_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO euroerp_runtime;
REVOKE INSERT,UPDATE,DELETE ON schema_versions FROM euroerp_runtime;
REVOKE UPDATE,DELETE ON audit,company_setup_history,setup_access_history,access_history FROM euroerp_runtime;

CREATE FUNCTION erp_identity() RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT COALESCE(NULLIF(current_setting('erp.identity',true),''),'{}')::jsonb
$$;
CREATE FUNCTION erp_service() RETURNS text LANGUAGE sql STABLE AS $$
 SELECT COALESCE(current_setting('erp.service',true),'')
$$;
CREATE FUNCTION erp_company_allowed(cid bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT erp_identity()->>'role'='Super Admin' OR COALESCE(erp_identity()->'companies','[]'::jsonb) @> to_jsonb(cid)
$$;
CREATE FUNCTION erp_order_allowed(cid bigint,tid bigint,lid bigint,oid bigint,unit text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT erp_service()='reminders' OR erp_identity()->>'role'='Super Admin' OR (
  erp_identity()->>'view_orders'='true' AND erp_company_allowed(cid) AND
  (erp_identity()->>'scope'='company' OR
   (erp_identity()->>'scope'='team' AND erp_identity()->'teams' @> to_jsonb(tid)) OR
   (erp_identity()->>'scope'='line' AND erp_identity()->'lines' @> to_jsonb(lid)) OR
   erp_identity()->'orders' @> to_jsonb(oid)) AND
  (erp_identity()->>'role'<>'Finishing' OR erp_identity()->'units' @> to_jsonb(unit)))
$$;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;
CREATE POLICY order_scope ON orders USING (erp_order_allowed(company_id,team_id,line_id,id,finishing_unit));
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['milestones','work_items','entries','quality','materials','documents','reminders'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY order_scope ON %I USING (EXISTS(SELECT 1 FROM orders WHERE id=order_id))',t);
 END LOOP;
 FOREACH t IN ARRAY ARRAY['teams','buyers','lines','suppliers','inventory'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY company_scope ON %I USING (erp_company_allowed(company_id))',t);
 END LOOP;
END $$;
ALTER TABLE purchases ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchases FORCE ROW LEVEL SECURITY;
CREATE POLICY purchase_scope ON purchases USING (erp_company_allowed(company_id) AND (erp_identity()->>'role'='Super Admin' OR erp_identity()->>'scope'='company' OR EXISTS(SELECT 1 FROM orders WHERE id=order_id)));
ALTER TABLE movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE movements FORCE ROW LEVEL SECURITY;
CREATE POLICY movement_scope ON movements USING (EXISTS(SELECT 1 FROM inventory WHERE id=item_id) AND (order_id IS NULL OR EXISTS(SELECT 1 FROM orders WHERE id=order_id)));
ALTER TABLE company_setups ENABLE ROW LEVEL SECURITY;
ALTER TABLE company_setups FORCE ROW LEVEL SECURITY;
CREATE POLICY company_setup_scope ON company_setups USING (
 erp_service() IN ('identity','reminders') OR erp_identity()->>'role'='Super Admin' OR
 (erp_identity()->>'view_setup'='true' AND (erp_identity()->>'role'='MD' OR assigned_to::text=erp_identity()->>'id'))
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['company_setup_files','company_setup_history','company_reminders'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY setup_scope ON %I USING (EXISTS(SELECT 1 FROM company_setups WHERE id=company_id))',t);
 END LOOP;
END $$;
ALTER TABLE documents ADD COLUMN encryption_version integer NOT NULL DEFAULT 0 CHECK(encryption_version IN (0,1));
INSERT INTO schema_versions(version) VALUES(8);

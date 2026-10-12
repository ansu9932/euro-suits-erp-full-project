SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

-- Existing rows retain all values and enter the original Export register.
ALTER TABLE export_invoices ADD COLUMN record_kind text NOT NULL DEFAULT 'export';
ALTER TABLE export_invoices ADD CONSTRAINT export_record_kind_check CHECK(record_kind IN ('export','jv'));
ALTER TABLE export_invoices ADD CONSTRAINT export_jv_shape_check CHECK(record_kind<>'jv' OR (
 company_id IS NULL AND party_type='manual' AND party_id IS NULL AND buyer_id IS NULL AND party_setup_id IS NULL AND order_id IS NULL
 AND receipts='[]'::jsonb AND ebrcs='[]'::jsonb AND finance_check='{}'::jsonb AND audit_check='{}'::jsonb
 AND (profile-ARRAY['export_date','shipping_mode','jv_by_date','scrip_tally_reference','statuses'])='{}'::jsonb
 AND COALESCE(profile->'statuses','{}'::jsonb)='{}'::jsonb
));
CREATE FUNCTION protect_export_record_kind() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
BEGIN
 IF NEW.record_kind IS DISTINCT FROM OLD.record_kind THEN
  RAISE EXCEPTION 'Export and JV classification cannot change after creation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_record_kind_immutable BEFORE UPDATE ON export_invoices FOR EACH ROW EXECUTE FUNCTION protect_export_record_kind();
CREATE POLICY export_discount_record_kind ON export_discounts AS RESTRICTIVE USING(
 EXISTS(SELECT 1 FROM export_invoices d WHERE d.id=invoice_id AND d.record_kind='export')
);
-- Keep supporting JV documents distinct from export document categories. Lock
-- the parent so this boundary shares the application's serialization point.
CREATE FUNCTION guard_export_file_kind() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE parent_kind text;
BEGIN
 SELECT record_kind INTO parent_kind FROM export_invoices WHERE id=NEW.invoice_id FOR UPDATE;
 IF parent_kind IS NULL OR (parent_kind='jv') IS DISTINCT FROM (NEW.kind='jv_supporting') THEN
  RAISE EXCEPTION 'Document category does not match the Export or JV register' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_file_kind_guard BEFORE INSERT OR UPDATE OF invoice_id,kind ON export_files FOR EACH ROW EXECUTE FUNCTION guard_export_file_kind();
CREATE TABLE export_bank_emails (
 id bigserial PRIMARY KEY, invoice_id bigint NOT NULL REFERENCES export_invoices,
 invoice_version integer NOT NULL CHECK(invoice_version>0), created_by bigint NOT NULL REFERENCES users,
 recipients jsonb NOT NULL CHECK(jsonb_typeof(recipients)='array' AND jsonb_array_length(recipients) BETWEEN 1 AND 5),
 subject text NOT NULL CHECK(length(subject) BETWEEN 1 AND 160),
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 10000),
 files jsonb NOT NULL CHECK(jsonb_typeof(files)='array' AND jsonb_array_length(files) BETWEEN 1 AND 12),
 request_hash text NOT NULL UNIQUE CHECK(length(request_hash)=64),
 status text NOT NULL DEFAULT 'Prepared' CHECK(status IN ('Prepared','Sending','Accepted','Uncertain','Failed')),
 created_at timestamptz NOT NULL DEFAULT now(), sent_at timestamptz, error_message text NOT NULL DEFAULT ''
);
CREATE INDEX export_bank_emails_parent ON export_bank_emails(invoice_id,id);
ALTER TABLE export_bank_emails ENABLE ROW LEVEL SECURITY;
ALTER TABLE export_bank_emails FORCE ROW LEVEL SECURITY;
CREATE POLICY export_bank_email_read ON export_bank_emails FOR SELECT USING(
 erp_identity()->>'exports'='true' AND erp_identity()->>'exports_edit'='true' AND erp_identity()->>'exports_download'='true'
 AND EXISTS(SELECT 1 FROM export_invoices d WHERE d.id=invoice_id AND d.record_kind='export')
);
CREATE POLICY export_bank_email_create ON export_bank_emails FOR INSERT WITH CHECK(
 erp_identity()->>'exports_edit'='true' AND erp_identity()->>'exports_download'='true'
 AND created_by=(erp_identity()->>'id')::bigint AND status='Prepared'
 AND EXISTS(SELECT 1 FROM export_invoices d WHERE d.id=invoice_id AND d.record_kind='export' AND d.version=invoice_version)
);
CREATE POLICY export_bank_email_update ON export_bank_emails FOR UPDATE USING(
 erp_identity()->>'exports_edit'='true' AND erp_identity()->>'exports_download'='true'
 AND created_by=(erp_identity()->>'id')::bigint
 AND EXISTS(SELECT 1 FROM export_invoices d WHERE d.id=invoice_id AND d.record_kind='export')
);
CREATE FUNCTION protect_export_bank_email() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Bank email history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['status','sent_at','error_message']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['status','sent_at','error_message']) THEN
  RAISE EXCEPTION 'Reviewed bank email content cannot change' USING ERRCODE='23514';
 END IF;
 IF NEW.status IS DISTINCT FROM OLD.status AND NOT (
  (OLD.status IN ('Prepared','Failed') AND NEW.status='Sending') OR
  (OLD.status='Sending' AND NEW.status IN ('Accepted','Uncertain','Failed'))
 ) THEN RAISE EXCEPTION 'Invalid bank email status transition' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_bank_email_immutable BEFORE UPDATE OR DELETE ON export_bank_emails FOR EACH ROW EXECUTE FUNCTION protect_export_bank_email();
GRANT SELECT,INSERT,UPDATE ON export_bank_emails TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE export_bank_emails_id_seq TO euroerp_runtime;
INSERT INTO schema_versions(version) VALUES(25);

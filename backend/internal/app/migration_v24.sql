SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

-- A removal is an append-only event. Original metadata and encrypted bytes stay
-- intact for historical evidence; no existing export_files rows are rewritten.
CREATE TABLE export_file_removals (
 file_id bigint PRIMARY KEY REFERENCES export_files,
 removed_by bigint NOT NULL REFERENCES users,
 removed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER export_file_removals_immutable BEFORE UPDATE OR DELETE ON export_file_removals FOR EACH ROW EXECUTE FUNCTION protect_audit();

-- The HTTP mutation already locks the invoice. Take the same parent lock here
-- so a direct runtime insert cannot race a submission or review transition.
CREATE FUNCTION guard_export_file_removal() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE parent_status text;
BEGIN
 SELECT d.workflow_status INTO parent_status
 FROM export_invoices d JOIN export_files f ON f.invoice_id=d.id
 WHERE f.id=NEW.file_id FOR UPDATE OF d;
 IF parent_status IS NULL OR parent_status NOT IN ('Draft','Changes requested') THEN
  RAISE EXCEPTION 'Only editable invoice documents can be removed' USING ERRCODE='23514';
 END IF;
 NEW.removed_at=now();
 RETURN NEW;
END $$;
CREATE TRIGGER export_file_removal_guard BEFORE INSERT ON export_file_removals FOR EACH ROW EXECUTE FUNCTION guard_export_file_removal();
ALTER TABLE export_file_removals ENABLE ROW LEVEL SECURITY;
ALTER TABLE export_file_removals FORCE ROW LEVEL SECURITY;
CREATE POLICY export_file_removal_read ON export_file_removals FOR SELECT USING(
 EXISTS(SELECT 1 FROM export_files f WHERE f.id=file_id)
);
CREATE POLICY export_file_removal_add ON export_file_removals FOR INSERT WITH CHECK(
 erp_identity()->>'exports'='true' AND erp_identity()->>'exports_files'='true'
 AND removed_by=(erp_identity()->>'id')::bigint
 AND EXISTS(SELECT 1 FROM export_files f JOIN export_invoices d ON d.id=f.invoice_id
  WHERE f.id=file_id AND d.workflow_status IN ('Draft','Changes requested'))
);
GRANT SELECT,INSERT ON export_file_removals TO euroerp_runtime;
INSERT INTO schema_versions(version) VALUES(24);

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

-- Existing documents remain shared. No invoice values, encrypted bytes or
-- receipt arrays are rewritten or associated by guesswork.
ALTER TABLE export_files ADD COLUMN receipt_key uuid;
ALTER TABLE export_files ADD COLUMN field_key text;
ALTER TABLE export_files ADD CONSTRAINT export_file_context_pair CHECK (
 (receipt_key IS NULL AND field_key IS NULL) OR
 (receipt_key IS NOT NULL AND field_key IS NOT NULL AND (
  (field_key IN ('inward_remittance','disposal_instruction') AND kind='payment_details') OR
  (field_key='credit_advice' AND kind='bank_advice')
 ))
);
CREATE INDEX export_files_receipt_context ON export_files(invoice_id,receipt_key,field_key) WHERE receipt_key IS NOT NULL;

CREATE FUNCTION export_receipt_keys_valid(receipts jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE SET search_path=public,pg_temp AS $$
DECLARE item jsonb; receipt text; seen text[]:=ARRAY[]::text[];
BEGIN
 IF jsonb_typeof(receipts)<>'array' THEN RETURN false; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(receipts) LOOP
  IF item ? 'receipt_key' THEN
   receipt:=item->>'receipt_key';
   IF jsonb_typeof(item->'receipt_key') IS DISTINCT FROM 'string' OR receipt IS NULL
    OR receipt !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    OR receipt='00000000-0000-0000-0000-000000000000' OR receipt=ANY(seen) THEN RETURN false; END IF;
   seen:=array_append(seen,receipt);
  END IF;
 END LOOP;
 RETURN true;
END $$;
ALTER TABLE export_invoices ADD CONSTRAINT export_receipt_keys_check CHECK(export_receipt_keys_valid(receipts));

CREATE FUNCTION guard_export_file_context() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE parent export_invoices%ROWTYPE; reconciliation text:=NULLIF(current_setting('erp.export_reconciliation',true),'');
BEGIN
 IF reconciliation IS NOT NULL THEN
  SELECT * INTO parent FROM export_invoices WHERE id=NEW.invoice_id FOR UPDATE;
  IF reconciliation IS DISTINCT FROM NEW.invoice_id::text OR NEW.kind<>'ebrc'
   OR NEW.receipt_key IS NOT NULL OR NEW.field_key IS NOT NULL
   OR parent.record_kind IS DISTINCT FROM 'export'
   OR erp_identity()->>'exports_edit' IS DISTINCT FROM 'true'
   OR COALESCE(erp_identity()->>'role','') NOT IN ('Finance','Finance Head','Super Admin')
   OR erp_identity()->>'exports_files' IS DISTINCT FROM 'true' THEN
   RAISE EXCEPTION 'Reconciliation accepts only authorized eBRC documents for its invoice' USING ERRCODE='23514';
  END IF;
 END IF;
 IF TG_OP='UPDATE' AND (NEW.invoice_id,NEW.receipt_key,NEW.field_key) IS DISTINCT FROM (OLD.invoice_id,OLD.receipt_key,OLD.field_key) THEN
  RAISE EXCEPTION 'Document receipt context cannot be reassigned' USING ERRCODE='23514';
 END IF;
 IF NEW.receipt_key IS NOT NULL THEN
  SELECT * INTO parent FROM export_invoices WHERE id=NEW.invoice_id FOR UPDATE;
  IF parent.record_kind IS DISTINCT FROM 'export' OR NOT EXISTS(
   SELECT 1 FROM jsonb_array_elements(parent.receipts) r WHERE r->>'receipt_key'=NEW.receipt_key::text
  ) THEN RAISE EXCEPTION 'Save a matching export receipt before attaching its document' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_file_context_guard BEFORE INSERT OR UPDATE ON export_files FOR EACH ROW EXECUTE FUNCTION guard_export_file_context();

CREATE FUNCTION protect_export_receipt_documents() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM export_files f WHERE f.invoice_id=NEW.id AND f.receipt_key IS NOT NULL
  AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)
  AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.receipts) r WHERE r->>'receipt_key'=f.receipt_key::text)) THEN
  RAISE EXCEPTION 'Remove attached documents before removing or replacing their receipt' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_receipt_documents_guard BEFORE UPDATE OF receipts ON export_invoices FOR EACH ROW EXECUTE FUNCTION protect_export_receipt_documents();

ALTER TABLE export_invoices ADD COLUMN documentation_cleared jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE export_invoices ADD CONSTRAINT export_documentation_clearance_shape CHECK (
 jsonb_typeof(documentation_cleared)='object' AND
 (record_kind='export' OR documentation_cleared='{}'::jsonb)
);
CREATE FUNCTION guard_export_documentation_clearance() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
BEGIN
 IF TG_OP='UPDATE' AND (NEW.content_revision IS DISTINCT FROM OLD.content_revision
  OR NEW.ebrcs IS DISTINCT FROM OLD.ebrcs
  OR (OLD.workflow_status='Approved' AND NEW.workflow_status='Draft')) THEN
  NEW.documentation_cleared:='{}'::jsonb;
 END IF;
 IF NEW.documentation_cleared<>'{}'::jsonb THEN
  IF TG_OP='INSERT' OR NEW.documentation_cleared IS DISTINCT FROM OLD.documentation_cleared THEN
   IF erp_identity()->>'exports_edit' IS DISTINCT FROM 'true'
    OR NEW.documentation_cleared->>'by' IS DISTINCT FROM erp_identity()->>'id' THEN
    RAISE EXCEPTION 'Only an authorized export editor can clear documentation' USING ERRCODE='23514';
   END IF;
  END IF;
  IF NEW.record_kind<>'export'
   OR (NEW.documentation_cleared-ARRAY['by','at','ebrc_number','file_id','content_revision'])<>'{}'::jsonb
   OR NOT(NEW.documentation_cleared ?& ARRAY['by','at','ebrc_number','file_id','content_revision'])
   OR jsonb_typeof(NEW.documentation_cleared->'by') IS DISTINCT FROM 'number'
   OR jsonb_typeof(NEW.documentation_cleared->'at') IS DISTINCT FROM 'string'
   OR jsonb_typeof(NEW.documentation_cleared->'ebrc_number') IS DISTINCT FROM 'string'
   OR jsonb_typeof(NEW.documentation_cleared->'file_id') IS DISTINCT FROM 'number'
   OR jsonb_typeof(NEW.documentation_cleared->'content_revision') IS DISTINCT FROM 'number'
   OR NEW.documentation_cleared->>'content_revision' IS DISTINCT FROM NEW.content_revision::text
   OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.ebrcs) e WHERE e->>'ebrc_number'=NEW.documentation_cleared->>'ebrc_number' AND length(trim(e->>'ebrc_number'))>0)
   OR NOT EXISTS(SELECT 1 FROM export_files f WHERE f.id::text=NEW.documentation_cleared->>'file_id' AND f.invoice_id=NEW.id AND f.kind='ebrc'
    AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)) THEN
   RAISE EXCEPTION 'Documentation clearance requires the current saved eBRC number and active document' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_documentation_clearance_guard BEFORE INSERT OR UPDATE ON export_invoices FOR EACH ROW EXECUTE FUNCTION guard_export_documentation_clearance();

CREATE FUNCTION invalidate_export_removed_clearance() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
BEGIN
 UPDATE export_invoices SET documentation_cleared='{}'::jsonb
 WHERE id=(SELECT invoice_id FROM export_files WHERE id=NEW.file_id)
 AND documentation_cleared->>'file_id'=NEW.file_id::text;
 RETURN NEW;
END $$;
CREATE TRIGGER export_removed_clearance_guard AFTER INSERT ON export_file_removals FOR EACH ROW EXECUTE FUNCTION invalidate_export_removed_clearance();


-- The dedicated reconciliation transaction is bound to one parent. Only eBRC
-- numbers and clearance bookkeeping may change; invoice approval and money
-- remain byte-for-byte unchanged even if a future handler mixes fields.
CREATE FUNCTION guard_export_reconciliation_write() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE reconciliation text:=NULLIF(current_setting('erp.export_reconciliation',true),'');
 old_count integer; new_count integer; changed integer:=0; i integer; item jsonb; number text;
BEGIN
 IF reconciliation IS NULL THEN RETURN NEW; END IF;
 IF reconciliation IS DISTINCT FROM NEW.id::text OR NEW.record_kind<>'export'
  OR erp_identity()->>'exports_edit' IS DISTINCT FROM 'true'
   OR COALESCE(erp_identity()->>'role','') NOT IN ('Finance','Finance Head','Super Admin')
  OR (to_jsonb(NEW)-ARRAY['ebrcs','documentation_cleared','version','updated_at']) IS DISTINCT FROM
     (to_jsonb(OLD)-ARRAY['ebrcs','documentation_cleared','version','updated_at']) THEN
  RAISE EXCEPTION 'Reconciliation cannot change invoice content, approval or financial values' USING ERRCODE='23514';
 END IF;
 IF NEW.version<>OLD.version+1 AND NOT(NEW.version=OLD.version
  AND NEW.documentation_cleared='{}'::jsonb
  AND (to_jsonb(NEW)-'documentation_cleared')=(to_jsonb(OLD)-'documentation_cleared')) THEN
  RAISE EXCEPTION 'Reconciliation requires the current invoice version' USING ERRCODE='23514';
 END IF;
 IF NEW.ebrcs IS NOT DISTINCT FROM OLD.ebrcs THEN RETURN NEW; END IF;
 IF jsonb_typeof(NEW.ebrcs) IS DISTINCT FROM 'array' THEN
  RAISE EXCEPTION 'Reconciliation requires saved eBRC entries' USING ERRCODE='23514';
 END IF;
 old_count:=jsonb_array_length(OLD.ebrcs); new_count:=jsonb_array_length(NEW.ebrcs);
 IF new_count<old_count OR new_count>old_count+1 OR new_count>100 THEN
  RAISE EXCEPTION 'Reconciliation may add or correct one eBRC number' USING ERRCODE='23514';
 END IF;
 IF new_count>0 THEN
  FOR i IN 0..new_count-1 LOOP
   item:=NEW.ebrcs->i;
   IF i<old_count AND (item-'ebrc_number') IS DISTINCT FROM ((OLD.ebrcs->i)-'ebrc_number') THEN
    RAISE EXCEPTION 'Other saved eBRC fields must remain unchanged' USING ERRCODE='23514';
   END IF;
   IF i>=old_count AND ((item-ARRAY['ebrc_number','statuses'])<>'{}'::jsonb OR COALESCE(item->'statuses','{}'::jsonb)<>'{}'::jsonb) THEN
    RAISE EXCEPTION 'Only a new eBRC number may be added in reconciliation' USING ERRCODE='23514';
   END IF;
   IF i>=old_count OR item IS DISTINCT FROM OLD.ebrcs->i THEN
    changed:=changed+1; number:=item->>'ebrc_number';
    IF jsonb_typeof(item->'ebrc_number') IS DISTINCT FROM 'string' OR length(btrim(number)) NOT BETWEEN 1 AND 160 OR number ~ E'[\r\n\t]' THEN
     RAISE EXCEPTION 'Enter a valid eBRC number' USING ERRCODE='23514';
    END IF;
   END IF;
  END LOOP;
 END IF;
 IF changed<>1 OR EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.ebrcs) e
  WHERE length(btrim(e->>'ebrc_number'))>0 GROUP BY upper(regexp_replace(btrim(e->>'ebrc_number'),'\s+',' ','g')) HAVING count(*)>1) THEN
  RAISE EXCEPTION 'Reconciliation may add or correct one unique eBRC number' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER export_reconciliation_write_guard BEFORE UPDATE ON export_invoices FOR EACH ROW EXECUTE FUNCTION guard_export_reconciliation_write();

CREATE OR REPLACE FUNCTION guard_export_file_removal() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE parent_id bigint; parent_status text; parent_kind text; file_kind text;
 reconciliation text:=NULLIF(current_setting('erp.export_reconciliation',true),''); permitted_evidence boolean;
BEGIN
 SELECT d.id,d.workflow_status,d.record_kind,f.kind INTO parent_id,parent_status,parent_kind,file_kind
 FROM export_invoices d JOIN export_files f ON f.invoice_id=d.id
 WHERE f.id=NEW.file_id FOR UPDATE OF d;
 permitted_evidence:=reconciliation=parent_id::text AND parent_kind='export' AND file_kind='ebrc'
  AND erp_identity()->>'exports_edit'='true' AND erp_identity()->>'exports_files'='true'
  AND erp_identity()->>'role' IN ('Finance','Finance Head','Super Admin');
 IF (reconciliation IS NOT NULL AND permitted_evidence IS DISTINCT FROM true)
  OR parent_status IS NULL
  OR (parent_status NOT IN ('Draft','Changes requested') AND permitted_evidence IS DISTINCT FROM true) THEN
  RAISE EXCEPTION 'Only editable documents or authorized reconciliation eBRC evidence can be removed' USING ERRCODE='23514';
 END IF;
 NEW.removed_at=now();
 RETURN NEW;
END $$;
DROP POLICY export_file_removal_add ON export_file_removals;
CREATE POLICY export_file_removal_add ON export_file_removals FOR INSERT WITH CHECK(
 erp_identity()->>'exports'='true' AND erp_identity()->>'exports_files'='true'
 AND removed_by=(erp_identity()->>'id')::bigint
 AND EXISTS(SELECT 1 FROM export_files f JOIN export_invoices d ON d.id=f.invoice_id
  WHERE f.id=file_id AND (d.workflow_status IN ('Draft','Changes requested') OR (
   current_setting('erp.export_reconciliation',true)=d.id::text AND d.record_kind='export' AND f.kind='ebrc' AND erp_identity()->>'exports_edit'='true'
  )))
);
INSERT INTO schema_versions(version) VALUES(26);

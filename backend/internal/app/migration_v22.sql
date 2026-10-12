SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

ALTER TABLE export_invoices ALTER COLUMN company_id DROP NOT NULL;
ALTER TABLE export_invoices ALTER COLUMN party_id DROP NOT NULL;
ALTER TABLE export_invoices DROP CONSTRAINT export_invoices_party_type_check;
ALTER TABLE export_invoices DROP CONSTRAINT export_invoices_check;
ALTER TABLE export_invoices ADD CONSTRAINT export_invoice_party_type CHECK(party_type IN ('buyer','company_setup','manual'));
ALTER TABLE export_invoices ADD CONSTRAINT export_invoice_party_scope CHECK(
 (party_type='manual' AND company_id IS NULL AND party_id IS NULL AND buyer_id IS NULL AND party_setup_id IS NULL AND order_id IS NULL AND length(btrim(party_name)) BETWEEN 1 AND 160)
 OR (company_id IS NOT NULL AND party_id IS NOT NULL AND (
  (party_type='buyer' AND buyer_id IS NOT NULL AND buyer_id=party_id AND party_setup_id IS NULL)
  OR (party_type='company_setup' AND party_setup_id IS NOT NULL AND party_setup_id=party_id AND buyer_id IS NULL)
 )));
CREATE UNIQUE INDEX export_standalone_invoice_number_unique ON export_invoices(upper(btrim(invoice_number))) WHERE company_id IS NULL;
ALTER TABLE export_invoices ADD COLUMN workflow_status text NOT NULL DEFAULT 'Draft' CHECK(workflow_status IN ('Draft','Submitted','Approved','Changes requested'));
ALTER TABLE export_invoices ADD COLUMN content_revision integer NOT NULL DEFAULT 1 CHECK(content_revision>0);
ALTER TABLE export_invoices ADD COLUMN submitted_revision integer;
ALTER TABLE export_invoices ADD COLUMN submitted_by bigint REFERENCES users;
ALTER TABLE export_invoices ADD COLUMN submitted_at timestamptz;
ALTER TABLE export_invoices ADD COLUMN reviewed_by bigint REFERENCES users;
ALTER TABLE export_invoices ADD COLUMN reviewed_at timestamptz;
ALTER TABLE export_invoices ADD COLUMN review_note text NOT NULL DEFAULT '';
ALTER TABLE export_invoices ADD COLUMN approved_revision integer;
ALTER TABLE export_invoices ADD CONSTRAINT export_invoice_review_revision CHECK(
 (workflow_status<>'Submitted' OR (submitted_revision IS NOT NULL AND submitted_revision=content_revision AND submitted_by IS NOT NULL AND submitted_at IS NOT NULL))
 AND (workflow_status<>'Approved' OR (approved_revision IS NOT NULL AND approved_revision=content_revision AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL AND submitted_by IS NOT NULL AND reviewed_by<>created_by AND reviewed_by<>submitted_by))
 AND (workflow_status='Approved' OR approved_revision IS NULL));

-- Preserve the existing bound-company predicate exactly. Only fully unlinked
-- finance records enter the new shared register; the shape CHECK prevents an
-- invoice from using this branch to carry a company, buyer or production PO.
DROP POLICY export_scope ON export_invoices;
CREATE POLICY export_scope ON export_invoices USING(
 (company_id IS NULL AND party_type='manual' AND erp_identity()->>'exports'='true' AND erp_identity()->>'role' IN ('Super Admin','MD','Finance','Finance Head'))
 OR (company_id IS NOT NULL AND erp_export_allowed(company_id,buyer_id,order_id) AND (party_setup_id IS NULL OR EXISTS(SELECT 1 FROM company_setups s WHERE s.id=party_setup_id AND s.category='Buyer' AND s.status='Approved' AND s.deleted_at IS NULL)))
);
CREATE TABLE export_comments(
 id bigserial PRIMARY KEY, invoice_id bigint NOT NULL REFERENCES export_invoices,
 author_id bigint NOT NULL REFERENCES users, note text NOT NULL CHECK(length(btrim(note)) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX export_comments_parent ON export_comments(invoice_id,id);
CREATE TRIGGER export_comments_immutable BEFORE UPDATE OR DELETE ON export_comments FOR EACH ROW EXECUTE FUNCTION protect_audit();
ALTER TABLE export_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE export_comments FORCE ROW LEVEL SECURITY;
CREATE POLICY export_scope ON export_comments USING(EXISTS(SELECT 1 FROM export_invoices WHERE id=invoice_id));
GRANT SELECT,INSERT ON export_comments TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE export_comments_id_seq TO euroerp_runtime;
-- Grant assignments are managed separately through reviewed account controls.
INSERT INTO schema_versions(version) VALUES(22);

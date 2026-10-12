SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE export_invoices (
 id bigserial PRIMARY KEY, company_id bigint NOT NULL REFERENCES companies,
 party_type text NOT NULL CHECK(party_type IN ('buyer','company_setup')),
 party_id bigint NOT NULL, buyer_id bigint REFERENCES buyers, party_setup_id bigint REFERENCES company_setups,
 party_name text NOT NULL, order_id bigint REFERENCES orders,
 invoice_number text NOT NULL CHECK(length(invoice_number) BETWEEN 1 AND 100), currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 invoice_amount numeric(17,2) NOT NULL CHECK(invoice_amount>=0),
 profile jsonb NOT NULL DEFAULT '{}', receipts jsonb NOT NULL DEFAULT '[]', ebrcs jsonb NOT NULL DEFAULT '[]',
 finance_check jsonb NOT NULL DEFAULT '{}', audit_check jsonb NOT NULL DEFAULT '{}',
 created_by bigint NOT NULL REFERENCES users, version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((party_type='buyer' AND buyer_id IS NOT NULL AND buyer_id=party_id AND party_setup_id IS NULL) OR (party_type='company_setup' AND party_setup_id IS NOT NULL AND party_setup_id=party_id AND buyer_id IS NULL)),
 CHECK(jsonb_typeof(profile)='object' AND jsonb_typeof(receipts)='array' AND jsonb_typeof(ebrcs)='array')
);
CREATE UNIQUE INDEX export_invoice_number_unique ON export_invoices(company_id,upper(btrim(invoice_number)));
CREATE INDEX export_invoice_company_party ON export_invoices(company_id,party_type,party_id,id DESC);
CREATE TABLE export_discounts (
 id bigserial PRIMARY KEY, invoice_id bigint NOT NULL REFERENCES export_invoices,
 amount numeric(17,2) NOT NULL CHECK(amount>0), currency text NOT NULL, reason text NOT NULL,
 status text NOT NULL DEFAULT 'Pending finance' CHECK(status IN ('Pending finance','Pending final','Approved','Rejected')),
 requested_by bigint NOT NULL REFERENCES users, first_by bigint REFERENCES users, first_at timestamptz,
 final_by bigint REFERENCES users, final_at timestamptz, review_note text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(first_by IS NULL OR final_by IS NULL OR first_by<>final_by)
);
CREATE UNIQUE INDEX export_one_pending_discount ON export_discounts(invoice_id) WHERE status IN ('Pending finance','Pending final');
CREATE FUNCTION protect_export_approved_discount() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.status='Approved' THEN RAISE EXCEPTION 'Approved discounts are immutable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER export_approved_discount_immutable BEFORE UPDATE OR DELETE ON export_discounts FOR EACH ROW EXECUTE FUNCTION protect_export_approved_discount();
CREATE TABLE export_history (
 id bigserial PRIMARY KEY, invoice_id bigint NOT NULL REFERENCES export_invoices,
 actor_id bigint NOT NULL REFERENCES users, action text NOT NULL, snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX export_history_parent ON export_history(invoice_id,id DESC);
CREATE TRIGGER export_history_immutable BEFORE UPDATE OR DELETE ON export_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
CREATE TABLE export_files (
 id bigserial PRIMARY KEY, invoice_id bigint NOT NULL REFERENCES export_invoices,
 kind text NOT NULL, filename text NOT NULL, mime text NOT NULL, size integer NOT NULL CHECK(size>0 AND size<=10485760),
 sha256 text NOT NULL,ciphertext bytea NOT NULL,created_by bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX export_files_parent ON export_files(invoice_id,id);

CREATE FUNCTION erp_export_orders(cid bigint) RETURNS TABLE(id bigint,company_id bigint,buyer_id bigint,po text) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT o.id,o.company_id,o.buyer_id,o.po FROM orders o
 WHERE o.company_id=cid AND erp_identity()->>'exports'='true' AND erp_company_allowed(cid)
 AND (erp_identity()->>'role'='Super Admin' OR (
 (COALESCE(erp_identity()->>'restrict_buyers','false')<>'true' OR erp_identity()->'buyer_ids' @> to_jsonb(o.buyer_id)) AND
 (erp_identity()->>'role'<>'Finishing' OR erp_identity()->'units' @> to_jsonb(o.finishing_unit)) AND
 (erp_identity()->>'scope'='company' OR
 (erp_identity()->>'scope'='team' AND erp_identity()->'teams' @> to_jsonb(o.team_id)) OR
 (erp_identity()->>'scope'='line' AND erp_identity()->'lines' @> to_jsonb(o.line_id)) OR
 (erp_identity()->>'scope'='po' AND erp_identity()->'orders' @> to_jsonb(o.id)) OR
 (erp_identity()->>'scope' IN ('cutting_table','finishing_line') AND EXISTS(SELECT 1 FROM production_areas a WHERE a.company_id=cid AND a.kind=erp_identity()->>'scope' AND erp_identity()->'production_areas' @> to_jsonb(a.id) AND a.name=CASE a.kind WHEN 'cutting_table' THEN o.cutting_table ELSE o.sub_line END)))))
$$;
REVOKE ALL ON FUNCTION erp_export_orders(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION erp_export_orders(bigint) TO euroerp_runtime;

CREATE FUNCTION erp_export_allowed(cid bigint,bid bigint,oid bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT erp_identity()->>'exports'='true' AND erp_company_allowed(cid) AND (
 erp_identity()->>'role'='Super Admin' OR (
  (COALESCE(erp_identity()->>'restrict_buyers','false')<>'true' OR erp_identity()->'buyer_ids' @> to_jsonb(bid)) AND
  (erp_identity()->>'scope'='company' OR
   (erp_identity()->>'scope'='team' AND EXISTS(SELECT 1 FROM buyers b WHERE b.id=bid AND b.company_id=cid AND erp_identity()->'teams' @> to_jsonb(b.team_id))) OR
   (erp_identity()->>'scope' NOT IN ('company','team') AND EXISTS(SELECT 1 FROM erp_export_orders(cid) o WHERE o.id=oid AND o.buyer_id=bid)))
 ))
$$;
ALTER TABLE export_invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE export_invoices FORCE ROW LEVEL SECURITY;
CREATE POLICY export_scope ON export_invoices USING(erp_export_allowed(company_id,buyer_id,order_id) AND (party_setup_id IS NULL OR EXISTS(SELECT 1 FROM company_setups s WHERE s.id=party_setup_id AND s.category='Buyer' AND s.status='Approved' AND s.deleted_at IS NULL)));
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['export_discounts','export_history','export_files'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY export_scope ON %I USING(EXISTS(SELECT 1 FROM export_invoices WHERE id=invoice_id))',t);
 END LOOP;
END $$;
-- This directory returns public approved-buyer labels only, never KYC, banking,
-- contact, supplier or document data. Restricted staff cannot use unmapped buyers.
CREATE FUNCTION erp_export_buyers(cid bigint) RETURNS TABLE(id bigint,name text) LANGUAGE sql STABLE SECURITY INVOKER SET search_path=public,pg_temp AS $$
 SELECT id,legal_name FROM company_setups WHERE category='Buyer' AND status='Approved' AND deleted_at IS NULL
 AND erp_identity()->>'exports'='true' AND erp_company_allowed(cid)
 AND (erp_identity()->>'role'='Super Admin' OR (erp_identity()->>'scope'='company' AND COALESCE(erp_identity()->>'restrict_buyers','false')<>'true'))
$$;
REVOKE ALL ON FUNCTION erp_export_buyers(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION erp_export_buyers(bigint) TO euroerp_runtime;
GRANT SELECT,INSERT,UPDATE ON export_invoices,export_discounts TO euroerp_runtime;
GRANT SELECT,INSERT ON export_history,export_files TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE export_invoices_id_seq,export_discounts_id_seq,export_history_id_seq,export_files_id_seq TO euroerp_runtime;
INSERT INTO schema_versions(version) VALUES(19);

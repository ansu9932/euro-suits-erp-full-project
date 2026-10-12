SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE SEQUENCE enquiry_booking_number;
CREATE TABLE enquiries (
 id bigserial PRIMARY KEY,
 company_id bigint NOT NULL REFERENCES companies,
 buyer_id bigint NOT NULL REFERENCES buyers,
 created_by bigint NOT NULL REFERENCES users,
 development_id bigint REFERENCES development_masters,
 material_id bigint REFERENCES material_samples,
 profile jsonb NOT NULL DEFAULT '{}', colours jsonb NOT NULL DEFAULT '[]',
 fabric jsonb NOT NULL DEFAULT '{}', costing jsonb NOT NULL DEFAULT '{}',
 status text NOT NULL DEFAULT 'Draft' CHECK(status IN ('Draft','Fabric requested','Costing','Submitted','Changes requested','Approved','Booked','Cancelled')),
 booking_number bigint UNIQUE,
 review_note text NOT NULL DEFAULT '',reviewed_by bigint REFERENCES users,
 version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX enquiries_company_status ON enquiries(company_id,status,id DESC);
CREATE INDEX enquiries_buyer ON enquiries(buyer_id,id DESC);
CREATE TABLE enquiry_history (
 id bigserial PRIMARY KEY,enquiry_id bigint NOT NULL REFERENCES enquiries,
 actor_id bigint NOT NULL REFERENCES users,action text NOT NULL,snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX enquiry_history_parent ON enquiry_history(enquiry_id,id DESC);
CREATE TRIGGER enquiry_history_immutable BEFORE UPDATE OR DELETE ON enquiry_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
CREATE TABLE enquiry_files (
 id bigserial PRIMARY KEY,enquiry_id bigint NOT NULL REFERENCES enquiries,
 category text NOT NULL,name text NOT NULL,ciphertext bytea NOT NULL,order_id bigint REFERENCES orders,
 created_by bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX enquiry_files_parent ON enquiry_files(enquiry_id);
CREATE TABLE enquiry_options (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,
 kind text NOT NULL,label text NOT NULL,created_by bigint NOT NULL REFERENCES users
);
CREATE UNIQUE INDEX enquiry_options_unique ON enquiry_options(company_id,kind,lower(label));
ALTER TABLE orders ADD COLUMN booking_terms jsonb NOT NULL DEFAULT '{}';
ALTER TABLE orders ADD COLUMN enquiry_id bigint REFERENCES enquiries;
ALTER TABLE orders ADD COLUMN enquiry_consumption numeric(16,6) NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN target_consumption numeric(16,6) NOT NULL DEFAULT 0;
CREATE INDEX orders_enquiry ON orders(enquiry_id) WHERE enquiry_id IS NOT NULL;

ALTER TABLE enquiries ENABLE ROW LEVEL SECURITY;
ALTER TABLE enquiries FORCE ROW LEVEL SECURITY;
CREATE POLICY enquiry_scope ON enquiries USING (
 erp_identity()->>'enquiries'='true' AND erp_company_allowed(company_id)
 AND (erp_identity()->>'role'='Super Admin' OR (
  (erp_identity()->>'scope'='company' OR (erp_identity()->>'scope'='team' AND EXISTS(SELECT 1 FROM buyers b WHERE b.id=buyer_id AND erp_identity()->'teams' @> to_jsonb(b.team_id))))
  AND (COALESCE(erp_identity()->>'restrict_buyers','false')<>'true' OR erp_identity()->'buyer_ids' @> to_jsonb(buyer_id))
 ))
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['enquiry_history','enquiry_files'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY enquiry_scope ON %I USING(EXISTS(SELECT 1 FROM enquiries WHERE id=enquiry_id))',t);
 END LOOP;
END $$;
ALTER TABLE enquiry_options ENABLE ROW LEVEL SECURITY;
ALTER TABLE enquiry_options FORCE ROW LEVEL SECURITY;
CREATE POLICY option_scope ON enquiry_options USING(erp_identity()->>'enquiries'='true' AND erp_company_allowed(company_id));
GRANT SELECT,INSERT,UPDATE ON enquiries TO euroerp_runtime;
GRANT SELECT,INSERT ON enquiry_history,enquiry_files,enquiry_options TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE enquiries_id_seq,enquiry_history_id_seq,enquiry_files_id_seq,enquiry_options_id_seq,enquiry_booking_number TO euroerp_runtime;
UPDATE access_designations SET permissions=permissions||ARRAY['enquiries'] WHERE role IN ('Super Admin','MD','CEO','Marketing','Sourcing') AND NOT ('enquiries'=ANY(permissions));
UPDATE access_designations SET permissions=permissions||ARRAY['enquiry_costing'] WHERE role IN ('Super Admin','MD','CEO','Marketing') AND NOT ('enquiry_costing'=ANY(permissions));
UPDATE access_designations SET permissions=permissions||ARRAY['enquiry_fabric'] WHERE (role='Super Admin' OR (role='Sourcing' AND 'fabric_edit'=ANY(permissions))) AND NOT ('enquiry_fabric'=ANY(permissions));
UPDATE access_designations SET permissions=permissions||ARRAY['enquiry_approve'] WHERE role IN ('Super Admin','MD') AND NOT ('enquiry_approve'=ANY(permissions));
INSERT INTO schema_versions(version) VALUES(17);

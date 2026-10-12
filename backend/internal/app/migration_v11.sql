CREATE TABLE material_options (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,
 kind text NOT NULL, label text NOT NULL CHECK(length(label) BETWEEN 1 AND 150),
 parent_id bigint REFERENCES material_options,active boolean NOT NULL DEFAULT true,
 version integer NOT NULL DEFAULT 1,created_by bigint NOT NULL REFERENCES users,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX material_options_unique ON material_options(company_id,kind,COALESCE(parent_id,0),lower(label));
CREATE TABLE material_samples (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,
 supplier_setup_id bigint NOT NULL REFERENCES company_setups,
 kind text NOT NULL CHECK(kind IN ('fabric','trim')),code text UNIQUE NOT NULL,
 profile jsonb NOT NULL DEFAULT '{}',active boolean NOT NULL DEFAULT true,
 version integer NOT NULL DEFAULT 1,created_by bigint NOT NULL REFERENCES users,
 updated_by bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX material_samples_company ON material_samples(company_id,kind,id);
CREATE TABLE material_history (
 id bigserial PRIMARY KEY,sample_id bigint NOT NULL REFERENCES material_samples,
 actor_id bigint NOT NULL REFERENCES users,action text NOT NULL,
 snapshot jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER material_history_immutable BEFORE UPDATE OR DELETE ON material_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
CREATE TABLE material_photos (
 id bigserial PRIMARY KEY,sample_id bigint NOT NULL REFERENCES material_samples,
 name text NOT NULL,mime text NOT NULL,size integer NOT NULL,ciphertext bytea NOT NULL,
 created_by bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE supplier_assessments (
 supplier_setup_id bigint PRIMARY KEY REFERENCES company_setups,
 rating text NOT NULL DEFAULT 'Not rated' CHECK(rating IN ('Not rated','Good','Acceptable','Avoid','Blocked','Delisted')),
 reason text NOT NULL DEFAULT '',version integer NOT NULL DEFAULT 1,
 updated_by bigint NOT NULL REFERENCES users,updated_at timestamptz NOT NULL DEFAULT now()
);
-- The directory exposes approved company names only, never private KYC or bank details.
CREATE FUNCTION erp_material_suppliers() RETURNS TABLE(id bigint,legal_name text,country text,city text,company_type text) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT id,legal_name,country,city,company_type FROM company_setups
 WHERE category='Supplier' AND status='Approved' AND deleted_at IS NULL
 AND erp_identity()->>'id' IS NOT NULL
$$;
REVOKE ALL ON FUNCTION erp_material_suppliers() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION erp_material_suppliers() TO euroerp_runtime;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['material_options','material_samples'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY company_scope ON %I USING(erp_company_allowed(company_id))',t);
 END LOOP;
 FOREACH t IN ARRAY ARRAY['material_history','material_photos'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY sample_scope ON %I USING(EXISTS(SELECT 1 FROM material_samples WHERE id=sample_id))',t);
 END LOOP;
END $$;
GRANT SELECT,INSERT,UPDATE ON material_options,material_samples,supplier_assessments TO euroerp_runtime;
GRANT SELECT,INSERT ON material_history,material_photos TO euroerp_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO euroerp_runtime;
UPDATE access_designations SET permissions=permissions||ARRAY['material_view'] WHERE role IN ('Super Admin','MD','CEO','Marketing','Development','Sampling','Sourcing','Stores','Quality') AND NOT ('material_view'=ANY(permissions));
UPDATE access_designations SET permissions=permissions||ARRAY['material_commercial'] WHERE role IN ('Super Admin','MD','CEO') OR department IN ('Fabric Sourcing','Trims Sourcing');
UPDATE access_designations SET permissions=permissions||ARRAY['fabric_edit'] WHERE department='Fabric Sourcing' AND (name LIKE '%Manager%' OR name LIKE '%Executive%' OR name LIKE '%Head%');
UPDATE access_designations SET permissions=permissions||ARRAY['trim_edit'] WHERE department='Trims Sourcing' AND (name LIKE '%Manager%' OR name LIKE '%Executive%' OR name LIKE '%Head%');
UPDATE access_designations SET permissions=permissions||ARRAY['material_masters'] WHERE department='Fabric Sourcing' AND (name LIKE '%Manager%' OR name LIKE '%Head%');
UPDATE access_designations SET permissions=permissions||ARRAY['fabric_edit','trim_edit','material_masters'] WHERE role='Super Admin';
CREATE TABLE development_masters (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,buyer_id bigint REFERENCES buyers,
 code text UNIQUE NOT NULL,profile jsonb NOT NULL,material_ids bigint[] NOT NULL DEFAULT '{}',
 status text NOT NULL CHECK(status IN ('Draft','Submitted','Approved','Changes requested')),
 review_note text NOT NULL DEFAULT '',reviewed_by bigint REFERENCES users,created_by bigint NOT NULL REFERENCES users,
 version integer NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE development_history(id bigserial PRIMARY KEY,development_id bigint NOT NULL REFERENCES development_masters,actor_id bigint NOT NULL REFERENCES users,action text NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TRIGGER development_history_immutable BEFORE UPDATE OR DELETE ON development_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
ALTER TABLE development_masters ENABLE ROW LEVEL SECURITY;
ALTER TABLE development_masters FORCE ROW LEVEL SECURITY;
CREATE POLICY company_scope ON development_masters USING(erp_company_allowed(company_id) AND (erp_identity()->>'role'<>'Marketing' OR (buyer_id IS NULL AND created_by::text=erp_identity()->>'id') OR EXISTS(SELECT 1 FROM buyers b WHERE b.id=buyer_id AND (erp_identity()->>'scope'='company' OR (erp_identity()->>'scope'='team' AND erp_identity()->'teams' @> to_jsonb(b.team_id))))));
ALTER TABLE development_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE development_history FORCE ROW LEVEL SECURITY;
CREATE POLICY development_scope ON development_history USING(EXISTS(SELECT 1 FROM development_masters d WHERE d.id=development_id));
GRANT SELECT,INSERT,UPDATE ON development_masters TO euroerp_runtime;
GRANT SELECT,INSERT ON development_history TO euroerp_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO euroerp_runtime;
UPDATE access_designations SET permissions=permissions||ARRAY['development_master'] WHERE role IN ('Super Admin','Development') OR (role='Marketing' AND 'orders'=ANY(permissions));
ALTER TABLE supplier_assessments ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_assessments FORCE ROW LEVEL SECURITY;
CREATE POLICY supplier_read ON supplier_assessments FOR SELECT USING(erp_identity()->>'id' IS NOT NULL);
CREATE POLICY supplier_insert ON supplier_assessments FOR INSERT WITH CHECK(erp_identity()->>'role' IN ('MD','Super Admin'));
CREATE POLICY supplier_update ON supplier_assessments FOR UPDATE USING(erp_identity()->>'role' IN ('MD','Super Admin')) WITH CHECK(erp_identity()->>'role' IN ('MD','Super Admin'));
INSERT INTO schema_versions(version) VALUES(11);

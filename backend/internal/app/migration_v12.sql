CREATE SEQUENCE sample_indent_number START 999;
CREATE TABLE sample_requests (
 id bigserial PRIMARY KEY, company_id bigint NOT NULL REFERENCES companies,
 buyer_id bigint NOT NULL REFERENCES buyers, created_by bigint NOT NULL REFERENCES users,
 profile jsonb NOT NULL, variants jsonb NOT NULL, bom jsonb NOT NULL,
 status text NOT NULL DEFAULT 'Draft' CHECK(status IN ('Draft','Submitted','Approved','Changes requested')),
 review_note text NOT NULL DEFAULT '',reviewed_by bigint REFERENCES users,
 version integer NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sample_indents (
 id bigint PRIMARY KEY DEFAULT nextval('sample_indent_number'),request_id bigint NOT NULL REFERENCES sample_requests,
 size text NOT NULL,colour text NOT NULL,status text NOT NULL DEFAULT 'Ready',
 production jsonb NOT NULL DEFAULT '{}',version integer NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sample_request_history (
 id bigserial PRIMARY KEY,request_id bigint NOT NULL REFERENCES sample_requests,actor_id bigint NOT NULL REFERENCES users,
 action text NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER sample_history_immutable BEFORE UPDATE OR DELETE ON sample_request_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
CREATE TABLE sample_request_files (
 id bigserial PRIMARY KEY,request_id bigint NOT NULL REFERENCES sample_requests,category text NOT NULL,
 name text NOT NULL,ciphertext bytea NOT NULL,created_by bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sample_options (id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,kind text NOT NULL,label text NOT NULL,UNIQUE(company_id,kind,label));
CREATE TABLE sample_stock (
 material_id bigint PRIMARY KEY REFERENCES material_samples,company_id bigint NOT NULL REFERENCES companies,
 metres numeric(12,3) NOT NULL DEFAULT 0 CHECK(metres>=0)
);
CREATE TABLE sample_stock_moves (
 id bigserial PRIMARY KEY,material_id bigint NOT NULL REFERENCES sample_stock,company_id bigint NOT NULL REFERENCES companies,
 indent_id bigint REFERENCES sample_indents,metres numeric(12,3) NOT NULL CHECK(metres<>0),reference text NOT NULL,
 actor_id bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE sample_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE sample_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY request_scope ON sample_requests USING(erp_company_allowed(company_id) AND (erp_identity()->>'role'<>'Marketing' OR EXISTS(SELECT 1 FROM buyers b WHERE b.id=buyer_id AND (erp_identity()->>'scope'='company' OR (erp_identity()->>'scope'='team' AND erp_identity()->'teams' @> to_jsonb(b.team_id))))));
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['sample_indents','sample_request_history','sample_request_files'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY request_scope ON %I USING(EXISTS(SELECT 1 FROM sample_requests WHERE id=request_id))',t);
 END LOOP;
 FOREACH t IN ARRAY ARRAY['sample_options','sample_stock','sample_stock_moves'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY company_scope ON %I USING(erp_company_allowed(company_id))',t);
 END LOOP;
END $$;
GRANT SELECT,INSERT,UPDATE ON sample_requests,sample_indents,sample_options,sample_stock TO euroerp_runtime;
GRANT SELECT,INSERT ON sample_request_history,sample_request_files,sample_stock_moves TO euroerp_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO euroerp_runtime;
UPDATE access_designations SET permissions=permissions||ARRAY['sample_indents'] WHERE role IN ('Super Admin','MD','Marketing','Sampling','Development','Quality','Stores') AND NOT ('sample_indents'=ANY(permissions));
INSERT INTO schema_versions(version) VALUES(12);

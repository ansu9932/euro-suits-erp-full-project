SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

-- Invoice customer/company choices are independent from Company Setup,
-- operational companies and scoped buyer masters. Existing invoices keep text.
CREATE TABLE export_party_options (
 id bigserial PRIMARY KEY,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160 AND name=btrim(name) AND name !~ '[[:cntrl:]]'),
 name_key text GENERATED ALWAYS AS (lower(regexp_replace(btrim(name),'[[:space:]]+',' ','g'))) STORED,
 created_by bigint REFERENCES users,
 created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT export_party_option_name_unique UNIQUE(name_key)
);
CREATE TRIGGER export_party_options_immutable BEFORE UPDATE OR DELETE ON export_party_options FOR EACH ROW EXECUTE FUNCTION protect_audit();
ALTER TABLE export_party_options ENABLE ROW LEVEL SECURITY;
ALTER TABLE export_party_options FORCE ROW LEVEL SECURITY;
CREATE POLICY export_party_option_read ON export_party_options FOR SELECT USING(
 erp_identity()->>'exports'='true' AND erp_identity()->>'role' IN ('Finance','Finance Head','MD','Super Admin')
);
CREATE POLICY export_party_option_add ON export_party_options FOR INSERT WITH CHECK(
 erp_identity()->>'exports'='true' AND erp_identity()->>'exports_edit'='true'
 AND erp_identity()->>'role' IN ('Finance','Finance Head','Super Admin')
 AND created_by=(erp_identity()->>'id')::bigint
);
GRANT SELECT,INSERT ON export_party_options TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE export_party_options_id_seq TO euroerp_runtime;

-- Three user-supplied production choices, preserving the exact spellings.
-- No fourth name is inferred and no Company Setup/buyer records are created.
INSERT INTO export_party_options(name) VALUES
 ('EURO TROUSERS MFG,CO, (FZC)'),
 ('P & C RETAIL UK LTD'),
 ('ERRY ELLIS EUROPE LIMITED');

INSERT INTO schema_versions(version) VALUES(23);

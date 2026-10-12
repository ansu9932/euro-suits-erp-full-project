SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

ALTER TABLE company_field_definitions ADD COLUMN company_setup_id bigint REFERENCES company_setups(id);
ALTER TABLE company_field_definitions ADD CONSTRAINT company_field_local_section_check
 CHECK(company_setup_id IS NULL OR section='Additional');
ALTER TABLE company_field_definitions DROP CONSTRAINT company_field_definitions_label_country_section_key;
CREATE UNIQUE INDEX company_field_global_identity ON company_field_definitions(label,country,section) WHERE company_setup_id IS NULL;
CREATE UNIQUE INDEX company_field_local_identity ON company_field_definitions(company_setup_id,lower(label)) WHERE company_setup_id IS NOT NULL;
CREATE INDEX company_field_local_lookup ON company_field_definitions(company_setup_id) WHERE company_setup_id IS NOT NULL;

ALTER TABLE company_field_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE company_field_definitions FORCE ROW LEVEL SECURITY;
CREATE POLICY company_field_read ON company_field_definitions FOR SELECT USING (
 company_setup_id IS NULL OR EXISTS(SELECT 1 FROM company_setups c WHERE c.id=company_setup_id AND c.deleted_at IS NULL)
);
CREATE POLICY company_field_insert ON company_field_definitions FOR INSERT WITH CHECK (
 (company_setup_id IS NULL AND erp_identity()->>'role'='Super Admin') OR
 (company_setup_id IS NOT NULL AND section='Additional' AND erp_identity()->>'role'='Finance Head'
  AND erp_identity()->>'view_setup'='true'
  AND EXISTS(SELECT 1 FROM company_setups c WHERE c.id=company_setup_id AND c.deleted_at IS NULL
   AND c.assigned_to::text=erp_identity()->>'id' AND c.status IN ('Draft','Changes requested')))
);
INSERT INTO schema_versions(version) VALUES(21);

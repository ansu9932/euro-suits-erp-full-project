ALTER TABLE company_setups ADD COLUMN deleted_at timestamptz;
ALTER TABLE company_setups ADD COLUMN deleted_by bigint REFERENCES users;
DROP INDEX company_identity_unique;
CREATE UNIQUE INDEX company_identity_unique ON company_setups(lower(legal_name),country,category) WHERE deleted_at IS NULL;
INSERT INTO schema_versions(version) VALUES(6);

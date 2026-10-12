CREATE TABLE company_custom_types (
 id bigserial PRIMARY KEY,
 category text NOT NULL CHECK(category IN ('Internal','Buyer','Supplier')),
 label text NOT NULL CHECK(length(label) BETWEEN 1 AND 100),
 created_by bigint NOT NULL REFERENCES users,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX company_custom_type_unique ON company_custom_types(category,lower(label));
UPDATE company_reminders SET remind_on=(expires_on-interval '2 months')::date;
INSERT INTO schema_versions(version) VALUES(4);

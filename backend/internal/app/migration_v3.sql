ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK(role IN ('Super Admin','Finance Head','MD','CEO','Finance','Marketing','Planning','Development','Sourcing','Stores','Sampling','Cutting','Production','Finishing','Quality','Warehouse'));
CREATE TABLE company_setups (
 id bigserial PRIMARY KEY, legal_name text NOT NULL CHECK(length(legal_name) BETWEEN 1 AND 150),
 category text NOT NULL CHECK(category IN ('Internal','Buyer','Supplier')),
 company_type text NOT NULL, country text NOT NULL CHECK(country ~ '^[A-Z]{2}$'),
 city text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'Draft' CHECK(status IN ('Draft','Pending','Approved','Changes requested')),
 private_data bytea NOT NULL, revision int NOT NULL DEFAULT 1, version int NOT NULL DEFAULT 1,
 created_by bigint NOT NULL REFERENCES users, approved_by bigint REFERENCES users,
 approval_note text NOT NULL DEFAULT '', submitted_at timestamptz, approved_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX company_identity_unique ON company_setups(lower(legal_name),country,category);
CREATE TABLE company_setup_files (
 id bigserial PRIMARY KEY, company_id bigint NOT NULL REFERENCES company_setups, name text NOT NULL,
 storage_key text NOT NULL UNIQUE, size bigint NOT NULL CHECK(size>0), sha256 text NOT NULL,
 created_by bigint NOT NULL REFERENCES users, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE company_setup_history (
 id bigserial PRIMARY KEY, company_id bigint NOT NULL REFERENCES company_setups,
 actor_id bigint NOT NULL REFERENCES users, action text NOT NULL, note text NOT NULL DEFAULT '',
 revision int NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE company_field_definitions (
 id bigserial PRIMARY KEY, label text NOT NULL CHECK(length(label) BETWEEN 1 AND 100),
 section text NOT NULL CHECK(section IN ('KYC','Property','Insurance','Additional')),
 country text NOT NULL DEFAULT '*', categories text[] NOT NULL DEFAULT '{Internal,Buyer,Supplier}',
 input_type text NOT NULL DEFAULT 'text' CHECK(input_type IN ('text','number','date','url')),
 required boolean NOT NULL DEFAULT false, document_required boolean NOT NULL DEFAULT false,
 expiry_enabled boolean NOT NULL DEFAULT false, active boolean NOT NULL DEFAULT true,
 UNIQUE(label,country,section)
);
INSERT INTO company_field_definitions(label,section,country,document_required,expiry_enabled) VALUES
 ('Business / incorporation licence','KYC','*',true,true),
 ('GST registration','KYC','IN',true,false),('PAN','KYC','IN',true,false),
 ('CIN','KYC','IN',true,false),('IEC','KYC','IN',true,false),('DIN','KYC','IN',false,false),
 ('Property tax','Property','*',true,true),
 ('Building insurance','Insurance','*',true,true),('Plant and machinery insurance','Insurance','*',true,true),
 ('Stock and raw materials insurance','Insurance','*',true,true),('Vehicle insurance','Insurance','*',true,true),
 ('Consequential loss insurance','Insurance','*',true,true);
CREATE TABLE company_ports (
 id bigserial PRIMARY KEY,name text NOT NULL,code text NOT NULL DEFAULT '',country text NOT NULL CHECK(country ~ '^[A-Z]{2}$'),
 mode text NOT NULL CHECK(mode IN ('Sea','Air','ICD')),active boolean NOT NULL DEFAULT true,UNIQUE(name,country,mode)
);
CREATE TABLE company_reminders (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES company_setups,obligation_key text NOT NULL,
 label text NOT NULL,expires_on date NOT NULL,remind_on date NOT NULL,
 email_status text NOT NULL DEFAULT 'Pending' CHECK(email_status IN ('Pending','Sent','Failed','Not configured')),
 attempts int NOT NULL DEFAULT 0,next_attempt timestamptz NOT NULL DEFAULT now(),sent_at timestamptz,
 last_error text NOT NULL DEFAULT '',active boolean NOT NULL DEFAULT true,
 UNIQUE(company_id,obligation_key,expires_on)
);
CREATE INDEX company_reminders_due ON company_reminders(remind_on) WHERE active;
CREATE TRIGGER company_history_immutable BEFORE UPDATE OR DELETE ON company_setup_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
INSERT INTO schema_versions(version) VALUES(3);

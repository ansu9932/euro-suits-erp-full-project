ALTER TABLE users ADD COLUMN setup_features text[] NOT NULL DEFAULT '{}';
ALTER TABLE users ADD COLUMN setup_custom_access boolean NOT NULL DEFAULT false;
ALTER TABLE company_field_definitions DROP CONSTRAINT company_field_definitions_section_check;
ALTER TABLE company_field_definitions ADD CONSTRAINT company_field_definitions_section_check CHECK(section IN ('Overview','Contacts','KYC','Bank accounts','Ports','Property','Insurance','Additional','Documents'));
ALTER TABLE company_field_definitions DROP CONSTRAINT company_field_definitions_input_type_check;
ALTER TABLE company_field_definitions ADD CONSTRAINT company_field_definitions_input_type_check CHECK(input_type IN ('text','textarea','number','date','url','email','tel','select','checkbox'));
ALTER TABLE company_field_definitions ADD COLUMN options text[] NOT NULL DEFAULT '{}';
CREATE TABLE setup_access_history (
 id bigserial PRIMARY KEY, actor_id bigint NOT NULL REFERENCES users, user_id bigint NOT NULL REFERENCES users,
 action text NOT NULL, detail jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER setup_access_history_immutable BEFORE UPDATE OR DELETE ON setup_access_history FOR EACH ROW EXECUTE FUNCTION protect_audit();
INSERT INTO schema_versions(version) VALUES(5);

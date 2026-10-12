ALTER TABLE users ADD COLUMN ip_restricted boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN allowed_ips text[] NOT NULL DEFAULT '{}';
ALTER TABLE users ADD CONSTRAINT developer_no_ip_restriction CHECK (NOT is_developer OR (NOT ip_restricted AND cardinality(allowed_ips)=0));
ALTER TABLE users ADD CONSTRAINT restricted_ip_list_required CHECK (NOT ip_restricted OR cardinality(allowed_ips)>0);
INSERT INTO schema_versions(version) VALUES(13);

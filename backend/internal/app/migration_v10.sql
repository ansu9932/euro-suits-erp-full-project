ALTER TABLE users ADD COLUMN reports_to_id bigint REFERENCES users(id);
ALTER TABLE users ADD COLUMN territory text NOT NULL DEFAULT '' CHECK(length(territory)<=120);
ALTER TABLE users ADD COLUMN restrict_buyers boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN buyer_ids bigint[] NOT NULL DEFAULT '{}';
ALTER TABLE users ADD CONSTRAINT user_not_own_manager CHECK(reports_to_id IS NULL OR reports_to_id<>id);
CREATE INDEX user_reporting_manager ON users(reports_to_id);

ALTER TABLE users ADD COLUMN production_areas bigint[] NOT NULL DEFAULT '{}';
ALTER TABLE users DROP CONSTRAINT users_scope_check;
ALTER TABLE users ADD CONSTRAINT users_scope_check CHECK(scope IN ('company','team','line','po','cutting_table','finishing_line'));
CREATE TABLE production_areas (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,
 kind text NOT NULL CHECK(kind IN ('cutting_table','finishing_line')),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),UNIQUE(company_id,kind,name)
);
GRANT SELECT,INSERT,UPDATE,DELETE ON production_areas TO euroerp_runtime;
GRANT USAGE,SELECT ON SEQUENCE production_areas_id_seq TO euroerp_runtime;
ALTER TABLE production_areas ENABLE ROW LEVEL SECURITY;
ALTER TABLE production_areas FORCE ROW LEVEL SECURITY;
CREATE POLICY company_scope ON production_areas USING(erp_company_allowed(company_id));
-- Preserve existing allocations as selectable areas; do not create sample data.
INSERT INTO production_areas(company_id,kind,name)
 SELECT DISTINCT company_id,'cutting_table',cutting_table FROM orders WHERE cutting_table<>''
 UNION SELECT DISTINCT company_id,'finishing_line',sub_line FROM orders WHERE sub_line<>'';
CREATE OR REPLACE FUNCTION erp_order_allowed(cid bigint,tid bigint,lid bigint,oid bigint,unit text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT erp_service()='reminders' OR erp_identity()->>'role'='Super Admin' OR (
  erp_identity()->>'view_orders'='true' AND erp_company_allowed(cid) AND
  (erp_identity()->>'scope' IN ('company','cutting_table','finishing_line') OR
   (erp_identity()->>'scope'='team' AND erp_identity()->'teams' @> to_jsonb(tid)) OR
   (erp_identity()->>'scope'='line' AND erp_identity()->'lines' @> to_jsonb(lid)) OR
   (erp_identity()->>'scope'='po' AND erp_identity()->'orders' @> to_jsonb(oid))) AND
  (erp_identity()->>'role'<>'Finishing' OR erp_identity()->'units' @> to_jsonb(unit)))
$$;
CREATE FUNCTION erp_area_allowed(cid bigint,ct text,fl text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT erp_service()='reminders' OR erp_identity()->>'role'='Super Admin' OR
 COALESCE(erp_identity()->>'scope','') NOT IN ('cutting_table','finishing_line') OR
 EXISTS(SELECT 1 FROM production_areas a WHERE a.company_id=cid AND a.kind=erp_identity()->>'scope'
 AND a.name=CASE a.kind WHEN 'cutting_table' THEN ct ELSE fl END
 AND erp_identity()->'production_areas' @> to_jsonb(a.id))
$$;
CREATE FUNCTION erp_buyer_allowed(bid bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT erp_identity()->>'role'='Super Admin' OR
 COALESCE(erp_identity()->>'restrict_buyers','false')<>'true' OR
 COALESCE(erp_identity()->'buyer_ids','[]'::jsonb) @> to_jsonb(bid)
$$;
ALTER POLICY order_scope ON orders USING (
 erp_order_allowed(company_id,team_id,line_id,id,finishing_unit) AND erp_buyer_allowed(buyer_id) AND erp_area_allowed(company_id,cutting_table,sub_line)
);
ALTER POLICY company_scope ON buyers USING (erp_company_allowed(company_id) AND erp_buyer_allowed(id));

ALTER POLICY company_scope ON suppliers USING (erp_company_allowed(company_id) AND (buyer_id IS NULL OR erp_buyer_allowed(buyer_id)));
ALTER POLICY purchase_scope ON purchases USING (
 erp_company_allowed(company_id) AND (erp_identity()->>'role'='Super Admin' OR
 (erp_identity()->>'scope'='company' AND COALESCE(erp_identity()->>'restrict_buyers','false')<>'true') OR
 EXISTS(SELECT 1 FROM orders WHERE id=order_id))
);

INSERT INTO access_designations(department,name,role,permissions,features) VALUES
 ('Marketing & Merchandising','Marketing Director','Marketing',ARRAY['view_orders','orders','documents']::text[],'{}'),
 ('Marketing & Merchandising','Marketing General Manager','Marketing',ARRAY['view_orders','orders','documents']::text[],'{}'),
 ('Marketing & Merchandising','Regional Marketing Manager','Marketing',ARRAY['view_orders','orders','documents']::text[],'{}'),
 ('Marketing & Merchandising','Assistant Merchant','Marketing',ARRAY['view_orders','orders','documents']::text[],'{}'),
 ('Marketing & Merchandising','Trainee / Intern','Marketing',ARRAY['view_orders']::text[],'{}'),
 ('Product Development','Development Coordinator','Development',ARRAY['view_orders','develop','documents']::text[],'{}'),
 ('Product Development','Development QA','Quality',ARRAY['view_orders','quality','documents']::text[],'{}'),
 ('Planning & PP','PPD Trainee','Planning',ARRAY['view_orders']::text[],'{}'),
 ('Product Development','Showroom Executive','Development',ARRAY['view_orders','develop','documents']::text[],'{}'),
 ('Fabric Sourcing','Fabric Sourcing Trainee','Sourcing',ARRAY['view_orders']::text[],'{}'),
 ('Fabric Sourcing','Fabric Technologist','Quality',ARRAY['view_orders','quality','documents']::text[],'{}'),
 ('Trims Sourcing','Trims Sourcing Trainee','Sourcing',ARRAY['view_orders']::text[],'{}'),
 ('Sampling','Assistant Sampling Manager','Sampling',ARRAY['view_orders','sample','documents']::text[],'{}'),
 ('Sampling','Sampling QA Manager','Quality',ARRAY['view_orders','quality','documents']::text[],'{}'),
 ('Cutting','Assistant Cutting Manager','Cutting',ARRAY['view_orders','cut','documents']::text[],'{}'),
 ('Sewing','Industrial Engineer','Production',ARRAY['view_orders','documents']::text[],'{}'),
 ('Sewing','IE Executive','Production',ARRAY['view_orders','documents']::text[],'{}'),
 ('Sewing','Sewing Team Leader','Production',ARRAY['view_orders','sew','documents']::text[],'{}'),
 ('Finishing','Finishing Supervisor','Finishing',ARRAY['view_orders','finish','documents']::text[],'{}'),
 ('Finishing','Trimming Executive','Finishing',ARRAY['view_orders','finish','documents']::text[],'{}'),
 ('Finishing','Button Attach / Bartack Executive','Finishing',ARRAY['view_orders','finish','documents']::text[],'{}'),
 ('Security','Gate Security Executive','Warehouse',ARRAY['view_orders']::text[],'{}')
ON CONFLICT(department,name) DO NOTHING;
INSERT INTO schema_versions(version) VALUES(10);

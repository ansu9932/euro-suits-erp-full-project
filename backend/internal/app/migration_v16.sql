ALTER TABLE sample_request_files ADD COLUMN indent_id bigint REFERENCES sample_indents;
CREATE INDEX sample_files_indent ON sample_request_files(indent_id) WHERE indent_id IS NOT NULL;
CREATE INDEX sample_planning_status ON sample_indents(status,id DESC);
INSERT INTO schema_versions(version) VALUES(16);
CREATE TABLE sample_capacities (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,
 resource text NOT NULL CHECK(length(resource) BETWEEN 1 AND 80), effective_date date NOT NULL,
 tailors integer NOT NULL CHECK(tailors BETWEEN 0 AND 500), pieces_per_tailor integer NOT NULL CHECK(pieces_per_tailor BETWEEN 1 AND 200),
 weekly_off integer[] NOT NULL DEFAULT '{0}',holidays date[] NOT NULL DEFAULT '{}',created_by bigint NOT NULL REFERENCES users,
 version integer NOT NULL DEFAULT 1,UNIQUE(company_id,resource,effective_date)
);
CREATE TABLE sample_plans (
 id bigserial PRIMARY KEY,company_id bigint NOT NULL REFERENCES companies,resource text NOT NULL,
 start_date date NOT NULL,created_by bigint NOT NULL REFERENCES users,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sample_plan_items (
 indent_id bigint PRIMARY KEY REFERENCES sample_indents,plan_id bigint NOT NULL REFERENCES sample_plans,
 position integer NOT NULL,plan_date date NOT NULL
);
CREATE INDEX sample_plan_dates ON sample_plan_items(plan_id,plan_date);
ALTER TABLE sample_capacities ENABLE ROW LEVEL SECURITY;
ALTER TABLE sample_capacities FORCE ROW LEVEL SECURITY;
CREATE POLICY capacity_read ON sample_capacities FOR SELECT USING(erp_company_allowed(company_id));
CREATE POLICY capacity_add ON sample_capacities FOR INSERT WITH CHECK(erp_company_allowed(company_id) AND erp_identity()->>'role'='Super Admin');
CREATE POLICY capacity_edit ON sample_capacities FOR UPDATE USING(erp_company_allowed(company_id) AND erp_identity()->>'role'='Super Admin');
ALTER TABLE sample_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE sample_plans FORCE ROW LEVEL SECURITY;
CREATE POLICY company_scope ON sample_plans USING(erp_company_allowed(company_id));
ALTER TABLE sample_plan_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE sample_plan_items FORCE ROW LEVEL SECURITY;
CREATE POLICY indent_scope ON sample_plan_items USING(EXISTS(SELECT 1 FROM sample_indents WHERE id=indent_id));
GRANT SELECT,INSERT,UPDATE ON sample_capacities TO euroerp_runtime;
GRANT SELECT,INSERT ON sample_plans,sample_plan_items TO euroerp_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO euroerp_runtime;
-- Return only totals, never hidden buyer or garment identities. The owner reads
-- all bookings so a buyer-restricted coordinator cannot overbook a shared line.
CREATE FUNCTION erp_sample_bookings(cid bigint,resource_name text,from_date date)
RETURNS TABLE(plan_date text,count bigint) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT i.plan_date::text,count(*) FROM sample_plan_items i JOIN sample_plans p ON p.id=i.plan_id
 WHERE p.company_id=cid AND p.resource=resource_name AND i.plan_date>=from_date
 AND erp_company_allowed(cid) AND erp_identity()->>'role' IN ('Super Admin','Sampling')
 GROUP BY i.plan_date ORDER BY i.plan_date
$$;
REVOKE ALL ON FUNCTION erp_sample_bookings(bigint,text,date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION erp_sample_bookings(bigint,text,date) TO euroerp_runtime;

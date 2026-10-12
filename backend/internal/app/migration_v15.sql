-- Fail quickly if an active transaction prevents this small-table migration.
-- For a large installation, create these indexes concurrently before upgrading.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
CREATE INDEX IF NOT EXISTS milestones_order_due ON milestones(order_id,due_date);
CREATE INDEX IF NOT EXISTS milestones_pending_due ON milestones(due_date,order_id) WHERE NOT done;
CREATE INDEX IF NOT EXISTS work_items_order ON work_items(order_id);
CREATE INDEX IF NOT EXISTS quality_order ON quality(order_id,id DESC);
CREATE INDEX IF NOT EXISTS materials_order ON materials(order_id);
CREATE INDEX IF NOT EXISTS purchases_order ON purchases(order_id);
CREATE INDEX IF NOT EXISTS buyers_team ON buyers(team_id);
CREATE INDEX IF NOT EXISTS company_responsible ON company_setups(responsible_user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS company_reminders_expiry ON company_reminders(company_id,expires_on) WHERE active;
CREATE INDEX IF NOT EXISTS company_reminders_delivery ON company_reminders(next_attempt,remind_on) WHERE active;
CREATE INDEX IF NOT EXISTS company_files_company ON company_setup_files(company_id);
CREATE INDEX IF NOT EXISTS company_history_company ON company_setup_history(company_id,id DESC);
CREATE INDEX IF NOT EXISTS material_photos_sample ON material_photos(sample_id);
CREATE INDEX IF NOT EXISTS material_history_sample ON material_history(sample_id,id DESC);
CREATE INDEX IF NOT EXISTS sample_requests_company ON sample_requests(company_id,id DESC);
CREATE INDEX IF NOT EXISTS sample_indents_request ON sample_indents(request_id);
CREATE INDEX IF NOT EXISTS sample_files_request ON sample_request_files(request_id);
CREATE INDEX IF NOT EXISTS sample_history_request ON sample_request_history(request_id,id DESC);
CREATE INDEX IF NOT EXISTS sample_stock_material ON sample_stock_moves(material_id,id DESC);
CREATE INDEX IF NOT EXISTS development_history_parent ON development_history(development_id,id DESC);
-- Evaluate request claims once per query, not once for every row in nested joins.
-- Keep the same company/team/line/PO/buyer/finishing-area checks as v10.
ALTER POLICY order_scope ON orders USING (
 (
  (SELECT erp_service())='reminders' OR
  (SELECT erp_identity()->>'role')='Super Admin' OR (
   (SELECT erp_identity()->>'view_orders')='true' AND
   ((SELECT erp_identity()->>'role')='Super Admin' OR
    COALESCE((SELECT erp_identity()->'companies'),'[]'::jsonb) @> to_jsonb(company_id)) AND
   ((SELECT erp_identity()->>'scope') IN ('company','cutting_table','finishing_line') OR
    ((SELECT erp_identity()->>'scope')='team' AND (SELECT erp_identity()->'teams') @> to_jsonb(team_id)) OR
    ((SELECT erp_identity()->>'scope')='line' AND (SELECT erp_identity()->'lines') @> to_jsonb(line_id)) OR
    ((SELECT erp_identity()->>'scope')='po' AND (SELECT erp_identity()->'orders') @> to_jsonb(id))) AND
   ((SELECT erp_identity()->>'role')<>'Finishing' OR (SELECT erp_identity()->'units') @> to_jsonb(finishing_unit))
  )
 ) AND (
  (SELECT erp_identity()->>'role')='Super Admin' OR
  COALESCE((SELECT erp_identity()->>'restrict_buyers'),'false')<>'true' OR
  COALESCE((SELECT erp_identity()->'buyer_ids'),'[]'::jsonb) @> to_jsonb(buyer_id)
 ) AND (
  (SELECT erp_service())='reminders' OR
  (SELECT erp_identity()->>'role')='Super Admin' OR
  COALESCE((SELECT erp_identity()->>'scope'),'') NOT IN ('cutting_table','finishing_line') OR
  EXISTS(SELECT 1 FROM production_areas a WHERE a.company_id=orders.company_id
   AND a.kind=(SELECT erp_identity()->>'scope')
   AND a.name=CASE a.kind WHEN 'cutting_table' THEN orders.cutting_table ELSE orders.sub_line END
   AND (SELECT erp_identity()->'production_areas') @> to_jsonb(a.id))
 )
);

INSERT INTO schema_versions(version) VALUES(15);

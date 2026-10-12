ALTER TABLE orders ALTER COLUMN stage SET DEFAULT 'Planning';
ALTER TABLE users ADD COLUMN permissions text[] NOT NULL DEFAULT '{}';
ALTER TABLE users ADD COLUMN custom_permissions boolean NOT NULL DEFAULT false;
-- Repair orders created before automatic department routing existed, without resetting work already done.
UPDATE orders o SET stage='Planning',version=version+1,updated_at=now()
 WHERE stage='Development' AND NOT EXISTS(SELECT 1 FROM work_items w WHERE w.order_id=o.id);
INSERT INTO work_items(order_id,kind,title,status)
 SELECT o.id,k.kind,o.style,'Open' FROM orders o CROSS JOIN (VALUES('Style development'),('CAD / pattern')) k(kind)
 WHERE o.stage='Development' AND NOT EXISTS(SELECT 1 FROM work_items w WHERE w.order_id=o.id AND w.kind=k.kind);
INSERT INTO work_items(order_id,kind,title,status)
 SELECT id,'Pre-production sample',style,'Open' FROM orders o WHERE stage='Sampling'
 AND NOT EXISTS(SELECT 1 FROM work_items w WHERE w.order_id=o.id AND w.kind='Pre-production sample');
INSERT INTO schema_versions(version) VALUES(2);

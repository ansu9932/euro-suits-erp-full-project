SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

-- Email opt-out does not remove the expiry obligation or responsibility block.
ALTER TABLE company_reminders DROP CONSTRAINT company_reminders_email_status_check;
ALTER TABLE company_reminders ADD CONSTRAINT company_reminders_email_status_check
 CHECK(email_status IN ('Pending','Sent','Failed','Not configured','Disabled'));
INSERT INTO schema_versions(version) VALUES(20);

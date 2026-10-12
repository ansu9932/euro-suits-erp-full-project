-- Additive company flow selection; existing approvals, encrypted profiles and history are unchanged.
ALTER TABLE company_setups ADD COLUMN business_flows text[] NOT NULL DEFAULT '{}';
UPDATE company_setups SET business_flows=CASE business_mode
  WHEN 'Domestic' THEN ARRAY['Domestic']
  WHEN 'Import' THEN ARRAY['Import']
  WHEN 'Export' THEN ARRAY['Export']
  WHEN 'Import & export' THEN ARRAY['Import','Export']
  ELSE ARRAY[]::text[]
END;

-- Canonical arrays are one-dimensional, ordered, duplicate-free and contain no nulls.
ALTER TABLE company_setups ADD CONSTRAINT company_setups_business_flows_valid CHECK (
  business_flows = array_remove(ARRAY[
    CASE WHEN 'Domestic'=ANY(business_flows) THEN 'Domestic'::text END,
    CASE WHEN 'Import'=ANY(business_flows) THEN 'Import'::text END,
    CASE WHEN 'Export'=ANY(business_flows) THEN 'Export'::text END
  ], NULL)
);
-- Retain the scalar for older clients; current APIs and KYC rules use the full array.
ALTER TABLE company_setups ADD CONSTRAINT company_setups_business_flows_legacy_consistent CHECK (
  business_mode = CASE
    WHEN business_flows @> ARRAY['Import','Export'] THEN 'Import & export'
    WHEN 'Import'=ANY(business_flows) THEN 'Import'
    WHEN 'Export'=ANY(business_flows) THEN 'Export'
    WHEN 'Domestic'=ANY(business_flows) THEN 'Domestic'
    ELSE 'Not specified'
  END
);
INSERT INTO schema_versions(version) VALUES(18);

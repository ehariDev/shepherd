-- 0026_org_attribute_matching_flags.down.sql
ALTER TABLE orgs
    DROP COLUMN IF EXISTS allow_label_matching,
    DROP COLUMN IF EXISTS allow_local_attribute_matching;

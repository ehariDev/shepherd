-- name: CreateOrg :one
INSERT INTO orgs (name, display_name, admin_group_id, reader_group_id, editor_group_id, tenant_id)
VALUES ($1, $2, $3, $4, sqlc.narg('editor_group_id'), sqlc.narg('tenant_id'))
RETURNING *;

-- name: GetOrgByID :one
SELECT * FROM orgs WHERE id = $1;

-- name: CountOrgContent :one
-- Backs DeleteOrg's not-empty check: an org with any cluster or pipeline
-- still attached must refuse deletion rather than orphan them. Derived
-- tables (rather than two scalar subqueries sharing one placeholder) sidestep
-- sqlc's analyzer treating the repeated org_id column name as ambiguous.
SELECT c.cluster_count::int AS cluster_count, p.pipeline_count::int AS pipeline_count
FROM (SELECT count(*) AS cluster_count FROM clusters WHERE clusters.org_id = sqlc.arg('target_org_id')) c,
     (SELECT count(*) AS pipeline_count FROM pipelines WHERE pipelines.org_id = sqlc.arg('target_org_id')) p;

-- name: ListOrgs :many
SELECT * FROM orgs ORDER BY name;

-- name: UpdateOrg :one
-- allow_label_matching/allow_local_attribute_matching use COALESCE against a
-- nullable param: a caller that omits either flag (NULL) leaves the org's
-- current value untouched instead of resetting it to false. This is what
-- makes the two flags safe for a client that doesn't know about them (e.g.
-- an org-edit form written before they existed) to update other org fields
-- without silently disabling fleet-wide matching. See PR-144 review §3.
UPDATE orgs
SET display_name    = $2,
    admin_group_id  = $3,
    reader_group_id = $4,
    editor_group_id = sqlc.narg('editor_group_id'),
    allow_experimental_components = sqlc.arg('allow_experimental_components'),
    allow_label_matching = COALESCE(sqlc.narg('allow_label_matching'), allow_label_matching),
    allow_local_attribute_matching = COALESCE(sqlc.narg('allow_local_attribute_matching'), allow_local_attribute_matching),
    updated_at      = now()
WHERE id = $1
RETURNING *;

-- name: SetOrgTenantID :one
-- Set-once, enforced in SQL rather than only in Go: the WHERE clause updates
-- nothing when tenant_id is already set, so a caller trying to CHANGE an
-- org's tenant identity gets no row back instead of a silent rewrite.
-- Changing it after routes exist would leave every existing HTTPRoute
-- injecting a tenant the org no longer claims — the routes keep working and
-- keep being wrong, which is the worst shape of all.
UPDATE orgs
SET tenant_id  = $2,
    updated_at = now()
WHERE id = $1
  AND tenant_id IS NULL
RETURNING *;

-- name: DeleteOrg :exec
DELETE FROM orgs WHERE id = $1;

-- name: GetOrgByName :one
-- Resolve an org by its unique slug (orgs.name), the external identifier an
-- operator uses when creating an agent-identity binding.
SELECT * FROM orgs WHERE name = $1;

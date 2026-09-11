// Package mgmtapi implements the shepherd.mgmt.v1 Connect services (Me,
// Admin, User, Fleet, Pipeline, Destination, GitOps, Wizard, Visual,
// Simulate, Audit, TenantRoute, Team, ServiceAccount — 14 services, mounted
// by MountRPC in router.go, each behind the shared authz interceptor in
// rpc_interceptor.go), plus a legacy REST shim (built by Router in
// router.go, mounted at /api) that predates the Connect surface and remains
// for external integrations that call plain JSON endpoints.
package mgmtapi

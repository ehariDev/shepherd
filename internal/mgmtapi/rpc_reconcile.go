package mgmtapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"

	mgmtv1 "shepherd/gen/shepherd/mgmt/v1"
	"shepherd/internal/merge"
	"shepherd/internal/reconcile"
	"shepherd/internal/signals"
	"shepherd/internal/store/sqlc"
)

// beaconStaleAfter mirrors internal/agentapi's beaconInventoryExpireAfter (5m,
// unexported there): a beacon_inventory row older than this is due for the
// sweeper's expiry, so reconciliation reports it as stale — still positive
// evidence a component WAS running, not proof it still is.
const beaconStaleAfter = 5 * time.Minute

// managedControllerPrefix is the controller_path prefix a merge-assembled
// pipeline reports under (merge declare-wraps each as `pipe_<sanitized>`).
// Reconciliation compares observed components only within this managed
// namespace: a `pipe_*` path observed but not served is real, actionable drift
// (a disabled/deleted pipeline a collector is still running). Root-level
// components — Alloy's root controller, where the beacon baseline and any BYO
// top-level components run — are not individually distinguishable via
// alloy_component_controller_running_components and are deliberately out of
// scope, which also keeps the baseline from false-flagging (see the plan).
const managedControllerPrefix = "pipe_"

// GetReconciliation returns a collector's declared-vs-served-vs-observed drift
// findings (#110). Org-reader; the per-id ownership check is loadOwnedCollector.
// Computed on demand — no reconciliation state is stored.
func (s *FleetService) GetReconciliation(ctx context.Context, req *connect.Request[mgmtv1.GetReconciliationRequest]) (*connect.Response[mgmtv1.GetReconciliationResponse], error) {
	id, err := s.loadOwnedCollector(ctx, req.Msg.GetOrgId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	orgID, _ := parseUUID(req.Msg.GetOrgId()) // already validated == owner by loadOwnedCollector

	coll, err := s.store.Queries.GetCollectorByID(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("collector not found"))
	}
	cluster, _ := s.store.Queries.GetClusterByID(ctx, coll.ClusterID) //nolint:errcheck // empty cluster name only affects matcher matching, degrades safely

	served, err := s.reconcileServed(ctx, orgID, coll, cluster.Name)
	if err != nil {
		s.logger.Warn("reconcile: building served set failed", "collector_id", id.String(), "err", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to reconcile served pipelines"))
	}
	observed, err := s.reconcileObserved(ctx, id)
	if err != nil {
		s.logger.Warn("reconcile: building observed set failed", "collector_id", id.String(), "err", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to reconcile observed components"))
	}

	findings, err := reconcile.Compare(reconcile.Declared{Role: coll.Role}, served, observed)
	if err != nil {
		// The only error Compare returns is an unrecognized declared role — a
		// data-integrity bug (a collector persisted with a role signals.Policies
		// doesn't know), not a routine drift finding.
		s.logger.Error("reconcile: comparing failed", "collector_id", id.String(), "role", coll.Role, "err", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("collector has an unrecognized role"))
	}
	return connect.NewResponse(&mgmtv1.GetReconciliationResponse{Findings: findingsToProto(findings)}), nil
}

// reconcileServed rebuilds the pipeline set a collector SHOULD be served now —
// the desired served state — using the same merge.Evaluate single decision
// point (match, then derive signals, then enforce role) that Assemble and the
// matcher preview handler use, built from the same BuildCollectorLabels
// org-lookup → admin-labels → local-attrs pattern previewMatchedCollectors
// uses (Phase 1 of docs/plans/2026-09-24-matcher-targeting-unified-plan.md —
// this used to hand-inline a CollectorLabels literal with only role/cluster,
// silently under-counting pipelines matched via an admin label or local
// attribute for any org with either flag on). Neither serve.Result nor
// merge.AssembleResult exposes the included set, hence the replication of
// Assemble's inputs here rather than a call to Assemble itself.
//
// Because this set is enforcement-clean by construction, reconcile's
// declared<->served check (role_signal_mismatch) never fires against it — that
// contradiction only exists for a stale/unenforced serve_cache, which this
// desired-state view deliberately does not read. The actionable signal this
// surface produces is therefore served<->observed drift: a collector running a
// managed pipeline that its desired served set no longer contains (a disabled or
// deleted pipeline it has not yet dropped).
func (s *FleetService) reconcileServed(ctx context.Context, orgID pgtype.UUID, coll sqlc.Collector, cluster string) ([]reconcile.ServedPipeline, error) {
	eps, err := s.store.Queries.ListEnabledPipelinesForMerge(ctx, orgID)
	if err != nil {
		return nil, err
	}
	org, _ := s.store.Queries.GetOrgByID(ctx, orgID) //nolint:errcheck // an org lookup failure degrades to no admin labels/local attrs below
	var localAttrs map[string]string
	if org.AllowLocalAttributeMatching {
		if summary, sumErr := s.store.Queries.GetLatestCollectorInstanceSummary(ctx, coll.ID); sumErr == nil {
			_ = json.Unmarshal(summary.LocalAttributes, &localAttrs) //nolint:errcheck // malformed local_attributes degrades to none
		}
	}
	cl := merge.BuildCollectorLabels(coll.ID.String(), cluster, coll.Role, adminLabelsIfAllowed(org.AllowLabelMatching, coll.Labels), localAttrs)

	var served []reconcile.ServedPipeline
	for i := range eps {
		ep := eps[i]
		var m []string
		if json.Unmarshal(ep.Matchers, &m) != nil {
			continue
		}
		p := merge.Pipeline{
			ID: ep.ID.String(), Name: ep.Name, Contents: ep.Contents,
			Matchers: m, Source: ep.Source,
			RepoLinkCollectorID: repoLinkCollectorID(ep.RepoLinkCollectorID),
		}
		result, evalErr := merge.Evaluate(p, cl, s.schema)
		if evalErr != nil || !result.Served {
			continue
		}
		// Evaluate already derived and enforced signals to reach Served; this
		// second Derive call only recovers the Signals value ServedPipeline
		// carries for reconcile.Compare — it cannot fail here since Evaluate
		// just succeeded against the same content and registry.
		sig, derErr := signals.Derive(p.Contents, s.schema)
		if derErr != nil {
			continue
		}
		served = append(served, reconcile.ServedPipeline{
			Name:           p.Name,
			ControllerPath: managedControllerPrefix + merge.SanitizeName(p.Name),
			Signals:        sig,
		})
	}
	return served, nil
}

// reconcileObserved reads the collector's beacon inventory (attributed via the
// shepherd_collector_id the baseline stamps) into the reconcile Observed set,
// scoped to the managed namespace (see managedControllerPrefix).
func (s *FleetService) reconcileObserved(ctx context.Context, collectorID pgtype.UUID) (reconcile.Observed, error) {
	rows, err := s.store.Queries.ListBeaconInventoryByCollector(ctx, collectorID)
	if err != nil {
		return reconcile.Observed{}, err
	}
	cutoff := time.Now().Add(-beaconStaleAfter)
	var comps []reconcile.ObservedComponent
	for i := range rows {
		r := rows[i]
		if !strings.HasPrefix(r.ComponentName, managedControllerPrefix) {
			continue
		}
		comps = append(comps, reconcile.ObservedComponent{
			ControllerPath: r.ComponentName,
			Healthy:        r.Healthy,
			Stale:          r.LastSeen.Valid && r.LastSeen.Time.Before(cutoff),
		})
	}
	return reconcile.Observed{Components: comps}, nil
}

func findingsToProto(findings []reconcile.Finding) []*mgmtv1.Finding {
	out := make([]*mgmtv1.Finding, 0, len(findings))
	for i := range findings {
		f := findings[i]
		out = append(out, &mgmtv1.Finding{
			Kind:           string(f.Kind),
			Sources:        []string{string(f.Sources[0]), string(f.Sources[1])},
			Summary:        f.Summary,
			PipelineName:   f.PipelineName,
			ControllerPath: f.ControllerPath,
			Stale:          f.Stale,
		})
	}
	return out
}

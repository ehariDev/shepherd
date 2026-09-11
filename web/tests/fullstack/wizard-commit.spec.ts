/**
 * Fullstack: self-monitoring wizard -> real pipeline -> enabled -> served
 * config contains its block (W7-10, via the API).
 *
 * internal/wizard/selfmonitoring: role="singleton" is deliberate (the
 * package doc explains why — it is the one wizard allowed to mix Metrics
 * and Logs), which is exactly why this test can target the seeded
 * "singleton" collector on prod-eu-1 without needing a cluster_pattern.
 */
import { expect, forceRecompute, loginAsAdmin, test } from './fixtures';

test.describe('wizard-commit', () => {
  test('committing the self-monitoring wizard produces a pipeline whose served config carries its block', async ({
    page,
  }) => {
    await loginAsAdmin(page);

    const meResp = await page.request.get('/api/me', {
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    });
    const me = (await meResp.json()) as { orgs: Array<{ id: string; name: string }> };
    const org = me.orgs.find((o) => o.name === 'platform-org');
    if (!org) throw new Error('dev seed must provide platform-org');
    const orgId = org.id;

    const name = `fs-selfmon-${Date.now()}`;
    const commitResp = await page.request.post(`/api/orgs/${orgId}/wizards/commit`, {
      headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
      data: {
        // orgId in the body is required despite the URL param: the REST
        // shim's protojson.Unmarshal(body, req) replaces the whole message
        // (including the OrgId this handler pre-sets from the URL) rather
        // than merging into it, so an orgId-less body reaches CommitWizard
        // with an empty OrgId and 500s on the pipelines.org_id NOT NULL
        // constraint.
        orgId,
        kind: 'self-monitoring',
        name,
        state: {
          job_name: 'alloy-self',
          scrape_interval: '60s',
          metrics_dest_name: 'prom-prod',
          // Logs deliberately left off (logs_dest_name/log_path empty):
          // Commit() only emits the loki.* blocks when both are set, and
          // this test only needs the pipeline to exist and match, not the
          // mixed-signal detail the wizard package itself already covers.
          logs_enabled: false,
        },
      },
    });
    expect(commitResp.status()).toBe(201);
    const pipeline = (await commitResp.json()) as { id: string; matchers: string[] };
    expect(pipeline.matchers).toContain('role="singleton"');

    const enableResp = await page.request.post(
      `/api/orgs/${orgId}/pipelines/${pipeline.id}/enable`,
      { headers: { 'X-Requested-With': 'XMLHttpRequest' } },
    );
    expect(enableResp.status()).toBe(200);

    const collectorsResp = await page.request.get(`/api/orgs/${orgId}/collectors`, {
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    });
    const collectors = (await collectorsResp.json()) as {
      items: Array<{ id: string; role: string; cluster: string }>;
    };
    const singletonCollector = collectors.items.find(
      (c) => c.role === 'singleton' && c.cluster === 'prod-eu-1',
    );
    if (!singletonCollector) {
      throw new Error('dev seed must provide the prod-eu-1/singleton collector');
    }

    // Nothing real polls the singleton collector (dev-guide: it has no
    // compose container backing it), so a lazy recompute needs a nudge here
    // — same ruling pipelines.spec.ts's scenario 6 already relies on.
    await forceRecompute(page, 'prod-eu-1', 'singleton');

    const blockName = `pipe_${name.replace(/-/g, '_')}`;
    await expect
      .poll(
        async () => {
          const resp = await page.request.get(
            `/api/orgs/${orgId}/collectors/${singletonCollector.id}/served-config`,
            { headers: { 'X-Requested-With': 'XMLHttpRequest' } },
          );
          const data = (await resp.json()) as { content: string };
          return data.content;
        },
        { timeout: 15000, intervals: [1000] },
      )
      .toContain(`declare "${blockName}"`);

    await page.request.delete(`/api/orgs/${orgId}/pipelines/${pipeline.id}`, {
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    });
  });
});

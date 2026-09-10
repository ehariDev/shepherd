import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { clients } from '@/api/transport';
import { QueryError } from '@/components/QueryError';
import { DataTable, type DataTableColumn } from '@/components/ui/DataTable';
import type { Collector } from '@/gen/shepherd/mgmt/v1/fleet_pb';
import { useOrgId } from '@/hooks/useOrg';
import { formatTimestampRelative } from '@/lib/utils';

const STATUS_COLORS: Record<string, string> = {
  APPLIED: 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20',
  APPLYING: 'text-yellow-400 bg-yellow-400/10 border-yellow-400/20',
  FAILED: 'text-red-400 bg-red-400/10 border-red-400/20',
};

const collectorColumns: DataTableColumn<Collector>[] = [
  {
    key: 'cluster',
    header: 'Cluster',
    render: (c) => (
      <Link to='/collectors/$id' params={{ id: c.id }}>
        {c.cluster}
      </Link>
    ),
  },
  { key: 'role', header: 'Role', cellClassName: 'px-4 py-2.5 text-muted', render: (c) => c.role },
  {
    key: 'status',
    header: 'Status',
    render: (c) => {
      const status = c.remoteConfigStatus?.toUpperCase() ?? '';
      const statusColor = STATUS_COLORS[status] ?? 'text-muted bg-border border-border-strong';
      return (
        <span className={`text-xs font-medium px-2 py-0.5 rounded border ${statusColor}`}>
          {status || 'UNKNOWN'}
        </span>
      );
    },
  },
  {
    key: 'lastSeen',
    header: 'Last Seen',
    cellClassName: 'px-4 py-2.5 text-muted',
    render: (c) => formatTimestampRelative(c.lastSeen),
  },
  {
    key: 'version',
    header: 'Version',
    cellClassName: 'px-4 py-2.5 text-muted',
    render: (c) => c.alloyVersion || '—',
  },
];

export function CollectorsPage() {
  const orgId = useOrgId();
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['collectors', orgId],
    queryFn: () => clients.fleet.listCollectors({ orgId }),
    enabled: !!orgId,
  });

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between'>
        <h1 className='text-xl font-semibold'>Collectors</h1>
      </div>
      {isError ? (
        <QueryError error={error} noun='collectors' />
      ) : isLoading ? (
        <p className='text-sm text-muted'>Loading…</p>
      ) : (
        <DataTable
          columns={collectorColumns}
          rows={data?.items ?? []}
          rowKey={(c) => c.id}
          rowClassName='border-t border-border hover:bg-card/60 cursor-pointer'
        />
      )}
    </div>
  );
}

import type { ReactNode } from 'react';

/**
 * A titled, bordered group of fields (S9c). AdminAuthPage.tsx defined this
 * locally before it had anywhere shared to live; lifted here so any settings
 * page that groups fields the same way can reuse it.
 */
export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className='space-y-3 rounded-lg border border-border bg-card/40 p-4'>
      <h2 className='text-sm font-semibold text-zinc-200'>{title}</h2>
      {children}
    </section>
  );
}

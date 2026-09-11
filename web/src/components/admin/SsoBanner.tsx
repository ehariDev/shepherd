import { AlertTriangle, CheckCircle2, Info } from 'lucide-react';
import type { ReactNode } from 'react';
import { Banner } from '@/components/ui/Banner';

export type SsoBannerTone = 'ok' | 'info' | 'warn' | 'error';

const TONE_TO_VARIANT = {
  ok: 'success',
  info: 'info',
  warn: 'warning',
  error: 'error',
} as const;

/** SSO settings page notices (S9c). Reconciles the page's former local
 * Banner (four tones, an icon per tone) onto the shared ui/Banner primitive
 * -- ui/Banner owns the box styling and role='alert' behaviour, this adds
 * the icon and the tone-name mapping the rest of the page still speaks. */
export function SsoBanner({
  tone,
  testId,
  children,
}: {
  tone: SsoBannerTone;
  testId: string;
  children: ReactNode;
}) {
  const Icon = tone === 'ok' ? CheckCircle2 : tone === 'error' ? AlertTriangle : Info;
  return (
    <Banner variant={TONE_TO_VARIANT[tone]} testId={testId} className='flex items-start gap-2'>
      <Icon size={15} className='mt-0.5 shrink-0' />
      <span>{children}</span>
    </Banner>
  );
}

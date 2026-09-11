import type { ReactNode } from 'react';

/**
 * Shared inline notice (S4). A handful of pages hand-rolled the same
 * rounded/border/padding notice box for a fact the viewer needs before
 * acting (a group-backed team's membership note, a forbidden-action
 * explanation). `variant='error'` gets role='alert' -- the others are
 * informational, not something a screen reader should interrupt for.
 */
type BannerVariant = 'info' | 'warning' | 'error' | 'success';

const VARIANT_CLASSES: Record<BannerVariant, string> = {
  info: 'border-sky-500/30 bg-sky-500/10 text-sky-200',
  warning: 'border-yellow-500/30 bg-yellow-500/10 text-yellow-300',
  error: 'border-red-500/30 bg-red-500/10 text-red-400',
  success: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300',
};

export function Banner({
  variant = 'info',
  testId,
  className = '',
  children,
}: {
  variant?: BannerVariant;
  testId?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      role={variant === 'error' ? 'alert' : undefined}
      data-testid={testId}
      className={`rounded-md border p-2.5 text-xs ${VARIANT_CLASSES[variant]} ${className}`.trim()}
    >
      {children}
    </div>
  );
}

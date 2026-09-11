import type { SsoFormState } from '@/components/admin/ssoForm';
import { Field, Input } from '@/components/ui/Field';
import { Section } from '@/components/ui/Section';

/** Admin → Single sign-on → "Claim mapping" section (S9c). */
export function SsoClaimsSection({
  form,
  onChange,
  disabled,
}: {
  form: SsoFormState;
  onChange: (updater: (f: SsoFormState) => SsoFormState) => void;
  disabled: boolean;
}) {
  return (
    <Section title='Claim mapping'>
      <div className='grid gap-3 sm:grid-cols-3'>
        <Field label='Subject claim'>
          <Input
            data-testid='sso-subject-claim'
            disabled={disabled}
            value={form.subjectClaim}
            onChange={(e) => onChange((f) => ({ ...f, subjectClaim: e.target.value }))}
          />
        </Field>
        <Field label='Email claim'>
          <Input
            data-testid='sso-email-claim'
            disabled={disabled}
            value={form.emailClaim}
            onChange={(e) => onChange((f) => ({ ...f, emailClaim: e.target.value }))}
          />
        </Field>
        <Field label='Name claim'>
          <Input
            data-testid='sso-name-claim'
            disabled={disabled}
            value={form.nameClaim}
            onChange={(e) => onChange((f) => ({ ...f, nameClaim: e.target.value }))}
          />
        </Field>
      </div>
    </Section>
  );
}

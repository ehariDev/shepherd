/**
 * @vitest-environment jsdom
 */
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { Banner } from './Banner';
import { Field, Input } from './Field';

afterEach(cleanup);

describe('Field', () => {
  it('associates its label with the wrapped control (getByLabelText works)', () => {
    render(
      <Field label='Name'>
        <Input value='prom-prod' onChange={() => undefined} />
      </Field>,
    );
    expect(screen.getByLabelText('Name')).toHaveProperty('value', 'prom-prod');
  });

  it('renders hint and error text', () => {
    render(
      <Field label='URL' hint='http(s) only' error='Enter a valid URL'>
        <Input value='' onChange={() => undefined} />
      </Field>,
    );
    expect(screen.getByText('http(s) only')).not.toBeNull();
    expect(screen.getByText('Enter a valid URL')).not.toBeNull();
  });
});

describe('Banner', () => {
  it('is role=alert only for the error variant', () => {
    render(<Banner variant='error'>failed</Banner>);
    expect(screen.getByRole('alert').textContent).toBe('failed');
  });

  it('carries a testId when given', () => {
    render(
      <Banner variant='info' testId='team-members-group-note'>
        note
      </Banner>,
    );
    expect(screen.getByTestId('team-members-group-note')).not.toBeNull();
  });
});

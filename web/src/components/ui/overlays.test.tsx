/**
 * @vitest-environment jsdom
 */
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Modal, ModalActions } from './Modal';

// vitest.config.ts does not set `test.globals: true`, so @testing-library/react's
// automatic afterEach cleanup (which detects a global `afterEach`) never fires —
// without this every test after the first would find more than one dialog.
afterEach(cleanup);

// S2: components/ui/Modal is the one overlay implementation. It must carry
// forward every accessibility guarantee AdminModal.tsx had (role=dialog,
// aria-modal, Escape-to-close, focus trap) plus the new size/testId knobs
// the migrated pages need.
describe('Modal', () => {
  it('renders a labelled dialog', () => {
    render(
      <Modal title='New destination' onClose={() => undefined}>
        body
      </Modal>,
    );
    expect(screen.getByRole('dialog', { name: 'New destination' })).not.toBeNull();
  });

  it('is aria-modal', () => {
    render(
      <Modal title='X' onClose={() => undefined}>
        body
      </Modal>,
    );
    expect(screen.getByRole('dialog').getAttribute('aria-modal')).toBe('true');
  });

  it('calls onClose when Escape is pressed', () => {
    const onClose = vi.fn();
    render(
      <Modal title='X' onClose={onClose}>
        body
      </Modal>,
    );
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('applies a data-testid to the dialog when given', () => {
    render(
      <Modal title='X' onClose={() => undefined} testId='destination-modal'>
        body
      </Modal>,
    );
    expect(screen.getByTestId('destination-modal')).not.toBeNull();
  });

  it('sizes the panel from the size prop', () => {
    render(
      <Modal title='X' onClose={() => undefined} size='xl'>
        body
      </Modal>,
    );
    expect(screen.getByRole('dialog').className).toMatch(/max-w-xl/);
  });
});

describe('ModalActions', () => {
  it('tags the submit button with submitTestId', () => {
    render(
      <ModalActions
        onCancel={() => undefined}
        submitLabel='Save'
        pendingLabel='Saving…'
        pending={false}
        submitTestId='save-destination'
      />,
    );
    expect(screen.getByTestId('save-destination').textContent).toBe('Save');
  });
});

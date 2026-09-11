// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AdminConfirmDialog } from './AdminConfirmDialog';

describe('AdminConfirmDialog', () => {
  it('renders the title and body inside the shared modal shell', () => {
    render(
      <AdminConfirmDialog
        title='Delete organisation'
        body='This cannot be undone.'
        confirmLabel='Delete'
        pendingLabel='Deleting…'
        pending={false}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const dialog = screen.getByRole('dialog');
    expect(dialog.textContent).toContain('Delete organisation');
    expect(screen.getByText('This cannot be undone.')).toBeTruthy();
  });

  it('calls onConfirm (not onCancel) when the form is submitted', () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(
      <AdminConfirmDialog
        title='Revoke token'
        body='The token stops working immediately.'
        confirmLabel='Revoke'
        pendingLabel='Revoking…'
        pending={false}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('calls onCancel, not onConfirm, when Cancel is clicked', () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(
      <AdminConfirmDialog
        title='Revoke token'
        body='The token stops working immediately.'
        confirmLabel='Revoke'
        pendingLabel='Revoking…'
        pending={false}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('also calls onCancel (via AdminModal onClose) when Escape is pressed', () => {
    // AdminConfirmDialog wires AdminModal's onClose to its own onCancel, so a
    // confirmation dialog dismisses like every other admin modal.
    const onCancel = vi.fn();
    render(
      <AdminConfirmDialog
        title='Revoke token'
        body='body'
        confirmLabel='Revoke'
        pendingLabel='Revoking…'
        pending={false}
        onConfirm={vi.fn()}
        onCancel={onCancel}
      />,
    );
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('disables the confirm button and shows pendingLabel while pending', () => {
    render(
      <AdminConfirmDialog
        title='Revoke token'
        body='body'
        confirmLabel='Revoke'
        pendingLabel='Revoking…'
        pending={true}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const submit = screen.getByRole('button', { name: 'Revoking…' }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(screen.queryByRole('button', { name: 'Revoke' })).toBeNull();
  });
});

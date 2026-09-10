// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AdminModal, AdminModalActions } from './AdminModal';

describe('AdminModal', () => {
  it('renders an accessible dialog whose aria-labelledby resolves to the title heading', () => {
    render(
      <AdminModal title='Delete organisation' onClose={vi.fn()}>
        <p>body</p>
      </AdminModal>,
    );
    const dialog = screen.getByRole('dialog');
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    const labelledBy = dialog.getAttribute('aria-labelledby');
    expect(labelledBy).toBeTruthy();
    const heading = document.getElementById(labelledBy as string);
    expect(heading?.tagName).toBe('H2');
    expect(heading?.textContent).toBe('Delete organisation');
  });

  it('moves focus onto the dialog panel on mount', () => {
    render(
      <AdminModal title='Focus me' onClose={vi.fn()}>
        <input placeholder='name' />
      </AdminModal>,
    );
    expect(document.activeElement).toBe(screen.getByRole('dialog'));
  });

  it('calls onClose exactly once when Escape is pressed', () => {
    const onClose = vi.fn();
    render(
      <AdminModal title='Escapable' onClose={onClose}>
        <p>body</p>
      </AdminModal>,
    );
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('calls onClose when the close (x) button is clicked', () => {
    const onClose = vi.fn();
    render(
      <AdminModal title='Closeable' onClose={onClose}>
        <p>body</p>
      </AdminModal>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('traps Tab: from the last focusable element it wraps to the true first (the Close button)', () => {
    // The Close (x) button renders before `children` in the DOM, so it is
    // itself the panel's first focusable element -- the trap must wrap to it,
    // not to the first child the caller passed in.
    render(
      <AdminModal title='Trap' onClose={vi.fn()}>
        <button type='button'>only child</button>
      </AdminModal>,
    );
    const closeButton = screen.getByRole('button', { name: 'Close' });
    const child = screen.getByText('only child');
    child.focus();
    expect(document.activeElement).toBe(child);
    fireEvent.keyDown(document, { key: 'Tab', shiftKey: false });
    expect(document.activeElement).toBe(closeButton);
  });

  it('traps Shift+Tab: from the true first (the Close button) it wraps to the last focusable element', () => {
    render(
      <AdminModal title='Trap' onClose={vi.fn()}>
        <button type='button'>only child</button>
      </AdminModal>,
    );
    const closeButton = screen.getByRole('button', { name: 'Close' });
    const child = screen.getByText('only child');
    closeButton.focus();
    expect(document.activeElement).toBe(closeButton);
    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(document.activeElement).toBe(child);
  });

  it('returns focus to the previously-focused element on unmount', () => {
    const trigger = document.createElement('button');
    document.body.appendChild(trigger);
    trigger.focus();
    expect(document.activeElement).toBe(trigger);

    const { unmount } = render(
      <AdminModal title='Returns focus' onClose={vi.fn()}>
        <p>body</p>
      </AdminModal>,
    );
    expect(document.activeElement).not.toBe(trigger);

    unmount();
    expect(document.activeElement).toBe(trigger);
    trigger.remove();
  });
});

describe('AdminModalActions', () => {
  it('calls onCancel when Cancel is clicked', () => {
    const onCancel = vi.fn();
    render(
      <AdminModalActions
        onCancel={onCancel}
        submitLabel='Save'
        pendingLabel='Saving…'
        pending={false}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('shows submitLabel and an enabled submit button when not pending', () => {
    render(
      <AdminModalActions
        onCancel={vi.fn()}
        submitLabel='Save'
        pendingLabel='Saving…'
        pending={false}
      />,
    );
    const submit = screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement;
    expect(submit.disabled).toBe(false);
  });

  it('shows pendingLabel and disables the submit button while pending', () => {
    render(
      <AdminModalActions
        onCancel={vi.fn()}
        submitLabel='Save'
        pendingLabel='Saving…'
        pending={true}
      />,
    );
    const submit = screen.getByRole('button', { name: 'Saving…' }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
  });
});

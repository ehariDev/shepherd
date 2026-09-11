// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { WizardStepper, type WizardStepperItem } from './WizardStepper';

const steps: WizardStepperItem[] = [
  { id: 'a', title: 'Step A' },
  { id: 'b', title: 'Step B' },
  { id: 'c', title: 'Step C' },
];

describe('WizardStepper', () => {
  it('marks steps before activeIndex done (a Check icon, not the step number) and the active step active', () => {
    render(<WizardStepper steps={steps} activeIndex={1} />);

    const done = screen.getByTestId('wizard-step-indicator-0');
    expect(done.dataset.state).toBe('done');
    expect(done.querySelector('svg')).not.toBeNull();
    expect(done.textContent).not.toBe('1');

    const active = screen.getByTestId('wizard-step-indicator-1');
    expect(active.dataset.state).toBe('active');
    expect(active.textContent).toBe('2');
    expect(active.querySelector('svg')).toBeNull();

    const upcoming = screen.getByTestId('wizard-step-indicator-2');
    expect(upcoming.dataset.state).toBe('upcoming');
    expect(upcoming.textContent).toBe('3');
  });

  it('marks no step done when activeIndex is 0', () => {
    render(<WizardStepper steps={steps} activeIndex={0} />);
    for (let i = 0; i < steps.length; i++) {
      expect(screen.getByTestId(`wizard-step-indicator-${i}`).dataset.state).not.toBe('done');
    }
    expect(screen.getByTestId('wizard-step-indicator-0').dataset.state).toBe('active');
  });

  it('marks every step done when activeIndex is past the last step', () => {
    render(<WizardStepper steps={steps} activeIndex={steps.length} />);
    for (let i = 0; i < steps.length; i++) {
      const indicator = screen.getByTestId(`wizard-step-indicator-${i}`);
      expect(indicator.dataset.state).toBe('done');
      expect(indicator.querySelector('svg')).not.toBeNull();
    }
  });

  it('renders each step title as text alongside its indicator', () => {
    render(<WizardStepper steps={steps} activeIndex={0} />);
    expect(screen.getByText('Step A')).toBeTruthy();
    expect(screen.getByText('Step B')).toBeTruthy();
    expect(screen.getByText('Step C')).toBeTruthy();
  });
});

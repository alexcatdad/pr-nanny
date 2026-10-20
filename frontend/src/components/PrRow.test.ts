import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
afterEach(cleanup);
import PrRow from './PrRow.svelte';
import type { PullRequest } from '../lib/types';

const pr: PullRequest = {
  id: 'fixture-1', number: 42, title: '<img src=x onerror=alert(1)> fixture title', repository: 'sample/private-repo',
  url: 'https://github.com/sample/private-repo/pull/42', createdAt: '2026-10-20T02:00:00Z', updatedAt: '2026-10-20T03:00:00Z',
  merged: false, approved: false, ci: 'not_available', authored: true, reviewRequested: false, bell: false,
};

describe('PR row', () => {
  it('renders API text as text and labels unknown CI as not available', () => {
    const onBell = vi.fn();
    const onOpen = vi.fn();
    const { container } = render(PrRow, { pr, onBell, onOpen });
    expect(screen.getByText(pr.title)).toBeVisible();
    expect(container.querySelector('img, script')).toBeNull();
    expect(screen.getByText('Not available')).toBeVisible();
    expect(screen.getByText('Not approved')).toBeVisible();
  });
  it('opens from the row and keeps the bell action separate', async () => {
    const onBell = vi.fn();
    const onOpen = vi.fn();
    render(PrRow, { pr, onBell, onOpen });
    await fireEvent.click(screen.getByRole('button', { name: /pull request #42/ }));
    expect(onOpen).toHaveBeenCalledWith('fixture-1');
    onOpen.mockClear();
    await fireEvent.click(screen.getByRole('button', { name: 'Turn notifications on' }));
    expect(onBell).toHaveBeenCalledWith('fixture-1', true);
    expect(onOpen).not.toHaveBeenCalled();
  });
});

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App.svelte';
import type { Settings, Snapshot } from './lib/types';

afterEach(() => {
  cleanup();
  delete window.__PR_NANNY_TEST_BRIDGE__;
  history.replaceState({}, '', '/');
});

const defaultSettings: Settings = {
  version: 1,
  authMethod: 'gh',
  account: 'octocat',
  intervalSeconds: 60,
  layout: 'all',
  notifications: true,
  events: { ci: true, approval: true, reviewRequested: true, merged: true, closed: true },
  launchAtLogin: true,
};

function makeSnapshot(): Snapshot {
  const now = Date.now();
  return {
    settings: structuredClone(defaultSettings),
    pullRequests: [
      {
        id: 'authored',
        number: 21,
        title: 'An authored change',
        repository: 'sample/app',
        url: 'https://github.com/sample/app/pull/21',
        createdAt: new Date(now - 60_000).toISOString(),
        updatedAt: new Date(now - 10_000).toISOString(),
        merged: false,
        approved: true,
        ci: 'passed',
        authored: true,
        reviewRequested: false,
        bell: false,
      },
      {
        id: 'review',
        number: 22,
        title: 'A review request',
        repository: 'sample/lib',
        url: 'https://github.com/sample/lib/pull/22',
        createdAt: new Date(now - 120_000).toISOString(),
        updatedAt: new Date(now - 20_000).toISOString(),
        merged: false,
        approved: false,
        ci: 'running',
        authored: false,
        reviewRequested: true,
        bell: false,
      },
      {
        id: 'closed',
        number: 23,
        title: 'A recent closure',
        repository: 'sample/docs',
        url: 'https://github.com/sample/docs/pull/23',
        createdAt: new Date(now - 180_000).toISOString(),
        updatedAt: new Date(now - 30_000).toISOString(),
        closedAt: new Date(now - 15_000).toISOString(),
        merged: true,
        approved: true,
        ci: 'passed',
        authored: false,
        reviewRequested: false,
        bell: false,
      },
    ],
    connection: 'connected',
    message: '',
    lastRefresh: new Date(now - 60_000).toISOString(),
    refreshing: false,
  };
}

function installBridge() {
  const state = makeSnapshot();
  const bridge = {
    GetSnapshot: vi.fn(async () => structuredClone(state)),
    SaveSettings: vi.fn(async (settings: Settings) => {
      state.settings = structuredClone(settings);
      return structuredClone(state);
    }),
    ConnectPAT: vi.fn(async () => structuredClone(state)),
    ConnectCLI: vi.fn(async () => structuredClone(state)),
    SetBell: vi.fn(async (id: string, enabled: boolean) => {
      state.pullRequests = state.pullRequests.map((pr) => (pr.id === id ? { ...pr, bell: enabled } : pr));
      return structuredClone(state);
    }),
    SetLayout: vi.fn(async (layout: string) => {
      state.settings.layout = layout;
      return structuredClone(state);
    }),
    Refresh: vi.fn(async () => undefined),
    OpenPR: vi.fn(async () => undefined),
    HidePopup: vi.fn(async () => undefined),
  };
  window.__PR_NANNY_TEST_BRIDGE__ = bridge;
  return bridge;
}

describe('popup application', () => {
  beforeEach(() => {
    history.replaceState({}, '', '/');
  });

  it('loads connected pull requests and switches between category and all layouts', async () => {
    const service = installBridge();
    render(App);

    expect(await screen.findByText('Connected as octocat')).toBeVisible();
    expect(screen.getByText('An authored change')).toBeVisible();
    expect(screen.getByText('A review request')).toBeVisible();
    expect(screen.getByText('A recent closure')).toBeVisible();

    await fireEvent.click(screen.getByRole('button', { name: 'Tabs' }));
    expect(service.SetLayout).toHaveBeenCalledWith('tabs');
    await waitFor(() => expect(screen.getByRole('navigation', { name: 'Pull request categories' })).toBeVisible());
    await fireEvent.click(screen.getByRole('button', { name: /Review/ }));
    expect(screen.getByText('A review request')).toBeVisible();
    expect(screen.queryByText('An authored change')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Show all' }));
    expect(service.SetLayout).toHaveBeenCalledWith('all');
  });

  it('keeps bell and open actions independent and refreshes through the bridge', async () => {
    const service = installBridge();
    render(App);
    await screen.findByText('Connected as octocat');

    await fireEvent.click(screen.getAllByRole('button', { name: 'Turn notifications on' })[0]);
    expect(service.SetBell).toHaveBeenCalledWith('authored', true);
    await fireEvent.click(screen.getByRole('button', { name: /pull request #21/ }));
    expect(service.OpenPR).toHaveBeenCalledWith('authored');
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh pull requests' }));
    expect(service.Refresh).toHaveBeenCalledOnce();
  });

  it('opens settings from the query, saves changed preferences, and shows build version', async () => {
    history.replaceState({}, '', '/?settings=1');
    const service = installBridge();
    render(App);

    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeVisible();
    expect(screen.getByText(`PR Nanny ${__APP_VERSION__}`)).toBeVisible();
    const notifications = document.querySelector('.toggle-row input') as HTMLInputElement;
    await fireEvent.click(notifications);
    await waitFor(() => expect(service.SaveSettings).toHaveBeenCalledOnce());
    expect(service.SaveSettings.mock.calls[0][0].notifications).toBe(false);
    await fireEvent.click(screen.getByRole('button', { name: 'Back to pull requests' }));
    expect(screen.getByText('An authored change')).toBeVisible();
  });

  it('renders an unavailable state after a failed initial bridge read and retries', async () => {
    const service = installBridge();
    service.GetSnapshot.mockRejectedValueOnce(new Error('synthetic bridge failure'));
    render(App);

    expect(await screen.findByRole('heading', { name: 'PR Nanny is unavailable' })).toBeVisible();
    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Connected as octocat')).toBeVisible();
    expect(service.GetSnapshot).toHaveBeenCalledTimes(2);
  });

  it('shows offline status with cached pull requests and hides on Escape', async () => {
    const service = installBridge();
    service.GetSnapshot.mockImplementation(async () => ({ ...makeSnapshot(), connection: 'offline' }));
    render(App);

    expect(
      await screen.findByText('You are offline. Showing the pull requests saved from your last refresh.'),
    ).toBeVisible();
    expect(screen.getByText('An authored change')).toBeVisible();
    await fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(service.HidePopup).toHaveBeenCalledOnce());
  });
});

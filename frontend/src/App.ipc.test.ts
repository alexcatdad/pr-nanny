import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App.svelte';
import type { Settings, Snapshot } from './lib/types';

const settings: Settings = {
  version: 1,
  authMethod: 'gh',
  account: 'octocat',
  intervalSeconds: 60,
  layout: 'all',
  notifications: true,
  events: { ci: true, approval: true, reviewRequested: true, merged: true, closed: true },
  launchAtLogin: true,
};
const T0 = Date.parse('2026-01-01T00:10:00Z');
const SYNC = 5000;

function snap(over: Partial<Snapshot> = {}, bell = false): Snapshot {
  return {
    settings: structuredClone(settings),
    pullRequests: [
      {
        id: 'a',
        number: 1,
        title: 'Counting change',
        repository: 'sample/app',
        url: 'https://github.com/sample/app/pull/1',
        createdAt: new Date(T0 - 60_000).toISOString(),
        updatedAt: new Date(T0 - 10_000).toISOString(),
        merged: false,
        approved: true,
        ci: 'passed',
        authored: true,
        reviewRequested: false,
        bell,
      },
    ],
    connection: 'connected',
    message: '',
    lastRefresh: new Date(T0 - 58_000).toISOString(),
    refreshing: false,
    ...over,
  };
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state });
  document.dispatchEvent(new Event('visibilitychange'));
}

function installBridge(initial: Snapshot = snap()) {
  const state = { current: initial };
  const bridge = {
    GetSnapshot: vi.fn(async () => structuredClone(state.current)),
    SaveSettings: vi.fn(async () => structuredClone(state.current)),
    ConnectPAT: vi.fn(async () => structuredClone(state.current)),
    ConnectCLI: vi.fn(async () => structuredClone(state.current)),
    SetBell: vi.fn(async (_id: string, enabled: boolean) => {
      state.current = snap({}, enabled);
      return structuredClone(state.current);
    }),
    SetLayout: vi.fn(async (layout: string) => {
      state.current = { ...state.current, settings: { ...state.current.settings, layout } };
      return structuredClone(state.current);
    }),
    Refresh: vi.fn(async () => undefined),
    OpenPR: vi.fn(async () => undefined),
    HidePopup: vi.fn(async () => undefined),
  };
  window.__PR_NANNY_TEST_BRIDGE__ = bridge;
  return { bridge, state };
}

const advance = (ms: number) => vi.advanceTimersByTimeAsync(ms);
async function mount() {
  render(App);
  await advance(0);
}
function holdNextSnapshot(bridge: ReturnType<typeof installBridge>['bridge']) {
  const held = { release: (_value: Snapshot) => {} };
  bridge.GetSnapshot.mockImplementationOnce(() => new Promise<Snapshot>((resolve) => (held.release = resolve)));
  return held;
}

describe('snapshot sync cadence', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(T0);
    setVisibility('visible');
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    delete window.__PR_NANNY_TEST_BRIDGE__;
    setVisibility('visible');
  });

  it('measures one hour of bridge traffic while visible and while hidden', async () => {
    const visibleRun = installBridge();
    await mount();
    await advance(3_600_000);
    const visibleCalls = visibleRun.bridge.GetSnapshot.mock.calls.length;
    cleanup();

    setVisibility('hidden');
    const hiddenRun = installBridge();
    await mount();
    await advance(3_600_000);
    const hiddenCalls = hiddenRun.bridge.GetSnapshot.mock.calls.length;
    console.info(`IPC GetSnapshot calls per simulated hour: visible=${visibleCalls} hidden=${hiddenCalls}`);
    expect(visibleCalls).toBe(1 + 3600 / (SYNC / 1000));
    expect(hiddenCalls).toBe(1);
  });

  it('advances relative time from the local clock with zero extra backend calls', async () => {
    const { bridge } = installBridge();
    await mount();
    expect(screen.getByText('Updated just now')).toBeVisible();
    await advance(3000);
    expect(screen.getByText('Updated 1m ago')).toBeVisible();
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(1);
  });

  it('shows new backend state within one poll interval while visible', async () => {
    const { bridge, state } = installBridge();
    await mount();
    expect(screen.queryByText('Offline · showing saved pull requests')).toBeNull();
    state.current = snap({ connection: 'offline' });
    await advance(SYNC);
    expect(screen.getByText('Offline · showing saved pull requests')).toBeVisible();
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(2);
  });

  it('stops polling and the clock while hidden, then refreshes immediately on becoming visible', async () => {
    const { bridge, state } = installBridge();
    await mount();
    setVisibility('hidden');
    state.current = snap({ connection: 'offline' });
    await advance(10 * 60_000);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
    setVisibility('visible');
    await advance(0);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(2);
    expect(screen.getByText('Offline · showing saved pull requests')).toBeVisible();
    expect(screen.getByText(/Updated 10m ago/)).toBeVisible();
  });

  it('refreshes when the window regains focus (popup reopened)', async () => {
    const { bridge } = installBridge();
    await mount();
    window.dispatchEvent(new Event('focus'));
    await advance(0);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(2);
  });

  it('does not stack polls while a read is still in flight', async () => {
    const { bridge } = installBridge();
    await mount();
    const held = holdNextSnapshot(bridge);
    await advance(SYNC * 4);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(2);
    held.release(snap());
    await advance(SYNC);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(3);
  });

  it('keeps the same rendered row when a poll returns identical data', async () => {
    const { bridge } = installBridge();
    await mount();
    const row = screen.getByText('Counting change');
    await advance(SYNC * 2);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(3);
    expect(screen.getByText('Counting change')).toBe(row);
  });

  it('ignores a stale poll that resolves after a newer layout change', async () => {
    const { bridge } = installBridge();
    await mount();
    const held = holdNextSnapshot(bridge);
    await advance(SYNC);
    await fireEvent.click(screen.getByRole('button', { name: 'Tabs' }));
    await advance(0);
    expect(screen.getByRole('button', { name: 'Tabs' })).toHaveAttribute('aria-pressed', 'true');
    held.release(snap());
    await advance(0);
    expect(screen.getByRole('button', { name: 'Tabs' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('ignores a stale poll that resolves after a newer bell change', async () => {
    const { bridge } = installBridge();
    await mount();
    const held = holdNextSnapshot(bridge);
    await advance(SYNC);
    await fireEvent.click(screen.getByRole('button', { name: 'Turn notifications on' }));
    await advance(0);
    expect(screen.getByRole('button', { name: 'Turn notifications off' })).toBeVisible();
    held.release(snap({}, false));
    await advance(0);
    expect(screen.getByRole('button', { name: 'Turn notifications off' })).toBeVisible();
  });

  it('clears timers and listeners on destroy', async () => {
    const { bridge } = installBridge();
    const removeDoc = vi.spyOn(document, 'removeEventListener');
    const removeWindow = vi.spyOn(window, 'removeEventListener');
    await mount();
    expect(vi.getTimerCount()).toBe(2);
    cleanup();
    expect(vi.getTimerCount()).toBe(0);
    expect(removeDoc).toHaveBeenCalledWith('visibilitychange', expect.any(Function));
    expect(removeWindow).toHaveBeenCalledWith('focus', expect.any(Function));
    setVisibility('hidden');
    setVisibility('visible');
    window.dispatchEvent(new Event('focus'));
    await advance(SYNC * 3);
    expect(bridge.GetSnapshot).toHaveBeenCalledTimes(1);
  });
});

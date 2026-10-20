import raw from '../../tests/fixtures/pull-requests.json';
import type { PullRequest, Settings, Snapshot } from './types';

const initial: Settings = {
  version: 1,
  authMethod: 'gh',
  account: 'octocat',
  intervalSeconds: 60,
  layout: 'all',
  notifications: true,
  events: { ci: true, approval: true, reviewRequested: true, merged: true, closed: true },
  launchAtLogin: true,
};
const params = new URLSearchParams(location.search);
const fixtureConnection = params.get('fixtureConnection') ?? 'connected';
const failPAT = params.has('fixturePat') && params.get('fixturePat') === 'fail';
const failSave = params.has('fixtureSave') && params.get('fixtureSave') === 'fail';
const stalePoll = params.has('fixtureStalePoll');
let snapshotCount = 0;
const state: Snapshot = {
  settings: structuredClone(initial),
  pullRequests: (raw as PullRequest[]).map((pr) =>
    pr.id === 'pr-104'
      ? {
          ...pr,
          closedAt:
            params.get('fixtureClosedAt') === 'expired'
              ? new Date(Date.now() - 61_000).toISOString()
              : new Date(Date.now() - 30_000).toISOString(),
        }
      : pr,
  ),
  connection: fixtureConnection,
  message: '',
  lastRefresh: new Date().toISOString(),
  refreshing: false,
};
export const mockBridge = {
  GetSnapshot: async () => {
    snapshotCount++;
    const captured = structuredClone(state);
    if (stalePoll && snapshotCount === 2) {
      captured.settings.layout = 'all';
      const testWindow = window as Window & {
        __PR_NANNY_STALE_POLL_PENDING__?: boolean;
        __PR_NANNY_RELEASE_STALE_POLL__?: () => void;
      };
      testWindow.__PR_NANNY_STALE_POLL_PENDING__ = true;
      await new Promise<void>((resolve) => {
        testWindow.__PR_NANNY_RELEASE_STALE_POLL__ = () => {
          testWindow.__PR_NANNY_STALE_POLL_PENDING__ = false;
          delete testWindow.__PR_NANNY_RELEASE_STALE_POLL__;
          resolve();
        };
      });
    }
    return captured;
  },
  SaveSettings: async (settings: Settings) => {
    if (failSave) {
      (window as Window & { __PR_NANNY_SAVE_PENDING__?: boolean }).__PR_NANNY_SAVE_PENDING__ = true;
      await new Promise((resolve) => setTimeout(resolve, 300));
      (window as Window & { __PR_NANNY_SAVE_PENDING__?: boolean }).__PR_NANNY_SAVE_PENDING__ = false;
      throw new Error('safe fixture failure');
    }
    state.settings = structuredClone(settings);
    return structuredClone(state);
  },
  ConnectPAT: async (_token: string) => {
    (window as Window & { __PR_NANNY_PAT_PENDING__?: boolean }).__PR_NANNY_PAT_PENDING__ = true;
    await new Promise((resolve) => setTimeout(resolve, 300));
    (window as Window & { __PR_NANNY_PAT_PENDING__?: boolean }).__PR_NANNY_PAT_PENDING__ = false;
    if (failPAT) throw new Error('safe fixture failure');
    state.settings.authMethod = 'pat';
    state.connection = 'connected';
    return structuredClone(state);
  },
  ConnectCLI: async () => {
    state.settings.authMethod = 'gh';
    state.connection = 'connected';
    return structuredClone(state);
  },
  SetBell: async (id: string, enabled: boolean) => {
    state.pullRequests = state.pullRequests.map((pr) => (pr.id === id ? { ...pr, bell: enabled } : pr));
    return structuredClone(state);
  },
  SetLayout: async (layout: string) => {
    state.settings.layout = layout;
    return structuredClone(state);
  },
  Refresh: async () => {
    state.lastRefresh = new Date().toISOString();
  },
  OpenPR: async (id: string) => {
    const w = window as Window & { __openedPR?: string; __openedPRCount?: number };
    w.__openedPR = id;
    w.__openedPRCount = (w.__openedPRCount ?? 0) + 1;
  },
  HidePopup: async () => {
    (window as Window & { __PR_NANNY_HIDDEN__?: boolean }).__PR_NANNY_HIDDEN__ = true;
  },
};
export function installMockBridge() {
  window.__PR_NANNY_TEST_BRIDGE__ = mockBridge;
}

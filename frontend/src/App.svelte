<script lang="ts">
import { onMount } from 'svelte';
import PrRow from './components/PrRow.svelte';
import Settings from './components/Settings.svelte';
import { bridge } from './lib/bridge';
import { relativeAge } from './lib/format';
import type { PullRequest, Snapshot } from './lib/types';

let snapshot = $state<Snapshot | null>(null);
let settingsOpen = $state(new URLSearchParams(location.search).has('settings'));
let activeTab = $state<'authored' | 'review' | 'recent'>('authored');
let error = $state('');
let loading = $state(true);
let busy = $state(false);
let now = $state(Date.now());
// Relative-time clock: local only, never calls the bridge.
const CLOCK_MS = 1000;
// Snapshot sync: bounded poll while the popup is visible, none while hidden.
const SYNC_MS = 5000;
let clockTimer: ReturnType<typeof setInterval> | undefined;
let syncTimer: ReturnType<typeof setInterval> | undefined;
let alive = true;
let stateVersion = 0;
let snapshotRequest = 0;
let syncInFlight = false;
let lastSerialized = '';
const visible = () => document.visibilityState !== 'hidden';
const prs = $derived(
  [...(snapshot?.pullRequests ?? [])].sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt)),
);
const recent = (pr: PullRequest) =>
  !!pr.closedAt && now - Date.parse(pr.closedAt) < 60_000 && now >= Date.parse(pr.closedAt);
const authored = () => prs.filter((pr) => pr.authored && !pr.closedAt);
const reviews = () => prs.filter((pr) => pr.reviewRequested && !pr.closedAt);
const recents = () => prs.filter(recent);
let categoryRows = $derived({ authored: authored(), review: reviews(), recent: recents() }[activeTab]);
const sections = $derived([
  { title: 'Authored by you', key: 'authored', items: authored() },
  { title: 'Review requested', key: 'review', items: reviews() },
  { title: 'Recently closed', key: 'recent', items: recents() },
]);
const lastRefresh = $derived(snapshot?.lastRefresh ? relativeAge(snapshot.lastRefresh, now) : '—');
const authRequired = $derived(
  ['disconnected', 'missing_auth', 'auth_error', 'authentication_error'].includes(snapshot?.connection ?? ''),
);
const statusMessage = $derived(
  snapshot?.message ||
    (
      {
        offline: 'You are offline. Showing the pull requests saved from your last refresh.',
        rate_limited: 'GitHub is limiting requests. PR Nanny will try again automatically.',
        error: 'GitHub could not be reached. PR Nanny will retry automatically.',
      } as Record<string, string>
    )[snapshot?.connection ?? ''] ||
    '',
);

// Skips the assignment when nothing changed so unchanged polls cause no rerender.
function apply(next: Snapshot) {
  const serialized = JSON.stringify(next);
  if (serialized === lastSerialized) return;
  lastSerialized = serialized;
  snapshot = next;
}
async function sync() {
  const request = ++snapshotRequest;
  const version = stateVersion;
  syncInFlight = true;
  try {
    const next = await bridge.GetSnapshot();
    if (alive && !busy && request === snapshotRequest && version === stateVersion) {
      apply(next);
      error = '';
    }
  } catch {
    if (alive && !busy && request === snapshotRequest && version === stateVersion)
      error = 'Could not reach PR Nanny. Try refreshing.';
  } finally {
    if (alive && request === snapshotRequest) {
      loading = false;
      syncInFlight = false;
    }
  }
}
function tickClock() {
  now = Date.now();
}
function stopTimers() {
  if (clockTimer) clearInterval(clockTimer);
  if (syncTimer) clearInterval(syncTimer);
  clockTimer = syncTimer = undefined;
}
function startTimers() {
  stopTimers();
  clockTimer = setInterval(tickClock, CLOCK_MS);
  syncTimer = setInterval(() => {
    if (!syncInFlight) void sync();
  }, SYNC_MS);
}
// Becoming visible or focused (popup reopened): refresh the clock and snapshot now.
function resume() {
  if (!alive || !visible()) return;
  tickClock();
  startTimers();
  void sync();
}
function onVisibility() {
  if (visible()) resume();
  else stopTimers();
}
async function act(action: () => Promise<Snapshot>): Promise<Snapshot> {
  const version = ++stateVersion;
  busy = true;
  error = '';
  try {
    const next = await action();
    if (alive && version === stateVersion) apply(next);
    return next;
  } catch {
    if (alive && version === stateVersion) error = 'That action did not complete. Please try again.';
    throw new Error('Action failed');
  } finally {
    if (version === stateVersion) {
      stateVersion++;
      busy = false;
    }
  }
}
async function hide() {
  try {
    await bridge.HidePopup();
  } catch {
    /* Web preview has no native popup. */
  }
}
async function openPR(id: string) {
  try {
    await bridge.OpenPR(id);
  } catch {
    error = 'Could not open this pull request.';
  }
}
function fire(action: () => Promise<Snapshot>) {
  void act(action).catch(() => {});
}
function updateBell(id: string, enabled: boolean) {
  fire(() => bridge.SetBell(id, enabled));
}
function back() {
  settingsOpen = false;
  history.replaceState(null, '', location.pathname);
}
onMount(() => {
  void sync();
  if (visible()) startTimers();
  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('focus', resume);
  const keydown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      void hide();
    }
  };
  const outside = (event: MouseEvent) => {
    if (event.target === document.documentElement || event.target === document.body) void hide();
  };
  window.addEventListener('keydown', keydown);
  document.addEventListener('click', outside);
  return () => {
    alive = false;
    stopTimers();
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('focus', resume);
    window.removeEventListener('keydown', keydown);
    document.removeEventListener('click', outside);
  };
});
</script>

<svelte:head><title>{settingsOpen ? 'Settings · PR Nanny' : 'PR Nanny'}</title></svelte:head>
<main class="popup">
  {#if settingsOpen && snapshot}
    <Settings
      initial={snapshot.settings}
      connected={snapshot.connection}
      message={snapshot.message}
      onBack={back}
      onSave={(value) => act(() => bridge.SaveSettings(value))}
      onConnectPAT={(value) => act(() => bridge.ConnectPAT(value))}
      onConnectCLI={() => act(() => bridge.ConnectCLI())}
    />
  {:else}
    <header class="topbar">
      <div class="brand">
        <span class="brand-mark"
          ><svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M12 3.2 18 5.4v5.1c0 4.3-2.7 7.8-6 9.6-3.3-1.8-6-5.3-6-9.6V5.4l6-2.2Z" />
            <path d="m9.3 11.8 1.8 1.8 3.8-4" />
          </svg></span
        ><strong>PR Nanny</strong>
      </div>
      <div class="actions">
        <button
          type="button"
          class="icon-button"
          aria-label="Refresh pull requests"
          title="Refresh"
          disabled={busy}
          onclick={() =>
            fire(async () => {
              await bridge.Refresh();
              return bridge.GetSnapshot();
            })}
        >
          <svg class:spin={busy || snapshot?.refreshing} viewBox="0 0 20 20" aria-hidden="true">
            <path
              d="M16.2 7A6.5 6.5 0 0 0 4.4 5.3L3 7m0 0V3.9M3 7h3.2M3.8 13a6.5 6.5 0 0 0 11.8 1.7L17 13m0 0v3.1M17 13h-3.2"
            />
          </svg>
        </button><button
          type="button"
          class="icon-button"
          aria-label="Open settings"
          title="Settings"
          onclick={() => {
            settingsOpen = true;
            history.replaceState(null, '', '?settings=1');
          }}
        >
          <svg viewBox="0 0 20 20" aria-hidden="true">
            <path
              d="M8 2.8h4l.5 1.9 1.5.9 1.9-.5 2 3.4-1.4 1.4v1.8l1.4 1.4-2 3.4-1.9-.5-1.5.9-.5 1.9H8l-.5-1.9-1.5-.9-1.9.5-2-3.4 1.4-1.4v-1.8L2.1 8.5l2-3.4 1.9.5 1.5-.9L8 2.8Z"
            />
            <circle cx="10" cy="10.8" r="2.4" />
          </svg>
        </button>
      </div>
    </header>
    {#if snapshot}
      <div class="connection-bar" class:error-connection={snapshot.connection === 'error'}>
        <span class="connection-dot" class:connected={snapshot.connection === 'connected'}></span
        ><span
          >{snapshot.connection === 'connected'
            ? `Connected${snapshot.settings.account ? ` as ${snapshot.settings.account}` : ''}`
            : ((
                {
                  offline: 'Offline · showing saved pull requests',
                  rate_limited: 'GitHub rate limit reached',
                  error: 'Connection problem',
                  auth_error: 'Authentication expired',
                  authentication_error: 'Authentication expired',
                } as Record<string, string>
              )[snapshot.connection] ?? 'Not connected')}</span
        ><span class="refresh-time"
          >{snapshot.lastRefresh
            ? lastRefresh === 'now'
              ? 'Updated just now'
              : `Updated ${lastRefresh} ago`
            : ''}</span
        >
      </div>
      {#if error}
        <div class="error-banner" role="alert">{error}</div>
      {:else if statusMessage && (snapshot.connection !== 'connected' || snapshot.message)}
        <div class="info-banner" role="status">{statusMessage}</div>
      {/if}
      {#if authRequired}
        <section class="empty-state">
          <div class="empty-icon">
            <svg viewBox="0 0 40 40" aria-hidden="true">
              <path d="M20 4.5 31 8.6V18c0 7.8-4.9 14.2-11 17.5C13.9 32.2 9 25.8 9 18V8.6l11-4.1Z" />
              <path d="m15 20 3.3 3.3 6.7-7" />
            </svg>
          </div>
          <h1>
            {['error', 'auth_error', 'rate_limited'].includes(snapshot.connection)
              ? 'Reconnect to GitHub'
              : 'Connect to GitHub'}
          </h1>
          <p>
            {snapshot.message ||
              (['error', 'auth_error', 'rate_limited'].includes(snapshot.connection)
                ? 'Your connection needs attention to keep PRs up to date.'
                : 'Connect your account to see your open pull requests here.')}
          </p>
          <button
            type="button"
            class="primary"
            onclick={() => {
              settingsOpen = true;
              history.replaceState(null, '', '?settings=1');
            }}
          >
            Open Settings
          </button>
        </section>
      {:else}
        <div class="layout-bar">
          <fieldset class="layout-toggle">
            <legend class="sr-only">Pull request layout</legend>
            <button
              type="button"
              class:chosen={snapshot.settings.layout === 'all'}
              disabled={busy}
              aria-pressed={snapshot.settings.layout === 'all'}
              onclick={() => fire(() => bridge.SetLayout('all'))}
            >
              Show all
            </button><button
              type="button"
              class:chosen={snapshot.settings.layout === 'tabs'}
              disabled={busy}
              aria-pressed={snapshot.settings.layout === 'tabs'}
              onclick={() => fire(() => bridge.SetLayout('tabs'))}
            >
              Tabs
            </button>
          </fieldset>
          <span class="pr-count">{prs.length} {prs.length === 1 ? 'pull request' : 'pull requests'}</span>
        </div>
        {#if snapshot.settings.layout === 'tabs'}
          <nav class="tabs" aria-label="Pull request categories">
            {#each [
              { key: 'authored', label: 'Yours', count: authored().length },
              { key: 'review', label: 'Review', count: reviews().length },
              { key: 'recent', label: 'Recent', count: recents().length },
            ] as tab}
              <button
                type="button"
                class:active={activeTab === tab.key}
                aria-current={activeTab === tab.key ? 'page' : undefined}
                onclick={() => (activeTab = tab.key as typeof activeTab)}
              >
                {tab.label}<span>{tab.count}</span>
              </button>
            {/each}
          </nav>
          <div class="list-area" aria-live="polite">
            {#if categoryRows.length}
              <div class="pr-list">
                {#each categoryRows as pr (pr.id)}
                  <PrRow {pr} {busy} onBell={updateBell} onOpen={openPR} />
                {/each}
              </div>
            {:else}
              <div class="list-empty">
                <p>
                  {activeTab === 'authored'
                    ? 'No open pull requests authored by you.'
                    : activeTab === 'review'
                      ? 'No review requests right now.'
                      : 'No recently closed pull requests.'}
                </p>
              </div>
            {/if}
          </div>
        {:else}
          <div class="list-area all-layout" aria-live="polite">
            {#if prs.length === 0}
              <div class="list-empty">
                <p>You’re all caught up.</p>
                <span>New pull requests will show up here.</span>
              </div>
            {:else}
              {#each sections as section}
                <section class="pr-section">
                  <div class="section-heading">
                    <h2>{section.title}</h2>
                    <span>{section.items.length}</span>
                  </div>
                  {#if section.items.length}
                    <div class="pr-list">
                      {#each section.items as pr (section.key + pr.id)}
                        <PrRow {pr} {busy} onBell={updateBell} onOpen={openPR} />
                      {/each}
                    </div>
                  {:else}
                    <p class="section-empty">None right now</p>
                  {/if}
                </section>
              {/each}
            {/if}
          </div>
        {/if}
      {/if}
    {:else if loading}
      <div class="loading" role="status"><span></span>Loading pull requests…</div>
    {:else}
      <section class="empty-state">
        <h1>PR Nanny is unavailable</h1>
        <p>{error || 'Try refreshing in a moment.'}</p>
        <button type="button" class="primary" onclick={() => void sync()}>Try again</button>
      </section>
    {/if}
  {/if}
</main>

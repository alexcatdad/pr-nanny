<script lang="ts">
  import { untrack } from 'svelte';
  import type { Settings, Snapshot } from '../lib/types';
  let { initial, onSave, onConnectPAT, onConnectCLI, onBack, connected, message }: {
    initial: Settings; onSave: (value: Settings) => Promise<Snapshot>; onConnectPAT: (value: string) => Promise<Snapshot>; onConnectCLI: () => Promise<Snapshot>; onBack: () => void; connected: string; message: string;
  } = $props();
  const copySettings = (value: Settings): Settings => ({ ...value, events: { ...value.events } });
  let settings = $state(untrack(() => copySettings(initial)));
  let token = $state('');
  let busy = $state(false);
  let notice = $state('');
  const update = async () => { busy = true; notice = ''; const preferences = copySettings(settings); preferences.authMethod = initial.authMethod; preferences.account = initial.account; try { const next = await onSave(preferences); settings = { ...copySettings(next.settings), authMethod: settings.authMethod }; } catch { notice = 'Could not save settings. Try again.'; } finally { busy = false; } };
  const submitToken = async () => {
    const submitted = token;
    token = '';
    if (!submitted) return;
    busy = true; notice = '';
    try { const next = await onConnectPAT(submitted); settings = copySettings(next.settings); notice = 'GitHub connected.'; }
    catch { notice = 'Could not validate that token. Check it and try again.'; }
    finally { busy = false; }
  };
  const connectCLI = async () => { busy = true; notice = ''; try { const next = await onConnectCLI(); settings = copySettings(next.settings); notice = 'GitHub CLI connected.'; } catch { notice = 'Could not find an authenticated GitHub CLI session.'; } finally { busy = false; } };
  const change = () => { void update(); };
  const events: [keyof Settings['events'], string][] = [['ci', 'CI status changes'], ['approval', 'Approval changes'], ['reviewRequested', 'Review requests'], ['merged', 'Pull request merged'], ['closed', 'Pull request closed']];
</script>

<div class="settings-page">
  <header class="settings-top"><button class="back" onclick={onBack} aria-label="Back to pull requests"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="m12.5 4-6 6 6 6" /></svg></button><h1>Settings</h1><span class="connection"><i class:online={connected === 'connected'}></i>{connected === 'connected' ? `Connected as ${settings.account || 'GitHub user'}` : connected === 'error' ? 'Connection needs attention' : 'Not connected'}</span></header>
  {#if message}<p class="settings-message" role="status">{message}</p>{/if}
  <section class="setting-section"><h2>GitHub account</h2><p>Choose how PR Nanny connects to GitHub.</p>
    <div class="segmented" role="group" aria-label="Authentication method">
      <button class:chosen={settings.authMethod === 'gh'} onclick={() => { settings.authMethod = 'gh'; token = ''; }} disabled={busy}>GitHub CLI</button><button class:chosen={settings.authMethod === 'pat'} onclick={() => { settings.authMethod = 'pat'; token = ''; }} disabled={busy}>Personal access token</button>
    </div>
    {#if settings.authMethod === 'pat'}
      <form class="token-form" onsubmit={(event) => { event.preventDefault(); void submitToken(); }}><label for="pat">Personal access token</label><div class="token-entry"><input id="pat" type="password" disabled={busy} autocomplete="off" spellcheck="false" bind:value={token} placeholder="ghp_••••••••••••" /><button type="submit" disabled={busy || !token}>Connect</button></div><small>Stored securely in your operating system’s credential store.</small></form>
    {:else}<button class="secondary connect-cli" onclick={() => void connectCLI()} disabled={busy}>Connect GitHub CLI</button>{/if}
  </section>
  <section class="setting-section"><h2>Updates</h2><label class="select-row"><span><strong>Check every</strong><small>How often PR Nanny checks GitHub</small></span><select aria-label="Update interval" disabled={busy} bind:value={settings.intervalSeconds} onchange={change}><option value={30}>30 seconds</option><option value={60}>60 seconds</option><option value={120}>2 minutes</option><option value={300}>5 minutes</option></select></label></section>
  <section class="setting-section"><h2>Notifications</h2><label class="toggle-row"><span><strong>Notifications</strong><small>Allow PR Nanny to send notifications</small></span><input type="checkbox" disabled={busy} bind:checked={settings.notifications} onchange={change} /></label>
    <div class="event-list" class:disabled={!settings.notifications}>{#each events as [key, label]}<label class="event-option"><span>{label}</span><input type="checkbox" bind:checked={settings.events[key]} disabled={busy || !settings.notifications} onchange={change} /></label>{/each}</div>
    <p class="hint">New review requests notify automatically. Other changes notify only for pull requests with their bell enabled.</p>
  </section>
  <section class="setting-section"><h2>Startup</h2><label class="toggle-row"><span><strong>Launch at login</strong><small>Start PR Nanny when you sign in</small></span><input type="checkbox" disabled={busy} bind:checked={settings.launchAtLogin} onchange={change} /></label></section>
  {#if notice}<p class="notice" role="status">{notice}</p>{/if}
</div>

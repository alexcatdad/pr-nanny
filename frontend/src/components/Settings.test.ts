import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Settings as Preferences, Snapshot } from '../lib/types';
import Settings from './Settings.svelte';

const initial: Preferences = {
  version: 1,
  authMethod: 'gh',
  account: 'octocat',
  intervalSeconds: 60,
  layout: 'all',
  notifications: true,
  events: { ci: true, approval: true, reviewRequested: true, merged: true, closed: true },
  launchAtLogin: true,
};

function connectedSnapshot(authMethod: Preferences['authMethod']): Snapshot {
  return {
    settings: { ...initial, authMethod },
    pullRequests: [],
    connection: 'connected',
    message: '',
    lastRefresh: new Date(0).toISOString(),
    refreshing: false,
  };
}

afterEach(cleanup);

describe('settings authentication success', () => {
  it('clears the token and confirms a successful PAT connection', async () => {
    const token = 'synthetic-pat-for-settings-test';
    const onConnectPAT = vi.fn(async () => connectedSnapshot('pat'));
    render(Settings, {
      props: {
        initial,
        connected: 'disconnected',
        message: '',
        onBack: vi.fn(),
        onSave: vi.fn(),
        onConnectPAT,
        onConnectCLI: vi.fn(),
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Personal access token' }));
    const input = screen.getByLabelText('Personal access token');
    await fireEvent.input(input, { target: { value: token } });
    await fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

    expect(onConnectPAT).toHaveBeenCalledWith(token);
    expect(input).toHaveValue('');
    expect(await screen.findByRole('status')).toHaveTextContent('GitHub connected.');
  });

  it('confirms a successful GitHub CLI connection', async () => {
    const onConnectCLI = vi.fn(async () => connectedSnapshot('gh'));
    render(Settings, {
      props: {
        initial,
        connected: 'disconnected',
        message: '',
        onBack: vi.fn(),
        onSave: vi.fn(),
        onConnectPAT: vi.fn(),
        onConnectCLI,
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Connect GitHub CLI' }));

    expect(onConnectCLI).toHaveBeenCalledOnce();
    expect(await screen.findByRole('status')).toHaveTextContent('GitHub CLI connected.');
  });
});

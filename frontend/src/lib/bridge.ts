import type { Settings, Snapshot } from './types';

type Bridge = {
  GetSnapshot: () => Promise<Snapshot>;
  SaveSettings: (settings: Settings) => Promise<Snapshot>;
  ConnectPAT: (token: string) => Promise<Snapshot>;
  ConnectCLI: () => Promise<Snapshot>;
  SetBell: (id: string, enabled: boolean) => Promise<Snapshot>;
  SetLayout: (layout: string) => Promise<Snapshot>;
  Refresh: () => Promise<void>;
  OpenPR: (id: string) => Promise<void>;
  HidePopup: () => Promise<void>;
};

declare global {
  interface Window { __PR_NANNY_TEST_BRIDGE__?: Bridge }
}

let runtime: Promise<{ Call: { ByName: (name: string, ...args: unknown[]) => Promise<unknown> } }> | undefined;
const runtimePath: string = '/wails/runtime.js';
const loadRuntime = () => runtime ??= import(/* @vite-ignore */ runtimePath) as Promise<{ Call: { ByName: (name: string, ...args: unknown[]) => Promise<unknown> } }>;

function invoke<T>(method: keyof Bridge, ...args: unknown[]): Promise<T> {
  if ((import.meta.env.MODE === 'test' || (import.meta.env.VITE_TEST_FIXTURES === 'true')) && window.__PR_NANNY_TEST_BRIDGE__) {
    return (window.__PR_NANNY_TEST_BRIDGE__[method] as (...values: unknown[]) => Promise<T>)(...args);
  }
  return loadRuntime().then(({ Call }) => Call.ByName(`main.Service.${method}`, ...args) as Promise<T>);
}

// This is the only frontend-to-Go boundary. All authentication and network work stays in Go.
export const bridge: Bridge = {
  GetSnapshot: () => invoke('GetSnapshot'),
  SaveSettings: settings => invoke('SaveSettings', settings),
  ConnectPAT: token => invoke('ConnectPAT', token),
  ConnectCLI: () => invoke('ConnectCLI'),
  SetBell: (id, enabled) => invoke('SetBell', id, enabled),
  SetLayout: layout => invoke('SetLayout', layout),
  Refresh: () => invoke('Refresh'),
  OpenPR: id => invoke('OpenPR', id),
  HidePopup: () => invoke('HidePopup'),
};

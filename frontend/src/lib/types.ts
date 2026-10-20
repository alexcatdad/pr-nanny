export type CI = 'passed' | 'not_passed' | 'running' | 'not_available';
export type PullRequest = {
  id: string;
  number: number;
  title: string;
  repository: string;
  url: string;
  createdAt: string;
  updatedAt: string;
  closedAt?: string | null;
  merged: boolean;
  approved: boolean;
  ci: CI;
  authored: boolean;
  reviewRequested: boolean;
  bell: boolean;
};
export type EventPreferences = {
  ci: boolean;
  approval: boolean;
  reviewRequested: boolean;
  merged: boolean;
  closed: boolean;
};
export type Settings = {
  version: number;
  authMethod: string;
  account: string;
  intervalSeconds: number;
  layout: string;
  notifications: boolean;
  events: EventPreferences;
  launchAtLogin: boolean;
};
export type Snapshot = {
  settings: Settings;
  pullRequests: PullRequest[];
  connection: string;
  message: string;
  lastRefresh?: string | null;
  refreshing: boolean;
  degraded?: boolean;
};

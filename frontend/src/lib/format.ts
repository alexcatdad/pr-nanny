export function relativeAge(value: string, now = Date.now()): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return '—';
  const seconds = Math.max(0, Math.floor((now - timestamp) / 1000));
  if (seconds < 60) return 'now';
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  if (seconds < 2592000) return `${Math.floor(seconds / 86400)}d`;
  if (seconds < 31536000) return `${Math.floor(seconds / 2592000)}mo`;
  return `${Math.floor(seconds / 31536000)}y`;
}
export const statusLabel = (pr: { approved: boolean; ci: string }) => ({ approval: pr.approved ? 'Approved' : 'Not approved', ci: ({ passed: 'Passed', not_passed: 'Not passed', running: 'Running', not_available: 'Not available' } as Record<string, string>)[pr.ci] ?? 'Not available' });

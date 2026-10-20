<script lang="ts">
import { relativeAge, statusLabel } from '../lib/format';
import type { PullRequest } from '../lib/types';

let {
  pr,
  onBell,
  onOpen,
  busy = false,
}: {
  pr: PullRequest;
  onBell: (id: string, value: boolean) => void;
  onOpen: (id: string) => void;
  busy?: boolean;
} = $props();
let states = $derived(statusLabel(pr));
const stop = (event: MouseEvent) => event.stopPropagation();
</script>

<div class="pr-row">
  <button
    type="button"
    class="pr-open"
    aria-label={`Open ${pr.repository} pull request #${pr.number}: ${pr.title}`}
    onclick={() => onOpen(pr.id)}
  ></button>
  <div class="pr-main">
    <div class="pr-heading">
      <span class="repo">{pr.repository}</span><span class="number">#{pr.number}</span
      ><span class="age" title={pr.createdAt}>{relativeAge(pr.createdAt)}</span>
    </div>
    <div class="title" title={pr.title}>{pr.title}</div>
    <div class="status-line">
      <span class:approved={pr.approved} class="status approval"><span class="dot"></span>{states.approval}</span>
      <span class={`status ci ${pr.ci}`}><span class="dot"></span>{states.ci}</span>
      {#if pr.merged}
        <span class="merged">Merged</span>
      {/if}
    </div>
  </div>
  <button
    type="button"
    class:enabled={pr.bell}
    class="bell"
    aria-label={pr.bell ? 'Turn notifications off' : 'Turn notifications on'}
    aria-pressed={pr.bell}
    disabled={busy}
    title={pr.bell ? 'Notifications on' : 'Notifications off'}
    onclick={(event) => {
      stop(event);
      onBell(pr.id, !pr.bell);
    }}
  >
    <svg viewBox="0 0 20 20" aria-hidden="true">
      <path d="M15.5 8a5.5 5.5 0 0 0-11 0c0 6-2 6-2 7.5h15C17.5 14 15.5 14 15.5 8ZM8 18h4" />
    </svg>
  </button>
</div>

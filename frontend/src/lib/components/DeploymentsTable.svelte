<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { fade } from 'svelte/transition';
  import { formatDistanceToNow } from 'date-fns'; // Recommended for 2026 UI standards

  // Props
  export let deployments: any[] = [];
  export let loading: boolean = false;

  const dispatch = createEventDispatcher();

  // 🛡️ SLA: Status color mapping to Kari Brand Palette
  const statusConfig = {
    PENDING: { color: 'text-amber-600 bg-amber-50 border-amber-200', label: 'Queued' },
    RUNNING: { color: 'text-kari-teal bg-kari-teal/5 border-kari-teal/20', label: 'In Progress' },
    SUCCESS: { color: 'text-emerald-600 bg-emerald-50 border-emerald-200', label: 'Stable' },
    FAILED: { color: 'text-red-600 bg-red-50 border-red-200', label: 'Alert' }
  };

  function handleViewLogs(id: string) {
    // 📡 Dispatch to parent to switch view to the Terminal
    dispatch('select', { id });
  }
</script>

<div class="card w-full bg-white border border-kari-warm-gray/10 shadow-kari overflow-hidden">
  <div class="px-6 py-4 border-b border-kari-warm-gray/10 bg-gray-50/50 flex justify-between items-center">
    <h3 class="text-sm font-bold text-kari-text uppercase tracking-widest">Recent Deployments</h3>
    <div class="flex gap-2">
      {#if loading}
        <span class="w-4 h-4 border-2 border-kari-teal/30 border-t-kari-teal rounded-full animate-spin"></span>
      {/if}
    </div>
  </div>

  <div class="overflow-x-auto">
    <table class="w-full text-left border-collapse">
      <thead>
        <tr class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-tighter bg-slate-50">
          <th class="px-6 py-3">Environment / App</th>
          <th class="px-6 py-3">Status</th>
          <th class="px-6 py-3">Branch</th>
          <th class="px-6 py-3">Initiated</th>
          <th class="px-6 py-3 text-right">Telemetry</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100">
        {#each deployments as d (d.id)}
          <tr class="hover:bg-kari-light-gray/30 transition-colors group" transition:fade>
            <td class="px-6 py-4">
              <div class="flex flex-col">
                <span class="text-sm font-semibold text-kari-text">{d.domain_name}</span>
                <span class="text-[10px] font-mono text-kari-warm-gray">{d.id.slice(0, 8)}</span>
              </div>
            </td>
            <td class="px-6 py-4">
              {@const conf = statusConfig[d.status] || statusConfig.PENDING}
              <span class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold border {conf.color}">
                {conf.label}
              </span>
            </td>
            <td class="px-6 py-4">
              <span class="text-xs font-mono text-slate-500">
                <svg xmlns="http://www.w3.org/2000/svg" class="inline h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
                </svg>
                {d.branch}
              </span>
            </td>
            <td class="px-6 py-4">
              <span class="text-xs text-slate-500">
                {formatDistanceToNow(new Date(d.created_at))} ago
              </span>
            </td>
            <td class="px-6 py-4 text-right">
              <button 
                on:click={() => handleViewLogs(d.id)}
                class="inline-flex items-center gap-2 px-3 py-1.5 text-xs font-bold text-kari-teal hover:bg-kari-teal hover:text-white rounded transition-all border border-kari-teal/20"
              >
                View Console
                <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 5l7 7-7 7M5 5l7 7-7 7" />
                </svg>
              </button>
            </td>
          </tr>
        {:else}
          <tr>
            <td colspan="5" class="px-6 py-12 text-center text-slate-400 italic text-sm">
              The Kari Muscle is idle. No deployments recorded.
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

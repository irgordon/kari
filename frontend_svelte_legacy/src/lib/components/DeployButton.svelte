<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Rocket, Loader2, AlertCircle } from 'lucide-svelte';
  import { fade } from 'svelte/transition';

  const dispatch = createEventDispatcher();

  // Props
  export let appData: any; // The payload from the Wizard
  export let disabled: boolean = false;

  // Internal State
  let status: 'idle' | 'loading' | 'error' = 'idle';
  let errorMessage: string = "";

  async function initiateDeployment() {
    if (status === 'loading' || disabled) return;

    status = 'loading';
    errorMessage = "";

    try {
      // 📡 POST to the Go Brain's hardened endpoint
      const response = await fetch('/api/v1/deployments', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          // 🛡️ Zero-Trust: In production, include the JWT here
          // 'Authorization': `Bearer ${token}` 
        },
        body: JSON.stringify(appData)
      });

      const result = await response.json();

      if (!response.ok) {
        throw new Error(result.error || "Brain rejected the deployment request.");
      }

      // ✅ Success: Dispatch the Trace ID to the Orchestrator (+page.svelte)
      // This will trigger the transition to the Terminal view
      dispatch('success', { traceId: result.trace_id });
      
    } catch (err: any) {
      status = 'error';
      errorMessage = err.message;
      console.error("Kari Panel Deployment Error:", err);
    } finally {
      if (status === 'loading') status = 'idle';
    }
  }
</script>

<div class="flex flex-col gap-3">
  {#if status === 'error'}
    <div 
      transition:fade 
      class="flex items-center gap-2 p-3 bg-red-50 border border-red-200 rounded-md text-red-700 text-xs font-medium"
    >
      <AlertCircle size={14} />
      <span>{errorMessage}</span>
    </div>
  {/if}

  <button
    on:click={initiateDeployment}
    disabled={disabled || status === 'loading'}
    class="relative flex items-center justify-center gap-3 w-full px-6 py-3 rounded-lg font-bold text-sm tracking-wide transition-all duration-200
    {status === 'loading' 
      ? 'bg-kari-warm-gray text-white cursor-wait' 
      : 'bg-kari-teal text-white hover:bg-[#158e87] active:scale-[0.98] shadow-lg shadow-kari-teal/20'
    } 
    disabled:opacity-50 disabled:cursor-not-allowed disabled:shadow-none"
  >
    {#if status === 'loading'}
      <Loader2 size={18} class="animate-spin" />
      <span>Provisioning Jail...</span>
    {:else}
      <Rocket size={18} class={disabled ? 'opacity-50' : 'animate-pulse'} />
      <span>Initialize Deployment</span>
    {/if}
  </button>
  
  <p class="text-[10px] text-center text-kari-warm-gray italic">
    🛡️ This will provision a new isolated instance on the Kari Muscle.
  </p>
</div>

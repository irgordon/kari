<script lang="ts">
    import type { PageData } from './$types';
    import { API } from '$lib/api/client';
    import DeploymentTerminal from '$lib/components/DeploymentTerminal.svelte';

    export let data: PageData;
    $: app = data.application;

    // Local UI State
    let isDeploying = false;
    let activeTraceId: string | null = null;
    let deployError: string | null = null;

    // ==============================================================================
    // Actions
    // ==============================================================================

    async function triggerManualDeploy() {
        if (isDeploying) return;
        
        isDeploying = true;
        deployError = null;
        activeTraceId = null; // Reset terminal if already open

        try {
            // Use our centralized API client to handle silent token refreshes
            const response = await API.post<{ trace_id: string, message: string }>(
                `/api/v1/applications/${app.id}/deploy`
            );
            
            // The Go orchestrator accepted the request and generated a trace ID.
            // Setting this variable reactively mounts the Xterm.js terminal component.
            activeTraceId = response.trace_id;
            
        } catch (error: any) {
            console.error('Deployment trigger failed:', error);
            deployError = error.message || 'Failed to trigger deployment. Please check system logs.';
            isDeploying = false;
        }
    }

    // Callback so the terminal can inform the parent component when the socket closes
    function handleStreamComplete() {
        isDeploying = false;
    }
</script>

<svelte:head>
    <title>{app.repo_url.split('/').pop()?.replace('.git', '')} - Karı Control Panel</title>
</svelte:head>

<div class="mb-8 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-kari-warm-gray/20 pb-6">
    <div>
        <div class="flex items-center gap-3">
            <h2 class="text-2xl font-sans font-bold text-kari-text">
                {app.repo_url.split('/').pop()?.replace('.git', '')}
            </h2>
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-kari-light-gray text-kari-warm-gray border border-kari-warm-gray/30">
                {app.app_type}
            </span>
        </div>
        <p class="mt-1 text-sm text-kari-warm-gray font-mono">{app.id}</p>
    </div>
    
    <div class="flex-shrink-0 flex gap-3">
        <button 
            on:click={triggerManualDeploy}
            disabled={isDeploying}
            class="inline-flex items-center justify-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-sans font-medium text-white bg-kari-teal hover:bg-[#158C85] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-kari-teal transition-colors disabled:opacity-70 disabled:cursor-not-allowed"
        >
            {#if isDeploying && !activeTraceId}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Initiating...
            {:else}
                <svg xmlns="http://www.w3.org/2000/svg" class="-ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                </svg>
                Trigger Deployment
            {/if}
        </button>
    </div>
</div>

{#if deployError}
    <div class="mb-6 bg-red-50 border-l-4 border-red-500 p-4 rounded-md">
        <p class="text-sm text-red-700 font-medium">{deployError}</p>
    </div>
{/if}

<div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
    
    <div class="lg:col-span-1 space-y-6">
        <div class="card p-5">
            <h3 class="text-sm font-sans font-semibold text-kari-text mb-4 uppercase tracking-wider">Source Control</h3>
            <dl class="space-y-4">
                <div>
                    <dt class="text-xs font-medium text-kari-warm-gray">Repository URL</dt>
                    <dd class="mt-1 text-sm font-mono text-kari-text break-all">{app.repo_url}</dd>
                </div>
                <div>
                    <dt class="text-xs font-medium text-kari-warm-gray">Tracked Branch</dt>
                    <dd class="mt-1 text-sm font-mono text-kari-text flex items-center gap-1">
                        <svg class="w-4 h-4 text-kari-warm-gray" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2v-2"></path></svg>
                        {app.branch}
                    </dd>
                </div>
            </dl>
        </div>

        <div class="card p-5">
            <h3 class="text-sm font-sans font-semibold text-kari-text mb-4 uppercase tracking-wider">Build & Run</h3>
            <dl class="space-y-4">
                <div>
                    <dt class="text-xs font-medium text-kari-warm-gray">Build Command</dt>
                    <dd class="mt-1 text-sm font-mono text-kari-text bg-kari-light-gray px-2 py-1 rounded border border-kari-warm-gray/20">
                        {app.build_command || 'None'}
                    </dd>
                </div>
                <div>
                    <dt class="text-xs font-medium text-kari-warm-gray">Start Command</dt>
                    <dd class="mt-1 text-sm font-mono text-kari-text bg-kari-light-gray px-2 py-1 rounded border border-kari-warm-gray/20">
                        {app.start_command}
                    </dd>
                </div>
            </dl>
        </div>
    </div>

    <div class="lg:col-span-2">
        {#if activeTraceId}
            <div class="animate-fade-in">
                <DeploymentTerminal traceId={activeTraceId} />
            </div>
        {:else}
            <div class="card h-[600px] flex flex-col items-center justify-center text-center bg-gray-50/50">
                <svg class="h-12 w-12 text-kari-warm-gray/50 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                </svg>
                <h3 class="text-lg font-sans font-medium text-kari-text">Terminal Idle</h3>
                <p class="mt-1 text-sm text-kari-warm-gray max-w-sm">
                    Trigger a manual deployment or push to the <span class="font-mono">{app.branch}</span> branch to view live build logs.
                </p>
            </div>
        {/if}
    </div>
</div>

<style>
    .animate-fade-in {
        animation: fadeIn 0.3s ease-out forwards;
    }
    @keyframes fadeIn {
        from { opacity: 0; transform: translateY(5px); }
        to { opacity: 1; transform: translateY(0); }
    }
</style>

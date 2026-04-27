<script lang="ts">
	import { onMount } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { Plus, Table, Terminal as TerminalIcon, LayoutDashboard } from 'lucide-svelte';

	import AppCreationWizard from '$lib/components/AppCreationWizard.svelte';
	import DeploymentsTable from '$lib/components/DeploymentsTable.svelte';
	import DeploymentTerminal from '$lib/components/DeploymentTerminal.svelte';
	import type { Deployment } from '$lib/types';

	// 🛡️ State Management
	type ViewMode = 'list' | 'create' | 'terminal';
	let view: ViewMode = 'list';
	let activeTraceId: string | null = null;
	let deployments: Deployment[] = [];
	let loading = true;

	// 📡 Initial Data Fetch from Go Brain
	async function fetchDeployments() {
		loading = true;
		try {
			const res = await fetch('/api/v1/deployments');
			if (res.ok) {
				deployments = await res.json();
			}
		} catch (err) {
			console.error("Kari Panel: Failed to fetch deployments", err);
		} finally {
			loading = false;
		}
	}

	onMount(fetchDeployments);

	// 🛡️ Navigation Handlers
	function openTerminal(id: string) {
		activeTraceId = id;
		view = 'terminal';
	}

	function handleCreated(event: any) {
		// Called when Wizard finishes the POST request
		activeTraceId = event.detail.traceId;
		view = 'terminal';
	}
</script>

<svelte:head>
	<title>Deployments | Karı Panel</title>
</svelte:head>

<main class="min-h-screen bg-kari-light-gray/20 p-6 lg:p-10 space-y-8">
	<header class="flex flex-col md:flex-row md:items-center justify-between gap-4">
		<div class="space-y-1">
			<div class="flex items-center gap-2 text-xs font-bold text-kari-warm-gray uppercase tracking-widest">
				<LayoutDashboard size={14} />
				<span>Orchestration Engine</span>
			</div>
			<h1 class="text-3xl font-bold text-kari-text tracking-tight">System Deployments</h1>
		</div>

		<div class="flex items-center gap-3">
			<button 
				on:click={() => { view = 'list'; fetchDeployments(); }}
				class="flex items-center gap-2 px-4 py-2 rounded-md text-sm font-bold transition-all
				{view === 'list' ? 'bg-kari-text text-white' : 'bg-white text-kari-text border border-kari-warm-gray/20 hover:bg-gray-50'}"
			>
				<Table size={16} /> List
			</button>
			
			<button 
				on:click={() => view = 'create'}
				class="flex items-center gap-2 px-4 py-2 rounded-md text-sm font-bold transition-all
				{view === 'create' ? 'bg-kari-teal text-white shadow-lg shadow-kari-teal/20' : 'bg-white text-kari-teal border border-kari-teal/20 hover:bg-kari-teal/5'}"
			>
				<Plus size={16} /> New App
			</button>
		</div>
	</header>

	<hr class="border-kari-warm-gray/10" />

	<section class="relative">
		{#if view === 'list'}
			<div in:fade={{ duration: 200 }}>
				<DeploymentsTable 
					{deployments} 
					{loading} 
					on:select={(e) => openTerminal(e.detail.id)} 
				/>
			</div>
		{:else if view === 'create'}
			<div in:slide={{ duration: 300 }}>
				<AppCreationWizard on:success={handleCreated} />
			</div>
		{:else if view === 'terminal' && activeTraceId}
			<div in:fade={{ duration: 200 }} class="space-y-4">
				<div class="flex items-center justify-between bg-white p-4 rounded-lg border border-kari-warm-gray/10">
					<div class="flex items-center gap-3">
						<div class="p-2 bg-kari-teal/10 rounded-lg text-kari-teal">
							<TerminalIcon size={20} />
						</div>
						<div>
							<h3 class="text-sm font-bold text-kari-text">Live Build Console</h3>
							<p class="text-[10px] text-kari-warm-gray font-mono uppercase">{activeTraceId}</p>
						</div>
					</div>
					<button 
						on:click={() => view = 'list'} 
						class="text-xs font-bold text-kari-warm-gray hover:text-kari-text transition-colors"
					>
						Close Console & Return
					</button>
				</div>
				<DeploymentTerminal traceId={activeTraceId} />
			</div>
		{/if}
	</section>
</main>

<style>
	:global(body) {
		@apply bg-[#F4F5F6];
	}
</style>

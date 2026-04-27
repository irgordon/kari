<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import DeploymentTerminal from './DeploymentTerminal.svelte';

	const dispatch = createEventDispatcher();

	// 🛡️ State Management
	let step: 'details' | 'deploying' = 'details';
	let loading = false;
	let error: string | null = null;
	let activeTraceId: string | null = null;

	// Form Data
	let appData = {
		name: '',
		repo_url: '',
		branch: 'main',
		build_command: 'npm install && npm run build',
		target_port: 3000,
		ssh_key: ''
	};

	// 🛡️ Zero-Trust: Validate inputs before they hit the network
	function validate() {
		if (!appData.name || !appData.repo_url) return "App Name and Repo URL are required.";
		if (appData.target_port < 1024 || appData.target_port > 65535) return "Port must be in the range 1024-65535.";
		return null;
	}

	async function handleDeploy() {
		error = validate();
		if (error) return;

		loading = true;
		error = null;

		try {
			// 📡 Send to Go Brain API
			const response = await fetch('/api/v1/apps/deploy', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(appData)
			});

			const result = await response.json();

			if (!response.ok) throw new Error(result.error || 'Deployment failed to initialize');

			// 🚀 Transition to Terminal View
			activeTraceId = result.trace_id;
			step = 'deploying';
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}
</script>

<div class="max-w-4xl mx-auto">
	{#if step === 'details'}
		<div class="card bg-white shadow-xl border border-slate-200" transition:fade>
			<div class="p-8 space-y-6">
				<header>
					<h2 class="text-2xl font-bold text-slate-900">Create New Application</h2>
					<p class="text-slate-500 text-sm">Provision a new jail and proxy on the Kari Muscle.</p>
				</header>

				{#if error}
					<div class="p-3 bg-red-50 border border-red-200 text-red-600 text-xs rounded-md" transition:slide>
						⚠️ {error}
					</div>
				{/if}

				<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
					<div class="space-y-4">
						<label class="block">
							<span class="text-xs font-bold uppercase text-slate-400 tracking-wider">App Name</span>
							<input type="text" bind:value={appData.name} placeholder="my-awesome-app" 
								class="mt-1 block w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-md text-sm shadow-sm focus:outline-none focus:border-[#1BA8A0] focus:ring-1 focus:ring-[#1BA8A0]" />
						</label>

						<label class="block">
							<span class="text-xs font-bold uppercase text-slate-400 tracking-wider">Target Port</span>
							<input type="number" bind:value={appData.target_port} 
								class="mt-1 block w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-md text-sm shadow-sm focus:outline-none focus:border-[#1BA8A0] focus:ring-1 focus:ring-[#1BA8A0]" />
						</label>
					</div>

					<div class="space-y-4">
						<label class="block">
							<span class="text-xs font-bold uppercase text-slate-400 tracking-wider">Repository URL</span>
							<input type="text" bind:value={appData.repo_url} placeholder="git@github.com:user/repo.git" 
								class="mt-1 block w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-md text-sm shadow-sm focus:outline-none focus:border-[#1BA8A0] focus:ring-1 focus:ring-[#1BA8A0]" />
						</label>

						<label class="block">
							<span class="text-xs font-bold uppercase text-slate-400 tracking-wider">Branch</span>
							<input type="text" bind:value={appData.branch} 
								class="mt-1 block w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-md text-sm shadow-sm focus:outline-none focus:border-[#1BA8A0] focus:ring-1 focus:ring-[#1BA8A0]" />
						</label>
					</div>
				</div>

				<label class="block">
					<span class="text-xs font-bold uppercase text-slate-400 tracking-wider">Build Command</span>
					<input type="text" bind:value={appData.build_command} 
						class="mt-1 block w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-md text-sm shadow-sm font-mono focus:outline-none focus:border-[#1BA8A0] focus:ring-1 focus:ring-[#1BA8A0]" />
				</label>

				<label class="block">
					<span class="text-xs font-bold uppercase text-slate-400 tracking-wider">Private Deployment Key (SSH)</span>
					<textarea bind:value={appData.ssh_key} placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" rows="4"
						class="mt-1 block w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded-md text-xs font-mono text-emerald-500 shadow-sm focus:outline-none focus:border-[#1BA8A0] focus:ring-1 focus:ring-[#1BA8A0]"></textarea>
					<p class="mt-2 text-[10px] text-slate-400 italic">🛡️ This key will be encrypted via AES-256-GCM upon receipt by the Brain.</p>
				</label>

				<footer class="pt-4 flex justify-end">
					<button 
						on:click={handleDeploy}
						disabled={loading}
						class="px-6 py-2 bg-[#1BA8A0] text-white font-bold rounded-md shadow-lg hover:bg-[#158e87] transition-all disabled:opacity-50 flex items-center gap-2"
					>
						{#if loading}
							<span class="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></span>
						{/if}
						🚀 Initialize Deployment
					</button>
				</footer>
			</div>
		</div>
	{:else if step === 'deploying' && activeTraceId}
		<div class="space-y-6" transition:fade>
			<header class="flex justify-between items-center">
				<div>
					<h2 class="text-2xl font-bold text-slate-900">Provisioning {appData.name}</h2>
					<p class="text-slate-500 text-sm">Real-time build telemetry from the Muscle Agent</p>
				</div>
				<button on:click={() => step = 'details'} class="text-xs text-[#1BA8A0] font-bold hover:underline">
					← Return to Config
				</button>
			</header>

			<DeploymentTerminal traceId={activeTraceId} />
		</div>
	{/if}
</div>

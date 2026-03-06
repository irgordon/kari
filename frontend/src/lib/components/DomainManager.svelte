<script lang="ts">
	import { fade, slide } from 'svelte/transition';
	import { canPerform } from '$lib/utils/auth';
	import { page } from '$app/stores';

	export let domains: Array<{ name: string; status: string; created_at: string }> = [];
	export let appId: string;

	let newDomain = '';
	let targetPort = 8080;
	let isSubmitting = false;
	let error = '';

	// 🛡️ Zero-Trust: Frontend validation for FQDN (Fully Qualified Domain Name)
	const domainRegex = /^(?!-)[A-Za-z0-9-]{1,63}(?<!-)(\.[A-Za-z0-9-]{1,63})*$/;

	async function addDomain() {
		if (!canPerform($page.data.user, 'domains:write')) return;
		if (!domainRegex.test(newDomain)) {
			error = 'Invalid domain format.';
			return;
		}

		isSubmitting = true;
		error = '';

		const res = await fetch(`/api/v1/apps/${appId}/domains`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name: newDomain, port: targetPort })
		});

		if (res.ok) {
			const domain = await res.json();
			domains = [...domains, domain];
			newDomain = '';
		} else {
			error = 'Brain refused domain attachment. Check if it is already taken.';
		}
		isSubmitting = false;
	}

	async function removeDomain(name: string) {
		const res = await fetch(`/api/v1/apps/${appId}/domains/${name}`, { method: 'DELETE' });
		if (res.ok) {
			domains = domains.filter((d) => d.name !== name);
		}
	}
</script>

<div class="space-y-6 p-6 bg-base-200/30 rounded-2xl border border-base-content/5 backdrop-blur-md">
	<header class="flex items-center justify-between">
		<div>
			<h3 class="text-xl font-black tracking-tight">Public Routing</h3>
			<p class="text-xs opacity-50">Apache VHost entries mapped to jailed app ports.</p>
		</div>
	</header>

	<div class="flex gap-2 p-2 bg-base-300/50 rounded-lg">
		<input 
			bind:value={newDomain} 
			placeholder="app.example.com"
			class="input input-sm bg-transparent flex-1 font-mono text-sm focus:outline-none"
		/>
		<div class="flex items-center gap-2 px-2 border-l border-base-content/10">
			<span class="text-[10px] opacity-40 font-bold uppercase">Port</span>
			<input bind:value={targetPort} type="number" class="w-16 bg-transparent text-sm font-mono" />
		</div>
		<button 
			on:click={addDomain} 
			disabled={isSubmitting || !newDomain}
			class="btn btn-sm btn-primary"
		>
			{isSubmitting ? '...' : 'Attach'}
		</button>
	</div>

	{#if error}
		<p transition:slide class="text-[10px] text-error font-bold text-center uppercase tracking-widest">{error}</p>
	{/if}

	<div class="space-y-2">
		{#each domains as domain (domain.name)}
			<div 
				transition:fade 
				class="flex items-center justify-between p-3 bg-base-100/50 rounded-xl border border-base-content/5 group"
			>
				<div class="flex flex-col">
					<span class="font-mono text-sm font-bold">{domain.name}</span>
					<span class="text-[10px] opacity-40">Targeting internal port {targetPort}</span>
				</div>
				
				<div class="flex items-center gap-4">
					<span class="badge badge-xs {domain.status === 'active' ? 'badge-success' : 'badge-warning'} animate-pulse">
						{domain.status}
					</span>
					<button 
						on:click={() => removeDomain(domain.name)}
						class="btn btn-xs btn-ghost text-error opacity-0 group-hover:opacity-100 transition-all"
					>
						Detach
					</button>
				</div>
			</div>
		{/each}
	</div>
</div>

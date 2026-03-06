<script lang="ts">
	import { fade, slide } from 'svelte/transition';
	import { canPerform } from '$lib/utils/auth';
	import { page } from '$app/stores';

	export let domains: Array<{ name: string; ssl_active: boolean; ssl_expiry?: string; status: string }> = [];
	export let appId: string;

	let processingDomain = '';
	let message = '';

	async function provisionSSL(domainName: string) {
		if (!canPerform($page.data.user, 'ssl:write')) return;
		
		processingDomain = domainName;
		message = `Initiating ACME challenge for ${domainName}...`;

		const res = await fetch(`/api/v1/apps/${appId}/domains/${domainName}/ssl`, {
			method: 'POST'
		});

		if (res.ok) {
			message = 'Certificate issued and synced with Muscle.';
			// Update local state to reflect SSL status
			domains = domains.map(d => 
				d.name === domainName ? { ...d, ssl_active: true, status: 'securing' } : d
			);
		} else {
			message = 'SSL Provisioning failed. Check DNS propagation.';
		}
		
		processingDomain = '';
		setTimeout(() => message = '', 5000);
	}

	function daysUntil(dateString: string) {
		const expiry = new Date(dateString);
		const now = new Date();
		const diff = expiry.getTime() - now.getTime();
		return Math.ceil(diff / (1000 * 3600 * 24));
	}
</script>

<div class="space-y-4 p-6 bg-primary/5 rounded-2xl border border-primary/10 backdrop-blur-sm">
	<header class="flex items-center justify-between">
		<div>
			<h3 class="text-lg font-bold text-primary">Encryption & Identity</h3>
			<p class="text-[10px] opacity-60 uppercase tracking-widest">Automated TLS Termination via Let's Encrypt</p>
		</div>
		<div class="p-2 bg-success/20 rounded-full">
			<svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5 text-success" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
		</div>
	</header>

	{#if message}
		<div transition:slide class="p-2 text-[10px] bg-base-300 rounded text-center font-mono italic">
			{message}
		</div>
	{/if}

	<div class="grid gap-3">
		{#each domains as domain (domain.name)}
			<div class="flex items-center justify-between p-4 bg-base-100/40 rounded-xl border border-base-content/5">
				<div class="flex flex-col">
					<span class="text-sm font-mono font-bold">{domain.name}</span>
					{#if domain.ssl_active && domain.ssl_expiry}
						<span class="text-[9px] {daysUntil(domain.ssl_expiry) < 15 ? 'text-error' : 'text-success'} uppercase font-black">
							Expires in {daysUntil(domain.ssl_expiry)} days
						</span>
					{:else}
						<span class="text-[9px] opacity-40 uppercase font-black">No Active Certificate</span>
					{/if}
				</div>

				<div>
					{#if domain.ssl_active}
						<div class="flex items-center gap-2 text-success">
							<span class="text-[10px] font-bold uppercase">Locked</span>
							<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
						</div>
					{:else}
						<button 
							on:click={() => provisionSSL(domain.name)}
							disabled={processingDomain === domain.name}
							class="btn btn-xs btn-outline btn-primary px-4"
						>
							{processingDomain === domain.name ? 'Provisioning...' : 'Secure Domain'}
						</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>

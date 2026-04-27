<script lang="ts">
	import { fade, slide } from 'svelte/transition';
	import { quintOut } from 'svelte/easing';
	import { 
		ShieldAlert, 
		CheckCircle2, 
		Clock, 
		Filter, 
		ChevronLeft, 
		ChevronRight,
		ExternalLink,
		Lock
	} from 'lucide-svelte';
	import { page } from '$app/stores';

	// 🛡️ SLA Props: Strictly typed via +page.server.ts
	export let alerts: any[] = [];
	export let totalCount: number = 0;
	export let filters = {
		severity: '',
		limit: 10,
		offset: 0
	};

	let processingIds = new Set<string>();

	// 🛡️ Zero-Trust: Determine capability from the layout data
	$: canResolve = $page.data.user?.role === 'admin';

	// Pagination Calculations
	$: totalPages = Math.ceil(totalCount / filters.limit);
	$: currentPage = Math.floor(filters.offset / filters.limit) + 1;

	async function toggleResolve(alertId: string) {
		if (!canResolve) return;
		
		processingIds.add(alertId);
		processingIds = processingIds; // Trigger reactivity

		try {
			const res = await fetch(`/api/v1/alerts/${alertId}/resolve`, { method: 'POST' });
			if (res.ok) {
				// 🛡️ SLA: Smooth optimistic removal
				alerts = alerts.filter(a => a.id !== alertId);
			}
		} finally {
			processingIds.delete(alertId);
			processingIds = processingIds;
		}
	}

	const severityMap = {
		critical: 'border-red-500 bg-red-50 text-red-700',
		warning: 'border-amber-500 bg-amber-50 text-amber-700',
		info: 'border-blue-500 bg-blue-50 text-blue-700'
	};
</script>

<div class="space-y-6">
	<header class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
		<div class="flex items-center gap-3">
			<div class="p-2 bg-kari-text text-white rounded-lg shadow-lg">
				<ShieldAlert size={20} />
			</div>
			<div>
				<h2 class="text-lg font-bold text-kari-text tracking-tight">Action Center</h2>
				<p class="text-[10px] text-kari-warm-gray uppercase font-bold tracking-widest">
					{totalCount} Unresolved Operational Threats
				</p>
			</div>
		</div>

		<div class="flex items-center gap-2">
			<Filter size={14} class="text-kari-warm-gray" />
			<select 
				bind:value={filters.severity} 
				class="text-xs font-bold bg-white border-kari-warm-gray/20 rounded-md py-1.5 px-3 focus:ring-kari-teal"
			>
				<option value="">All Severities</option>
				<option value="critical">Critical Only</option>
				<option value="warning">Warnings</option>
			</select>
		</div>
	</header>

	<div class="grid gap-4">
		{#each alerts as alert (alert.id)}
			<div 
				transition:slide={{ duration: 300, easing: quintOut }}
				class="group relative p-5 border rounded-xl bg-white shadow-sm transition-all hover:shadow-md {severityMap[alert.severity]}"
			>
				<div class="flex items-start justify-between gap-4">
					<div class="space-y-2 flex-1">
						<div class="flex items-center gap-3">
							<span class="text-[10px] font-black uppercase tracking-widest px-2 py-0.5 rounded bg-white/50 border border-current">
								{alert.category}
							</span>
							<div class="flex items-center gap-1 text-[10px] font-medium opacity-60 uppercase">
								<Clock size={10} />
								{new Date(alert.created_at).toLocaleString()}
							</div>
						</div>
						
						<p class="text-sm font-semibold leading-relaxed">
							{alert.message}
						</p>
						
						{#if alert.metadata?.trace_id}
							<div class="flex items-center gap-2">
								<code class="text-[10px] font-mono bg-white/40 px-2 py-1 rounded">
									TRACE: {alert.metadata.trace_id.slice(0, 12)}...
								</code>
								{#if alert.category === 'deployment'}
									<button class="text-[10px] font-bold underline flex items-center gap-1 hover:text-kari-teal">
										<ExternalLink size={10} /> View Terminal
									</button>
								{/if}
							</div>
						{/if}
					</div>

					<div class="flex flex-col items-end gap-2">
						{#if canResolve}
							<button 
								disabled={processingIds.has(alert.id)}
								on:click={() => toggleResolve(alert.id)}
								class="flex items-center gap-2 px-4 py-2 bg-white rounded-lg text-xs font-bold border border-current transition-all hover:bg-current hover:text-white disabled:opacity-50"
							>
								{#if processingIds.has(alert.id)}
									<RefreshCw size={14} class="animate-spin" />
								{:else}
									<CheckCircle2 size={14} />
									<span>Resolve</span>
								{/if}
							</button>
						{:else}
							<div class="p-2 text-kari-warm-gray bg-gray-100 rounded-lg" title="Administrative Privileges Required">
								<Lock size={14} />
							</div>
						{/if}
					</div>
				</div>
			</div>
		{:else}
			<div class="py-24 flex flex-col items-center justify-center bg-white rounded-2xl border-2 border-dashed border-kari-warm-gray/10 text-kari-warm-gray">
				<CheckCircle2 size={48} class="opacity-10 mb-4 text-kari-teal" />
				<p class="text-sm font-bold uppercase tracking-widest">System Normalized</p>
				<p class="text-xs italic opacity-60">No high-priority alerts in current buffer.</p>
			</div>
		{/each}
	</div>

	{#if totalPages > 1}
		<footer class="flex items-center justify-center gap-6 pt-6 border-t border-kari-warm-gray/10">
			<button 
				disabled={currentPage === 1}
				on:click={() => filters.offset -= filters.limit}
				class="p-2 bg-white border border-kari-warm-gray/20 rounded-lg hover:bg-kari-teal hover:text-white transition-all disabled:opacity-20"
			>
				<ChevronLeft size={18} />
			</button>
			
			<div class="flex flex-col items-center">
				<span class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest">Page</span>
				<span class="text-sm font-mono font-bold text-kari-text">{currentPage} / {totalPages}</span>
			</div>

			<button 
				disabled={currentPage === totalPages}
				on:click={() => filters.offset += filters.limit}
				class="p-2 bg-white border border-kari-warm-gray/20 rounded-lg hover:bg-kari-teal hover:text-white transition-all disabled:opacity-20"
			>
				<ChevronRight size={18} />
			</button>
		</footer>
	{/if}
</div>

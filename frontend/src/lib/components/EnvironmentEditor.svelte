<script lang="ts">
	import { slide } from 'svelte/transition';
	import { canPerform } from '$lib/utils/auth';
	import { page } from '$app/stores';

	// 🛡️ Multi-Environment Support
	export let envGroups: Record<string, Record<string, string>> = { "production": {} };
	export let appId: string;

	let activeEnv = "production";
	let isSaving = false;
	let message = '';
	let showSecrets = false;

	// Local state for the current active environment's items
	let items = [];
	
	// Reactively update items when env or group changes
	$: {
		const currentGroup = envGroups[activeEnv] || {};
		items = Object.entries(currentGroup).map(([key, value]) => ({ 
			key, value, id: crypto.randomUUID() 
		}));
	}

	const isValidKey = (key: string) => /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(key);

	function addItem() {
		items = [...items, { key: '', value: '', id: crypto.randomUUID() }];
	}

	function handleBulkImport(e: Event) {
		const text = (e.target as HTMLTextAreaElement).value;
		const lines = text.split('\n');
		const newItems = lines.map(line => {
			const [key, ...valParts] = line.split('=');
			const value = valParts.join('=').replace(/^["']|["']$/g, '');
			if (key && isValidKey(key.trim())) {
				return { key: key.trim(), value: value.trim(), id: crypto.randomUUID() };
			}
			return null;
		}).filter(Boolean);
		items = [...items, ...newItems];
	}

	async function save() {
		if (!canPerform($page.data.user, 'apps:write')) return;
		
		const invalid = items.find(i => !isValidKey(i.key));
		if (invalid) {
			message = `Invalid key format: ${invalid.key}`;
			return;
		}

		isSaving = true;
		const payload = Object.fromEntries(items.map((i) => [i.key, i.value]));

		// 🛡️ Scoped Sync: Sending updates for a specific environment target
		const res = await fetch(`/api/v1/apps/${appId}/env/${activeEnv}`, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ env_vars: payload })
		});

		if (res.ok) {
			message = `Config for ${activeEnv} hardened.`;
			setTimeout(() => message = '', 3000);
		} else {
			message = 'Brain sync failed.';
		}
		isSaving = false;
	}
</script>

<div class="space-y-4 p-6 bg-base-200/50 rounded-xl border border-base-content/5">
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-4">
			<h3 class="text-lg font-bold">Environment</h3>
			<div class="tabs tabs-boxed bg-base-300">
				{#each Object.keys(envGroups) as env}
					<button 
						class="tab tab-sm {activeEnv === env ? 'tab-active' : ''}" 
						on:click={() => activeEnv = env}
					>{env}</button>
				{/each}
			</div>
		</div>
		<div class="flex gap-2">
			<button on:click={() => showSecrets = !showSecrets} class="btn btn-sm btn-ghost">
				{showSecrets ? '🙈 Hide' : '👁️ Show'}
			</button>
			<button on:click={addItem} class="btn btn-sm btn-circle btn-ghost border border-base-content/20">+</button>
		</div>
	</div>

	<div class="space-y-2 max-h-96 overflow-y-auto pr-2">
		{#each items as item (item.id)}
			<div transition:slide|local class="flex gap-2 group">
				<input 
					bind:value={item.key} 
					placeholder="KEY"
					class="input input-sm bg-base-300 w-1/3 font-mono text-xs {item.key && !isValidKey(item.key) ? 'text-red-500' : ''}"
				/>
				<input 
					bind:value={item.value} 
					placeholder="value"
					type={showSecrets ? "text" : "password"}
					class="input input-sm bg-base-300 flex-1 font-mono text-xs"
				/>
				<button on:click={() => items = items.filter(i => i.id !== item.id)} class="text-error opacity-20 group-hover:opacity-100">×</button>
			</div>
		{/each}
	</div>

	<textarea 
		placeholder="Bulk paste .env here (KEY=VALUE)" 
		class="textarea textarea-bordered w-full text-xs h-12 bg-base-300/50"
		on:input={handleBulkImport}
	></textarea>

	<div class="flex items-center justify-between pt-2">
		<p class="text-[10px] opacity-50 uppercase tracking-tighter">Target: {activeEnv}</p>
		<button on:click={save} disabled={isSaving} class="btn btn-sm btn-primary">
			{isSaving ? 'Syncing...' : `Save ${activeEnv}`}
		</button>
	</div>
</div>

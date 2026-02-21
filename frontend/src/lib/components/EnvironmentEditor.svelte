<script lang="ts">
	import { slide } from 'svelte/transition';
	import { canPerform } from '$lib/utils/auth';
	import { page } from '$app/stores';

	export let envVars: Record<string, string> = {};
	export let appId: string;

	let items = Object.entries(envVars).map(([key, value]) => ({ key, value, id: crypto.randomUUID() }));
	let isSaving = false;
	let message = '';

	// 🛡️ SLA: Validation Guard
	// Prevents keys that would break shell execution or systemd parsing
	const isValidKey = (key: string) => /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(key);

	function addItem() {
		items = [...items, { key: '', value: '', id: crypto.randomUUID() }];
	}

	function removeItem(id: string) {
		items = items.filter((i) => i.id !== id);
	}

	async function save() {
		if (!canPerform($page.data.user.permissions, 'apps:write')) return;
		
		// 🛡️ Designed Secure: Pre-save Validation
		const invalid = items.find(i => !isValidKey(i.key));
		if (invalid) {
			message = `Invalid key format: ${invalid.key}`;
			return;
		}

		isSaving = true;
		const payload = Object.fromEntries(items.map((i) => [i.key, i.value]));

		const res = await fetch(`/api/v1/apps/${appId}/env`, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ env_vars: payload })
		});

		if (res.ok) {
			message = 'Configuration hardened and synced.';
			setTimeout(() => message = '', 3000);
		} else {
			message = 'Failed to sync with Brain.';
		}
		isSaving = false;
	}
</script>

<div class="space-y-4 p-6 bg-base-200/50 rounded-xl border border-base-content/5">
	<div class="flex items-center justify-between">
		<div>
			<h3 class="text-lg font-bold">Environment Variables</h3>
			<p class="text-xs opacity-60">Injected into the jail at runtime via AEAD encryption.</p>
		</div>
		<button on:click={addItem} class="btn btn-sm btn-circle btn-ghost border border-base-content/20">+</button>
	</div>

	<div class="space-y-2">
		{#each items as item (item.id)}
			<div transition:slide|local class="flex gap-2 group">
				<input 
					bind:value={item.key} 
					placeholder="KEY_NAME"
					class="input input-sm bg-base-300 w-1/3 font-mono text-xs {item.key && !isValidKey(item.key) ? 'border-red-500' : ''}"
				/>
				<input 
					bind:value={item.value} 
					placeholder="value"
					type="password"
					class="input input-sm bg-base-300 flex-1 font-mono text-xs"
				/>
				<button 
					on:click={() => removeItem(item.id)}
					class="btn btn-sm btn-ghost text-red-500 opacity-0 group-hover:opacity-100 transition-opacity"
				>
					×
				</button>
			</div>
		{/each}
	</div>

	<div class="flex items-center justify-between pt-4 border-t border-base-content/5">
		<p class="text-xs font-medium {message.includes('Failed') ? 'text-red-500' : 'text-green-500'}">
			{message}
		</p>
		<button 
			on:click={save} 
			disabled={isSaving}
			class="btn btn-sm btn-primary px-6"
		>
			{isSaving ? 'Syncing...' : 'Save Configuration'}
		</button>
	</div>
</div>

<script lang="ts">
    import { fade, scale } from 'svelte/transition';
    import { 
        X, 
        Lock, 
        Eye, 
        EyeOff, 
        Plus, 
        Trash2, 
        Save, 
        ShieldCheck,
        RefreshCw,
        Info
    } from 'lucide-svelte';
    import { createEventDispatcher } from 'svelte';

    // 🛡️ Props
    export let appName: string;
    export let existingVars: Array<{ key: string, value: string }> = [];

    const dispatch = createEventDispatcher();

    // 🛡️ Local UI State
    let vars = existingVars.length > 0 ? [...existingVars] : [{ key: '', value: '' }];
    let revealed = new Set<number>();
    let isSaving = false;
    let showHint = true;

    // 🛡️ Actions
    const addRow = () => vars = [...vars, { key: '', value: '' }];
    const removeRow = (i: number) => vars = vars.filter((_, index) => index !== i);
    const toggleReveal = (i: number) => {
        if (revealed.has(i)) revealed.delete(i);
        else revealed.add(i);
        revealed = revealed; // Trigger reactivity
    };

    async function handleCommit() {
        isSaving = true;
        // Logic: Post to /api/v1/apps/{id}/env
        // The Brain will zero-out the old secrets and re-wrap the new ones.
        setTimeout(() => {
            isSaving = false;
            dispatch('close');
        }, 800);
    }
</script>

<div 
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-kari-text/40 backdrop-blur-sm"
    transition:fade={{ duration: 150 }}
>
    <div 
        class="bg-white w-full max-w-2xl rounded-2xl shadow-2xl border border-kari-warm-gray/10 overflow-hidden"
        transition:scale={{ start: 0.95, duration: 200 }}
    >
        <header class="px-6 py-4 border-b border-kari-warm-gray/10 flex items-center justify-between bg-gray-50/50">
            <div class="flex items-center gap-3">
                <div class="p-2 bg-kari-teal/10 text-kari-teal rounded-lg">
                    <Lock size={18} />
                </div>
                <div>
                    <h3 class="text-sm font-bold text-kari-text uppercase tracking-widest">Environment Variables</h3>
                    <p class="text-[10px] text-kari-warm-gray font-bold uppercase tracking-tighter">Scoped to: {appName}</p>
                </div>
            </div>
            <button on:click={() => dispatch('close')} class="text-kari-warm-gray hover:text-kari-text transition-colors">
                <X size={20} />
            </button>
        </header>

        <div class="p-6 space-y-6">
            {#if showHint}
                <div transition:slide class="bg-indigo-50 border border-indigo-100 p-4 rounded-xl flex gap-3 items-start">
                    <Info size={18} class="text-indigo-500 shrink-0 mt-0.5" />
                    <div class="text-xs text-indigo-700 leading-relaxed">
                        <p class="font-bold mb-1">Privacy-First Mode Active</p>
                        Values are encrypted at rest. We've masked them here to prevent accidental exposure. Click the eye icon to reveal a specific secret.
                    </div>
                    <button on:click={() => showHint = false} class="text-indigo-400 hover:text-indigo-600">
                        <X size={14} />
                    </button>
                </div>
            {/if}

            <div class="space-y-3 max-h-[400px] overflow-y-auto pr-2 custom-scrollbar">
                {#each vars as v, i}
                    <div class="flex gap-2 items-center group" in:fade>
                        <input 
                            type="text" 
                            bind:value={v.key}
                            placeholder="KEY_NAME"
                            class="flex-1 bg-kari-light-gray/30 border-kari-warm-gray/20 rounded-lg text-xs font-mono px-3 py-2 outline-none focus:ring-2 focus:ring-kari-teal/10 focus:border-kari-teal transition-all"
                        />
                        <div class="relative flex-[1.5]">
                            <input 
                                type={revealed.has(i) ? "text" : "password"}
                                bind:value={v.value}
                                placeholder="••••••••••••••••"
                                class="w-full bg-kari-light-gray/30 border-kari-warm-gray/20 rounded-lg text-xs font-mono px-3 py-2 pr-10 outline-none focus:ring-2 focus:ring-kari-teal/10 focus:border-kari-teal transition-all"
                            />
                            <button 
                                type="button"
                                on:click={() => toggleReveal(i)}
                                class="absolute right-3 top-1/2 -translate-y-1/2 text-kari-warm-gray hover:text-kari-teal transition-colors"
                            >
                                {#if revealed.has(i)} <EyeOff size={14} /> {:else} <Eye size={14} /> {/if}
                            </button>
                        </div>
                        <button 
                            on:click={() => removeRow(i)}
                            class="p-2 text-kari-warm-gray hover:text-red-500 opacity-0 group-hover:opacity-100 transition-all"
                        >
                            <Trash2 size={16} />
                        </button>
                    </div>
                {/each}
            </div>

            <button 
                on:click={addRow}
                class="flex items-center gap-2 text-xs font-bold text-kari-teal hover:text-[#158C85] transition-colors"
            >
                <Plus size={16} />
                Add another variable
            </button>
        </div>

        <footer class="px-6 py-4 border-t border-kari-warm-gray/10 bg-gray-50/50 flex items-center justify-between">
            <p class="text-[10px] text-kari-warm-gray italic">
                🛡️ Changes will trigger a rolling restart of the application.
            </p>
            <button 
                on:click={handleCommit}
                disabled={isSaving}
                class="flex items-center gap-2 px-6 py-2.5 bg-kari-text text-white rounded-xl text-sm font-bold hover:bg-black transition-all shadow-lg disabled:opacity-50"
            >
                {#if isSaving}
                    <RefreshCw size={16} class="animate-spin" />
                    Deploying Config...
                {:else}
                    <ShieldCheck size={16} />
                    Secure & Restart
                {/if}
            </button>
        </footer>
    </div>
</div>

<style>
    .custom-scrollbar::-webkit-scrollbar { width: 4px; }
    .custom-scrollbar::-webkit-scrollbar-thumb { background: #E4E4E7; border-radius: 10px; }
</style>

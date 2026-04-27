<script lang="ts">
    import { fade, slide, scale } from 'svelte/transition';
    import { 
        Globe, 
        ShieldCheck, 
        ShieldAlert, 
        ShieldEllipsis, 
        Plus, 
        Trash2, 
        Lock,
        AlertCircle,
        RefreshCw
    } from 'lucide-svelte';

    // 🛡️ Props: Initial data provided by +page.server.ts
    export let domains: Array<{
        id: string;
        domain_name: string;
        ssl_status: 'none' | 'pending' | 'active' | 'failed';
        created_at: string;
    }> = [];

    // Local UI State
    let newDomainName = '';
    let isSubmitting = false;
    let actionStates: Record<string, 'provisioning' | 'deleting'> = {};
    let error: string | null = null;

    // 🛡️ Validation: Zero-Trust input scrubbing
    const isValidDomain = (d: string) => /^[a-z0-9]+([\-\.]{1}[a-z0-9]+)*\.[a-z]{2,5}$/.test(d);

    async function handleAddDomain() {
        if (!isValidDomain(newDomainName) || isSubmitting) {
            error = "Please enter a valid FQDN (e.g., app.kari.io)";
            return;
        }
        
        isSubmitting = true;
        error = null;

        try {
            const response = await fetch('/api/v1/domains', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ domain_name: newDomainName.toLowerCase() })
            });

            if (!response.ok) throw new Error("Brain rejected the domain registration.");
            
            const newDomain = await response.json();
            domains = [newDomain, ...domains];
            newDomainName = '';
        } catch (e: any) {
            error = e.message;
        } finally {
            isSubmitting = false;
        }
    }

    async function handleProvisionSSL(domainId: string) {
        if (actionStates[domainId]) return;
        
        actionStates[domainId] = 'provisioning';
        error = null;

        try {
            // 📡 Trigger Go Brain to Muscle gRPC call for ACME Challenge
            const response = await fetch(`/api/v1/domains/${domainId}/ssl`, { method: 'POST' });
            if (!response.ok) throw new Error("SSL verification failed. Ensure DNS points to this Muscle.");
            
            domains = domains.map(d => d.id === domainId ? { ...d, ssl_status: 'active' } : d);
        } catch (e: any) {
            error = e.message;
            domains = domains.map(d => d.id === domainId ? { ...d, ssl_status: 'failed' } : d);
        } finally {
            delete actionStates[domainId];
            actionStates = actionStates; // Trigger reactivity
        }
    }

    async function handleDelete(domainId: string) {
        if (!confirm('This will purge the domain and all associated SSL certificates. Proceed?')) return;
        
        actionStates[domainId] = 'deleting';
        try {
            await fetch(`/api/v1/domains/${domainId}`, { method: 'DELETE' });
            domains = domains.filter(d => d.id !== domainId);
        } catch (e: any) {
            error = "Deletion failed.";
        } finally {
            delete actionStates[domainId];
            actionStates = actionStates;
        }
    }
</script>

<div class="space-y-6">
    {#if error}
        <div transition:slide class="flex items-center gap-3 p-4 bg-red-50 border border-red-200 rounded-xl text-red-700">
            <AlertCircle size={18} />
            <p class="text-sm font-medium">{error}</p>
        </div>
    {/if}

    <div class="bg-white p-5 rounded-xl border border-kari-warm-gray/10 shadow-sm">
        <form on:submit|preventDefault={handleAddDomain} class="flex gap-3">
            <div class="relative flex-1">
                <Globe class="absolute left-3 top-1/2 -translate-y-1/2 text-kari-warm-gray" size={18} />
                <input 
                    type="text" 
                    bind:value={newDomainName}
                    placeholder="app.production.io"
                    class="w-full pl-10 pr-4 py-2 bg-kari-light-gray/30 border-kari-warm-gray/20 rounded-lg text-sm font-mono focus:ring-2 focus:ring-kari-teal/20 focus:border-kari-teal outline-none transition-all"
                />
            </div>
            <button 
                type="submit"
                disabled={isSubmitting || !newDomainName}
                class="flex items-center gap-2 px-5 py-2 bg-kari-teal text-white rounded-lg text-sm font-bold hover:bg-[#158C85] disabled:opacity-50 transition-all"
            >
                {#if isSubmitting}
                    <RefreshCw size={16} class="animate-spin" />
                {:else}
                    <Plus size={16} />
                {/if}
                Add Domain
            </button>
        </form>
    </div>

    <div class="bg-white rounded-xl border border-kari-warm-gray/10 shadow-sm overflow-hidden">
        {#if domains.length === 0}
            <div class="p-16 text-center" transition:fade>
                <Globe size={48} class="mx-auto text-kari-warm-gray/20 mb-4" />
                <h3 class="text-sm font-bold text-kari-text uppercase tracking-widest">No Domains Detected</h3>
                <p class="text-xs text-kari-warm-gray mt-1">Add a domain to bridge your applications to the web.</p>
            </div>
        {:else}
            <ul class="divide-y divide-gray-50">
                {#each domains as domain (domain.id)}
                    <li class="p-5 flex items-center justify-between hover:bg-gray-50/50 transition-colors" in:slide>
                        <div class="flex items-center gap-4">
                            <div class="relative">
                                {#if domain.ssl_status === 'active'}
                                    <div class="p-2.5 bg-emerald-50 text-emerald-600 rounded-full" transition:scale>
                                        <ShieldCheck size={20} />
                                    </div>
                                {:else if actionStates[domain.id] === 'provisioning'}
                                    <div class="p-2.5 bg-amber-50 text-amber-600 rounded-full animate-pulse">
                                        <ShieldEllipsis size={20} />
                                    </div>
                                {:else if domain.ssl_status === 'failed'}
                                    <div class="p-2.5 bg-red-50 text-red-600 rounded-full">
                                        <ShieldAlert size={20} />
                                    </div>
                                {:else}
                                    <div class="p-2.5 bg-gray-100 text-gray-400 rounded-full">
                                        <Lock size={20} />
                                    </div>
                                {/if}
                            </div>

                            <div>
                                <h4 class="text-sm font-mono font-bold text-kari-text">{domain.domain_name}</h4>
                                <p class="text-[10px] text-kari-warm-gray uppercase tracking-widest font-bold mt-1">
                                    Managed since {new Date(domain.created_at).toLocaleDateString()}
                                </p>
                            </div>
                        </div>

                        <div class="flex items-center gap-3">
                            {#if domain.ssl_status !== 'active'}
                                <button 
                                    on:click={() => handleProvisionSSL(domain.id)}
                                    disabled={!!actionStates[domain.id]}
                                    class="text-[10px] font-bold uppercase tracking-widest text-kari-teal hover:underline disabled:opacity-30"
                                >
                                    Enable SSL
                                </button>
                            {/if}
                            
                            <button 
                                on:click={() => handleDelete(domain.id)}
                                class="p-2 text-kari-warm-gray hover:text-red-500 hover:bg-red-50 rounded-lg transition-all"
                                title="Purge Domain"
                            >
                                <Trash2 size={18} />
                            </button>
                        </div>
                    </li>
                {/each}
            </ul>
        {/if}
    </div>
</div>

<script lang="ts">
    import { API } from '$lib/api/client';
    
    export let domains: Array<{
        id: string;
        domain_name: string;
        ssl_status: 'none' | 'pending' | 'active' | 'failed';
        created_at: string;
    }> = [];

    // Local UI State
    let newDomainName = '';
    let isSubmitting = false;
    let actionStates: Record<string, 'idle' | 'provisioning' | 'deleting'> = {};
    let errorMessage: string | null = null;

    // ==============================================================================
    // Actions
    // ==============================================================================

    async function handleAddDomain() {
        if (!newDomainName.trim() || isSubmitting) return;
        
        isSubmitting = true;
        errorMessage = null;

        try {
            const newDomain = await API.post<any>('/api/v1/domains', { 
                domain_name: newDomainName.trim().toLowerCase() 
            });
            
            // Reactively update the list
            domains = [newDomain, ...domains];
            newDomainName = ''; // Reset input
        } catch (error: any) {
            errorMessage = error.message || 'Failed to add domain.';
        } finally {
            isSubmitting = false;
        }
    }

    async function handleProvisionSSL(domainId: string) {
        if (actionStates[domainId]) return;
        
        actionStates[domainId] = 'provisioning';
        errorMessage = null;

        // Optimistically update UI to 'pending'
        domains = domains.map(d => d.id === domainId ? { ...d, ssl_status: 'pending' } : d);

        try {
            // Trigger the Go Brain to negotiate the HTTP-01 challenge via Let's Encrypt
            await API.post(`/api/v1/domains/${domainId}/ssl`);
            
            // Update to active upon success
            domains = domains.map(d => d.id === domainId ? { ...d, ssl_status: 'active' } : d);
        } catch (error: any) {
            errorMessage = error.message || 'SSL Provisioning failed. Check DNS propagation.';
            // Revert status on failure
            domains = domains.map(d => d.id === domainId ? { ...d, ssl_status: 'failed' } : d);
        } finally {
            actionStates[domainId] = 'idle';
        }
    }

    async function handleDeleteDomain(domainId: string) {
        if (!confirm('Are you sure you want to delete this domain and its SSL certificates?')) return;
        
        actionStates[domainId] = 'deleting';
        errorMessage = null;

        try {
            await API.delete(`/api/v1/domains/${domainId}`);
            domains = domains.filter(d => d.id !== domainId);
        } catch (error: any) {
            errorMessage = error.message || 'Failed to delete domain.';
        } finally {
            actionStates[domainId] = 'idle';
        }
    }
</script>

<div class="space-y-6">
    {#if errorMessage}
        <div class="bg-red-50 border-l-4 border-red-500 p-4 rounded-md animate-fade-in-up">
            <p class="text-sm text-red-700 font-medium">{errorMessage}</p>
        </div>
    {/if}

    <div class="card p-5 bg-white">
        <form on:submit|preventDefault={handleAddDomain} class="flex gap-4">
            <div class="flex-1 relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                    <svg class="h-5 w-5 text-kari-warm-gray" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
                    </svg>
                </div>
                <input 
                    type="text" 
                    bind:value={newDomainName}
                    placeholder="api.example.com" 
                    class="block w-full pl-10 pr-3 py-2 border border-kari-warm-gray/30 rounded-md leading-5 bg-white placeholder-kari-warm-gray/50 focus:outline-none focus:ring-1 focus:ring-kari-teal focus:border-kari-teal sm:text-sm text-kari-text font-mono"
                >
            </div>
            <button 
                type="submit" 
                disabled={isSubmitting || !newDomainName.trim()}
                class="inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-kari-teal hover:bg-[#158C85] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-kari-teal transition-colors disabled:opacity-70"
            >
                {#if isSubmitting}
                    Adding...
                {:else}
                    Add Domain
                {/if}
            </button>
        </form>
    </div>

    <div class="card overflow-hidden bg-white">
        {#if domains.length === 0}
            <div class="p-12 text-center">
                <svg class="mx-auto h-12 w-12 text-kari-warm-gray/40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <h3 class="mt-2 text-sm font-medium text-kari-text">No domains configured</h3>
                <p class="mt-1 text-sm text-kari-warm-gray">Route traffic to your applications by adding a domain.</p>
            </div>
        {:else}
            <ul class="divide-y divide-kari-warm-gray/10">
                {#each domains as domain (domain.id)}
                    <li class="p-5 hover:bg-kari-light-gray/30 transition-colors flex items-center justify-between">
                        
                        <div class="flex items-center gap-4">
                            {#if domain.ssl_status === 'active'}
                                <div class="h-10 w-10 rounded-full bg-green-100 flex items-center justify-center shrink-0" title="SSL Active">
                                    <svg class="h-5 w-5 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" /></svg>
                                </div>
                            {:else if domain.ssl_status === 'pending' || actionStates[domain.id] === 'provisioning'}
                                <div class="h-10 w-10 rounded-full bg-yellow-100 flex items-center justify-center shrink-0" title="Provisioning SSL...">
                                    <svg class="h-5 w-5 text-yellow-600 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                                </div>
                            {:else if domain.ssl_status === 'failed'}
                                <div class="h-10 w-10 rounded-full bg-red-100 flex items-center justify-center shrink-0" title="SSL Provision Failed">
                                    <svg class="h-5 w-5 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
                                </div>
                            {:else}
                                <div class="h-10 w-10 rounded-full bg-kari-light-gray flex items-center justify-center shrink-0" title="No SSL">
                                    <svg class="h-5 w-5 text-kari-warm-gray" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 11V7a4 4 0 118 0m-4 8v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2z" /></svg>
                                </div>
                            {/if}

                            <div>
                                <h4 class="text-sm font-sans font-medium text-kari-text font-mono">{domain.domain_name}</h4>
                                <p class="text-xs text-kari-warm-gray mt-0.5">
                                    Added {new Date(domain.created_at).toLocaleDateString()}
                                </p>
                            </div>
                        </div>

                        <div class="flex items-center gap-3">
                            {#if domain.ssl_status !== 'active'}
                                <button 
                                    on:click={() => handleProvisionSSL(domain.id)}
                                    disabled={actionStates[domain.id] !== undefined}
                                    class="text-xs font-medium text-kari-teal hover:text-[#158C85] disabled:opacity-50 transition-colors"
                                >
                                    Enable SSL
                                </button>
                            {/if}

                            <button 
                                on:click={() => handleDeleteDomain(domain.id)}
                                disabled={actionStates[domain.id] !== undefined}
                                class="text-kari-warm-gray hover:text-red-600 disabled:opacity-50 transition-colors p-1"
                                title="Delete Domain"
                            >
                                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                            </button>
                        </div>

                    </li>
                {/each}
            </ul>
        {/if}
    </div>
</div>

<style>
    .animate-fade-in-up { animation: fadeInUp 0.3s ease-out forwards; }
    @keyframes fadeInUp {
        from { opacity: 0; transform: translateY(4px); }
        to { opacity: 1; transform: translateY(0); }
    }
</style>

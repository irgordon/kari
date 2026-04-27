<script lang="ts">
    import { API } from '$lib/api/client';
    import { slide, fade } from 'svelte/transition';
    import type { SystemAlert } from '../../../routes/(app)/dashboard/+page';

    // Props passed down from the dashboard loader
    export let alerts: SystemAlert[] = [];

    // Local state to prevent spam-clicking resolve buttons
    let resolvingIds = new Set<string>();

    // ==============================================================================
    // Actions
    // ==============================================================================

    async function resolveAlert(id: string) {
        if (resolvingIds.has(id)) return;

        // Trigger reactivity for the loading state
        resolvingIds.add(id);
        resolvingIds = resolvingIds; 

        try {
            // Tell the Go Brain to mark this alert as resolved in PostgreSQL
            await API.post(`/api/v1/audit/alerts/${id}/resolve`);
            
            // Optimistically remove it from the UI using Svelte's reactive assignment
            alerts = alerts.filter(alert => alert.id !== id);
        } catch (error) {
            console.error('Failed to resolve alert:', error);
            alert('Failed to dismiss alert. Please try again.');
        } finally {
            resolvingIds.delete(id);
            resolvingIds = resolvingIds;
        }
    }

    // ==============================================================================
    // Presentation Helpers
    // ==============================================================================

    function getSeverityStyles(severity: string) {
        switch (severity) {
            case 'critical':
                return { bg: 'bg-red-50', border: 'border-red-200', text: 'text-red-800', icon: 'text-red-500' };
            case 'warning':
                return { bg: 'bg-yellow-50', border: 'border-yellow-200', text: 'text-yellow-800', icon: 'text-yellow-500' };
            default: // info
                return { bg: 'bg-blue-50', border: 'border-blue-200', text: 'text-blue-800', icon: 'text-blue-500' };
        }
    }

    function formatCategory(category: string) {
        return category.toUpperCase().replace('_', ' ');
    }
</script>

<div class="card bg-white flex flex-col h-full border border-kari-warm-gray/20 shadow-sm overflow-hidden">
    
    <div class="px-5 py-4 border-b border-kari-warm-gray/20 bg-gray-50/50 flex items-center justify-between shrink-0">
        <div class="flex items-center gap-2">
            <svg class="h-5 w-5 text-kari-text" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
            </svg>
            <h3 class="font-sans font-semibold text-kari-text">Action Center</h3>
        </div>
        
        {#if alerts.length > 0}
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-800">
                {alerts.length} Requires Attention
            </span>
        {/if}
    </div>

    <div class="flex-1 overflow-y-auto bg-kari-light-gray/20 p-4">
        {#if alerts.length === 0}
            <div class="h-full flex flex-col items-center justify-center text-center p-6 animate-fade-in-up">
                <div class="h-12 w-12 rounded-full bg-green-100 flex items-center justify-center mb-3">
                    <svg class="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                    </svg>
                </div>
                <h4 class="text-sm font-sans font-semibold text-kari-text">All Systems Operational</h4>
                <p class="mt-1 text-xs text-kari-warm-gray">No critical alerts or warnings require your attention at this time.</p>
            </div>
        {:else}
            <div class="space-y-3">
                {#each alerts as alert (alert.id)}
                    {@const styles = getSeverityStyles(alert.severity)}
                    
                    <div 
                        transition:slide|local={{ duration: 250 }} 
                        class="rounded-lg border {styles.border} {styles.bg} p-4 shadow-sm relative overflow-hidden group"
                    >
                        <div class="flex items-start gap-3">
                            <div class="shrink-0 mt-0.5">
                                {#if alert.severity === 'critical'}
                                    <svg class="h-5 w-5 {styles.icon}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
                                {:else if alert.severity === 'warning'}
                                    <svg class="h-5 w-5 {styles.icon}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                                {:else}
                                    <svg class="h-5 w-5 {styles.icon}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                                {/if}
                            </div>
                            
                            <div class="flex-1 min-w-0">
                                <div class="flex items-center justify-between gap-2">
                                    <p class="text-xs font-bold {styles.text} font-mono tracking-wide">
                                        {formatCategory(alert.category)}
                                    </p>
                                    <p class="text-[10px] text-kari-warm-gray font-medium">
                                        {new Date(alert.created_at).toLocaleDateString()}
                                    </p>
                                </div>
                                <p class="mt-1 text-sm font-medium {styles.text} break-words leading-relaxed">
                                    {alert.message}
                                </p>
                            </div>
                        </div>

                        <div class="mt-3 pt-3 border-t {styles.border} flex justify-end">
                            <button
                                on:click={() => resolveAlert(alert.id)}
                                disabled={resolvingIds.has(alert.id)}
                                class="inline-flex items-center text-xs font-semibold {styles.text} hover:opacity-70 transition-opacity disabled:opacity-50"
                            >
                                {#if resolvingIds.has(alert.id)}
                                    <svg class="animate-spin -ml-1 mr-1.5 h-3 w-3" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                                    Resolving...
                                {:else}
                                    <svg class="mr-1 h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
                                    Mark as Resolved
                                {/if}
                            </button>
                        </div>
                    </div>
                {/each}
            </div>
        {/if}
    </div>
</div>

<style>
    .animate-fade-in-up {
        animation: fadeInUp 0.3s ease-out forwards;
    }
    @keyframes fadeInUp {
        from { opacity: 0; transform: translateY(4px); }
        to { opacity: 1; transform: translateY(0); }
    }
</style>

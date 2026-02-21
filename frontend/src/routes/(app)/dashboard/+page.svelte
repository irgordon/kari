<script lang="ts">
    import { fade, slide } from 'svelte/transition';
    import type { PageData } from './$types';
    import { 
        Activity, 
        ShieldAlert, 
        Server, 
        Cpu, 
        Database, 
        Clock,
        AlertTriangle,
        Info,
        ShieldCheck
    } from 'lucide-svelte';

    export let data: PageData;
    
    // Reactive mapping from server data
    $: stats = data.stats;
    $: alerts = data.alerts;

    // Helper: Format uptime into a human-readable string
    function formatUptime(seconds: number) {
        const days = Math.floor(seconds / (24 * 3600));
        const hrs = Math.floor((seconds % (24 * 3600)) / 3600);
        const mins = Math.floor((seconds % 3600) / 60);
        return days > 0 ? `${days}d ${hrs}h` : `${hrs}h ${mins}m`;
    }

    // Helper: Severity styling for alerts
    const alertStyles = {
        critical: 'border-red-500 bg-red-50 text-red-700 icon-red-500',
        warning: 'border-amber-500 bg-amber-50 text-amber-700 icon-amber-500',
        info: 'border-blue-500 bg-blue-50 text-blue-700 icon-blue-500'
    };
</script>

<div class="space-y-8" in:fade={{ duration: 200 }}>
    <section class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <div class="card bg-white p-6 border border-kari-warm-gray/10 shadow-sm flex items-center gap-4">
            <div class="p-3 bg-kari-teal/10 text-kari-teal rounded-lg">
                <Server size={24} />
            </div>
            <div>
                <p class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest">Active Jails</p>
                <h3 class="text-2xl font-bold text-kari-text">{stats.active_jails}</h3>
            </div>
        </div>

        <div class="card bg-white p-6 border border-kari-warm-gray/10 shadow-sm">
            <div class="flex items-center justify-between mb-2">
                <p class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest">CPU Load</p>
                <Cpu size={16} class="text-kari-warm-gray" />
            </div>
            <h3 class="text-2xl font-bold text-kari-text">{stats.cpu_usage}%</h3>
            <div class="w-full bg-gray-100 h-1.5 rounded-full mt-3 overflow-hidden">
                <div class="bg-kari-teal h-full transition-all duration-500" style="width: {stats.cpu_usage}%"></div>
            </div>
        </div>

        <div class="card bg-white p-6 border border-kari-warm-gray/10 shadow-sm">
            <div class="flex items-center justify-between mb-2">
                <p class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest">Memory</p>
                <Database size={16} class="text-kari-warm-gray" />
            </div>
            <h3 class="text-2xl font-bold text-kari-text">{stats.ram_usage}%</h3>
            <div class="w-full bg-gray-100 h-1.5 rounded-full mt-3 overflow-hidden">
                <div class="bg-indigo-500 h-full transition-all duration-500" style="width: {stats.ram_usage}%"></div>
            </div>
        </div>

        <div class="card bg-white p-6 border border-kari-warm-gray/10 shadow-sm flex items-center gap-4">
            <div class="p-3 bg-emerald-50 text-emerald-600 rounded-lg">
                <Clock size={24} />
            </div>
            <div>
                <p class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest">System Uptime</p>
                <h3 class="text-2xl font-bold text-kari-text">{formatUptime(stats.uptime_seconds)}</h3>
            </div>
        </div>
    </section>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
        <section class="lg:col-span-2 space-y-6">
            <header class="flex items-center justify-between">
                <h2 class="text-sm font-bold text-kari-text uppercase tracking-widest flex items-center gap-2">
                    <ShieldAlert size={18} class="text-kari-warm-gray" />
                    Priority Alerts
                </h2>
                <span class="text-[10px] font-mono text-kari-warm-gray">Snapshot: {new Date(data.snapshotAt).toLocaleTimeString()}</span>
            </header>

            <div class="space-y-3">
                {#each alerts as alert (alert.id)}
                    <div 
                        transition:slide
                        class="flex gap-4 p-4 rounded-lg border-l-4 shadow-sm bg-white {alertStyles[alert.severity].split(' icon')[0]}"
                    >
                        <div class="shrink-0 mt-0.5">
                            {#if alert.severity === 'critical'}
                                <AlertTriangle size={18} class="text-red-500" />
                            {:else if alert.severity === 'warning'}
                                <AlertTriangle size={18} class="text-amber-500" />
                            {:else}
                                <Info size={18} class="text-blue-500" />
                            {/if}
                        </div>
                        <div class="flex-1">
                            <div class="flex justify-between items-start">
                                <span class="text-[10px] font-bold uppercase tracking-tighter opacity-70">{alert.category}</span>
                                <span class="text-[10px] font-mono opacity-50">{new Date(alert.created_at).toLocaleTimeString()}</span>
                            </div>
                            <p class="text-sm font-medium mt-0.5">{alert.message}</p>
                        </div>
                    </div>
                {:else}
                    <div class="flex flex-col items-center justify-center py-20 bg-white rounded-xl border border-dashed border-kari-warm-gray/30 text-kari-warm-gray">
                        <ShieldCheck size={48} class="opacity-20 mb-4" />
                        <p class="text-sm font-medium">All systems operational.</p>
                        <p class="text-[10px] uppercase tracking-widest opacity-50">No unresolved high-priority threats.</p>
                    </div>
                {/each}
            </div>
        </section>

        <section class="space-y-6">
            <h2 class="text-sm font-bold text-kari-text uppercase tracking-widest flex items-center gap-2">
                <Activity size={18} class="text-kari-warm-gray" />
                Muscle Status
            </h2>

            <div class="bg-kari-text text-white rounded-xl p-6 shadow-xl space-y-6">
                <div>
                    <p class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest mb-4">Orchestration Integrity</p>
                    <div class="flex items-center gap-4">
                        <div class="relative flex h-3 w-3">
                            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-kari-teal opacity-75"></span>
                            <span class="relative inline-flex rounded-full h-3 w-3 bg-kari-teal"></span>
                        </div>
                        <span class="text-sm font-bold">Encrypted Link Active</span>
                    </div>
                </div>

                <div class="pt-4 border-t border-white/10">
                    <p class="text-[10px] text-white/50 leading-relaxed italic">
                        The Karı Muscle is currently enforcing {stats.active_jails} jail boundaries. 
                        Resource utilization is within SLA safety margins.
                    </p>
                </div>

                <a 
                    href="/applications" 
                    class="block text-center w-full py-3 bg-white/10 hover:bg-white/20 rounded-lg text-xs font-bold transition-all border border-white/10"
                >
                    Manage Infrastructure
                </a>
            </div>
        </section>
    </div>
</div>

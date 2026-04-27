<script lang="ts">
    import { onMount, onDestroy, tick } from 'svelte';
    import { fade } from 'svelte/transition';
    import { 
        Terminal, 
        Search, 
        Pause, 
        Play, 
        Trash2, 
        Download,
        Database,
        Activity,
        Filter
    } from 'lucide-svelte';

    // 🛡️ State Management
    let logs: Array<{ id: string; level: string; msg: string; service: string; ts: string }> = [];
    let filterQuery = "";
    let selectedLevel = "ALL";
    let isPaused = false;
    let logContainer: HTMLDivElement;
    let eventSource: EventSource | null = null;

    // 🛡️ SLA: Filtered view of the local buffer
    $: filteredLogs = logs.filter(log => {
        const matchesQuery = log.msg.toLowerCase().includes(filterQuery.toLowerCase());
        const matchesLevel = selectedLevel === "ALL" || log.level === selectedLevel;
        return matchesQuery && matchesLevel;
    });

    onMount(() => {
        connectStream();
    });

    function connectStream() {
        if (eventSource) eventSource.close();
        
        // 📡 Connecting to the Go Brain's internal telemetry hub
        eventSource = new EventSource('/api/v1/system/logs/stream');

        eventSource.onmessage = async (event) => {
            if (isPaused) return;

            const newLog = JSON.parse(event.data);
            
            // 🛡️ Privacy-First: Maintain a rolling buffer of 500 logs to prevent memory leaks
            logs = [newLog, ...logs].slice(0, 500);

            // Auto-scroll logic for a "Tail -f" feel
            if (logContainer && logContainer.scrollTop === 0) {
                await tick();
                logContainer.scrollTo({ top: 0, behavior: 'smooth' });
            }
        };
    }

    const getLevelStyle = (level: string) => {
        switch (level) {
            case 'ERROR': return 'text-red-500 bg-red-500/10 border-red-500/20';
            case 'WARN':  return 'text-amber-500 bg-amber-500/10 border-amber-500/20';
            case 'DEBUG': return 'text-blue-400 bg-blue-400/10 border-blue-400/20';
            default:      return 'text-emerald-500 bg-emerald-500/10 border-emerald-500/20';
        }
    };

    onDestroy(() => {
        eventSource?.close();
    });
</script>

<div class="flex flex-col h-[600px] bg-white border border-kari-warm-gray/10 rounded-2xl shadow-xl overflow-hidden">
    <header class="p-4 bg-gray-50 border-b border-kari-warm-gray/10 space-y-4">
        <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
                <div class="p-2 bg-kari-text text-white rounded-lg">
                    <Terminal size={18} />
                </div>
                <div>
                    <h3 class="text-sm font-bold text-kari-text uppercase tracking-widest">System Backplane Logs</h3>
                    <div class="flex items-center gap-2">
                        <span class="relative flex h-2 w-2">
                            {#if !isPaused}
                                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-kari-teal opacity-75"></span>
                                <span class="relative inline-flex rounded-full h-2 w-2 bg-kari-teal"></span>
                            {/if}
                        </span>
                        <span class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-tighter">
                            {isPaused ? 'Paused' : 'Live Feed'} • {filteredLogs.length} buffered
                        </span>
                    </div>
                </div>
            </div>

            <div class="flex items-center gap-2">
                <button 
                    on:click={() => isPaused = !isPaused}
                    class="p-2 hover:bg-gray-200 rounded-lg transition-colors text-kari-text"
                    title={isPaused ? "Resume" : "Pause"}
                >
                    {#if isPaused} <Play size={18} /> {:else} <Pause size={18} /> {/if}
                </button>
                <button 
                    on:click={() => logs = []}
                    class="p-2 hover:bg-red-50 hover:text-red-500 rounded-lg transition-colors text-kari-warm-gray"
                >
                    <Trash2 size={18} />
                </button>
            </div>
        </div>

        <div class="flex flex-wrap gap-3">
            <div class="relative flex-1 min-w-[200px]">
                <Search class="absolute left-3 top-1/2 -translate-y-1/2 text-kari-warm-gray" size={14} />
                <input 
                    type="text" 
                    bind:value={filterQuery}
                    placeholder="Search logs (e.g. gRPC, SQL, auth)..."
                    class="w-full pl-9 pr-4 py-1.5 bg-white border border-kari-warm-gray/20 rounded-lg text-xs outline-none focus:ring-2 focus:ring-kari-teal/20 focus:border-kari-teal transition-all"
                />
            </div>
            
            <select 
                bind:value={selectedLevel}
                class="text-xs font-bold border-kari-warm-gray/20 rounded-lg px-3 py-1.5 bg-white outline-none"
            >
                <option value="ALL">All Levels</option>
                <option value="INFO">Info</option>
                <option value="WARN">Warning</option>
                <option value="ERROR">Error</option>
                <option value="DEBUG">Debug</option>
            </select>
        </div>
    </header>

    <div 
        bind:this={logContainer}
        class="flex-1 overflow-y-auto bg-[#1A1A1C] p-2 font-mono text-[11px] selection:bg-kari-teal/30"
    >
        {#each filteredLogs as log (log.id)}
            <div 
                transition:fade={{ duration: 100 }}
                class="group flex gap-4 py-1 px-2 hover:bg-white/5 rounded transition-colors border-l-2 border-transparent hover:border-kari-teal/50"
            >
                <span class="text-white/30 shrink-0 select-none">[{new Date(log.ts).toLocaleTimeString()}]</span>
                <span class="font-black px-1.5 rounded text-[9px] h-fit mt-0.5 border {getLevelStyle(log.level)}">
                    {log.level}
                </span>
                <span class="text-indigo-400 shrink-0 font-bold tracking-tighter uppercase text-[10px] mt-0.5">
                    {log.service}:
                </span>
                <span class="text-white/90 break-all leading-relaxed">{log.msg}</span>
            </div>
        {:else}
            <div class="h-full flex flex-row items-center justify-center gap-3 text-white/20">
                <Database size={24} />
                <p class="text-sm font-bold uppercase tracking-widest italic">Listening for system events...</p>
            </div>
        {/each}
    </div>
</div>

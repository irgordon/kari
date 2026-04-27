<script lang="ts">
    import { fade } from 'svelte/transition';
    import { 
        ShieldCheck, 
        Key, 
        Zap, 
        Globe, 
        Save, 
        RefreshCw,
        Lock,
        History
    } from 'lucide-svelte';

    // 🛡️ State Management
    let loading = false;
    let success = false;

    // Config State (Usually fetched from Go Brain onMount)
    let config = {
        proxy_provider: 'nginx',
        health_interval_seconds: 15,
        master_key_last_rotated: '2026-01-15T12:00:00Z',
        audit_retention_days: 90,
        enforce_peer_creds: true
    };

    async function saveSettings() {
        loading = true;
        // Logic for POST /api/v1/system/config
        setTimeout(() => {
            loading = false;
            success = true;
            setTimeout(() => success = false, 3000);
        }, 1000);
    }
</script>

<div class="space-y-8" in:fade={{ duration: 200 }}>
    <header>
        <h2 class="text-sm font-bold text-kari-text uppercase tracking-widest flex items-center gap-2">
            <Lock size={18} class="text-kari-warm-gray" />
            Core Infrastructure Configuration
        </h2>
        <p class="text-xs text-kari-warm-gray mt-1">Manage global orchestration parameters and cryptographic boundaries.</p>
    </header>

    <div class="grid grid-cols-1 xl:grid-cols-3 gap-8">
        <div class="xl:col-span-2 space-y-6">
            <div class="bg-white rounded-xl border border-kari-warm-gray/10 shadow-sm overflow-hidden">
                <div class="px-6 py-4 border-b border-kari-warm-gray/10 bg-gray-50/50 flex items-center gap-2">
                    <Zap size={16} class="text-kari-teal" />
                    <span class="text-xs font-bold text-kari-text uppercase tracking-tight">Muscle & Proxy Defaults</span>
                </div>
                
                <div class="p-6 space-y-6">
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <label class="block">
                            <span class="text-[10px] font-bold text-kari-warm-gray uppercase mb-2 block">Default Proxy Engine</span>
                            <select bind:value={config.proxy_provider} class="w-full bg-kari-light-gray/30 border-kari-warm-gray/20 rounded-lg text-sm px-3 py-2 focus:ring-kari-teal focus:border-kari-teal">
                                <option value="nginx">Nginx (High Performance)</option>
                                <option value="apache2">Apache2 (Legacy Compatibility)</option>
                            </select>
                        </label>

                        <label class="block">
                            <span class="text-[10px] font-bold text-kari-warm-gray uppercase mb-2 block">Health Polling Interval</span>
                            <div class="flex items-center gap-3">
                                <input type="number" bind:value={config.health_interval_seconds} class="w-20 bg-kari-light-gray/30 border-kari-warm-gray/20 rounded-lg text-sm px-3 py-2" />
                                <span class="text-xs text-kari-warm-gray font-medium">seconds</span>
                            </div>
                        </label>
                    </div>

                    <div class="flex items-center justify-between p-4 bg-emerald-50 rounded-lg border border-emerald-100">
                        <div class="flex items-center gap-3">
                            <ShieldCheck size={20} class="text-emerald-600" />
                            <div>
                                <p class="text-xs font-bold text-emerald-800">Peer Credential Verification</p>
                                <p class="text-[10px] text-emerald-600 italic">Enforce UID/GID matching on all gRPC Unix Socket calls.</p>
                            </div>
                        </div>
                        <input type="checkbox" bind:checked={config.enforce_peer_creds} class="text-kari-teal rounded focus:ring-kari-teal" />
                    </div>
                </div>
            </div>

            <div class="bg-white rounded-xl border border-kari-warm-gray/10 shadow-sm overflow-hidden">
                <div class="px-6 py-4 border-b border-kari-warm-gray/10 bg-gray-50/50 flex items-center gap-2">
                    <History size={16} class="text-kari-warm-gray" />
                    <span class="text-xs font-bold text-kari-text uppercase tracking-tight">Data Retention</span>
                </div>
                <div class="p-6">
                    <label class="block max-w-xs">
                        <span class="text-[10px] font-bold text-kari-warm-gray uppercase mb-2 block">Audit Log Retention</span>
                        <div class="flex items-center gap-3">
                            <input type="number" bind:value={config.audit_retention_days} class="w-20 bg-kari-light-gray/30 border-kari-warm-gray/20 rounded-lg text-sm px-3 py-2" />
                            <span class="text-xs text-kari-warm-gray font-medium">days</span>
                        </div>
                    </label>
                </div>
            </div>
        </div>

        <div class="space-y-6">
            <div class="bg-kari-text text-white p-6 rounded-xl shadow-xl space-y-6">
                <div class="flex items-center gap-3">
                    <div class="p-2 bg-white/10 rounded-lg">
                        <Key size={20} class="text-kari-teal" />
                    </div>
                    <div>
                        <h3 class="text-sm font-bold uppercase tracking-wider">Master Key</h3>
                        <p class="text-[10px] text-kari-warm-gray uppercase font-mono">AES-256-GCM</p>
                    </div>
                </div>

                <div class="space-y-1">
                    <p class="text-[10px] text-white/50 uppercase font-bold">Last Rotated</p>
                    <p class="text-xs font-mono">{new Date(config.master_key_last_rotated).toLocaleDateString()}</p>
                </div>

                <button class="w-full py-2.5 bg-white/10 hover:bg-white/20 border border-white/10 rounded-lg text-[10px] font-bold uppercase tracking-widest transition-all flex items-center justify-center gap-2">
                    <RefreshCw size={14} />
                    Initiate Rotation
                </button>

                <p class="text-[10px] text-white/40 leading-relaxed italic border-t border-white/5 pt-4">
                    Rotating the Master Key requires re-encrypting all application secrets. 
                    Ensure the Karı Brain has sufficient compute overhead.
                </p>
            </div>

            <button 
                on:click={saveSettings}
                disabled={loading}
                class="w-full flex items-center justify-center gap-3 py-4 bg-kari-teal text-white rounded-xl font-bold shadow-lg shadow-kari-teal/20 hover:bg-[#158e87] transition-all"
            >
                {#if loading}
                    <RefreshCw size={18} class="animate-spin" />
                {:else if success}
                    <ShieldCheck size={18} />
                    Configuration Persisted
                {:else}
                    <Save size={18} />
                    Commit Changes
                {/if}
            </button>
        </div>
    </div>
</div>

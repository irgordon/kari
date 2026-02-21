import type { PageServerLoad } from './$types';
import { brainFetch } from '$lib/server/api';

// 🛡️ SLA: Explicitly define the Dashboard State
export interface SystemAlert {
    id: string;
    severity: 'info' | 'warning' | 'critical';
    category: 'ssl' | 'system' | 'security' | 'deployment';
    message: string;
    created_at: string;
}

export interface SystemStats {
    active_jails: number;
    cpu_usage: number;
    ram_usage: number;
    uptime_seconds: number;
}

/**
 * 🛡️ Kari Panel: Dashboard Orchestrator
 * Parallelizes background service calls to maximize UI responsiveness.
 */
export const load: PageServerLoad = async ({ cookies }) => {
    // 1. 🛡️ Concurrent Fetching Strategy (SLA Efficiency)
    // We fire all requests simultaneously rather than awaiting them sequentially.
    const [alertsRes, statsRes] = await Promise.all([
        brainFetch('/api/v1/audit/alerts?status=unresolved', {}, cookies).catch(() => null),
        brainFetch('/api/v1/system/stats', {}, cookies).catch(() => null)
    ]);

    // 2. 🛡️ Adaptive Parsing
    // If a service is down (null), we provide safe defaults.
    const alerts: SystemAlert[] = alertsRes?.ok ? await alertsRes.json() : [];
    const stats: SystemStats = statsRes?.ok ? await statsRes.json() : {
        active_jails: 0,
        cpu_usage: 0,
        ram_usage: 0,
        uptime_seconds: 0
    };

    return {
        alerts,
        stats,
        // 🛡️ Security Traceability: Timestamp the snapshot
        snapshotAt: new Date().toISOString()
    };
};

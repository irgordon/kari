import { writable, type Writable } from 'svelte/store';
import type { SystemStatus } from '$lib/types';

/**
 * 🛡️ SLA: SystemHealth Store
 * Provides a globally accessible, auto-polling telemetry state.
 * Implements a fail-safe 'status' indicator for Zero-Trust monitoring.
 */
interface HealthState {
    data: SystemStatus | null;
    loading: boolean;
    error: string | null;
    status: 'online' | 'degraded' | 'offline';
    lastUpdate: Date | null;
}

const initialState: HealthState = {
    data: null,
    loading: true,
    error: null,
    status: 'offline',
    lastUpdate: null
};

function createHealthStore() {
    const { subscribe, set, update }: Writable<HealthState> = writable(initialState);
    let interval: ReturnType<typeof setInterval>;

    async function poll() {
        try {
            // 📡 Fetching the cached gRPC result from the Brain's prober
            const response = await fetch('/api/v1/system/status');
            
            if (!response.ok) throw new Error('Brain-to-Muscle link severed');

            const healthData: SystemStatus = await response.json();

            update(state => ({
                ...state,
                data: healthData,
                loading: false,
                error: null,
                status: healthData.healthy ? 'online' : 'degraded',
                lastUpdate: new Date()
            }));
        } catch (err: any) {
            update(state => ({
                ...state,
                loading: false,
                error: err.message,
                status: 'offline'
            }));
        }
    }

    return {
        subscribe,
        /**
         * 🛡️ SLA: Controlled start/stop of the telemetry heartbeat.
         * Prevents background resource leaks when the user leaves the dashboard.
         */
        start: (ms: number = 15000) => {
            poll(); // Initial immediate fetch
            interval = setInterval(poll, ms);
        },
        stop: () => {
            clearInterval(interval);
            set(initialState);
        },
        // Force an immediate refresh (Tactical UI feedback)
        refresh: poll
    };
}

export const systemHealth = createHealthStore();
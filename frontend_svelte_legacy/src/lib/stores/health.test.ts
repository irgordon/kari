import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { systemHealth } from './health';

describe('systemHealth store', () => {
    beforeEach(() => {
        vi.useFakeTimers();
        vi.stubGlobal('fetch', vi.fn());
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.unstubAllGlobals();
        systemHealth.stop();
    });

    it('should have initial state', () => {
        const state = get(systemHealth);
        expect(state.data).toBeNull();
        expect(state.loading).toBe(true);
        expect(state.error).toBeNull();
        expect(state.status).toBe('offline');
        expect(state.lastUpdate).toBeNull();
    });

    it('should update state on successful poll', async () => {
        const mockData = {
            healthy: true,
            active_jails: 2,
            cpu_usage_percent: 10.5,
            memory_usage_mb: 256,
            agent_version: '1.0.0',
            uptime_seconds: 3600
        };

        (fetch as any).mockResolvedValue({
            ok: true,
            json: async () => mockData
        });

        await systemHealth.refresh();

        const state = get(systemHealth);
        expect(state.data).toEqual(mockData);
        expect(state.loading).toBe(false);
        expect(state.error).toBeNull();
        expect(state.status).toBe('online');
        expect(state.lastUpdate).toBeInstanceOf(Date);
    });

    it('should set status to degraded if healthy is false', async () => {
        const mockData = {
            healthy: false,
            active_jails: 2,
            cpu_usage_percent: 10.5,
            memory_usage_mb: 256,
            agent_version: '1.0.0',
            uptime_seconds: 3600
        };

        (fetch as any).mockResolvedValue({
            ok: true,
            json: async () => mockData
        });

        await systemHealth.refresh();

        const state = get(systemHealth);
        expect(state.status).toBe('degraded');
    });

    it('should handle fetch failure', async () => {
        (fetch as any).mockResolvedValue({
            ok: false
        });

        await systemHealth.refresh();

        const state = get(systemHealth);
        expect(state.loading).toBe(false);
        expect(state.status).toBe('offline');
        expect(state.error).toBe('Brain-to-Muscle link severed');
    });

    it('should handle fetch exception', async () => {
        (fetch as any).mockRejectedValue(new Error('Network error'));

        await systemHealth.refresh();

        const state = get(systemHealth);
        expect(state.loading).toBe(false);
        expect(state.status).toBe('offline');
        expect(state.error).toBe('Network error');
    });

    it('should start polling on start()', async () => {
        const mockData = { healthy: true };
        (fetch as any).mockResolvedValue({
            ok: true,
            json: async () => mockData
        });

        systemHealth.start(1000);

        // Initial poll
        expect(fetch).toHaveBeenCalledTimes(1);

        // Fast-forward 1 second
        await vi.advanceTimersByTimeAsync(1000);
        expect(fetch).toHaveBeenCalledTimes(2);

        // Fast-forward another 1 second
        await vi.advanceTimersByTimeAsync(1000);
        expect(fetch).toHaveBeenCalledTimes(3);
    });

    it('should stop polling and reset on stop()', async () => {
        const mockData = { healthy: true };
        (fetch as any).mockResolvedValue({
            ok: true,
            json: async () => mockData
        });

        systemHealth.start(1000);
        await vi.advanceTimersByTimeAsync(1000);
        expect(fetch).toHaveBeenCalledTimes(2);

        systemHealth.stop();

        const state = get(systemHealth);
        expect(state.data).toBeNull();
        expect(state.status).toBe('offline');

        await vi.advanceTimersByTimeAsync(1000);
        expect(fetch).toHaveBeenCalledTimes(2); // Should not have increased
    });
});

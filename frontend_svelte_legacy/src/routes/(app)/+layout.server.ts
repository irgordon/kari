import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';
import { env } from '$env/dynamic/private';

/**
 * 🛡️ Kari Panel: Server-Side Layout Orchestrator
 * * This function serves as the second line of defense. While the Hook guards the 
 * network boundary, this Layout ensures that the UI state is perfectly 
 * synchronized with the verified identity in locals.
 */
export const load: LayoutServerLoad = async ({ locals, url }) => {
    // 1. 🛑 Enforcement Check
    // If hooks.server.ts failed to verify the session, we halt here.
    if (!locals.user) {
        // We capture the intended destination for a seamless post-login experience.
        const redirectTo = url.pathname !== '/' 
            ? `?redirectTo=${encodeURIComponent(url.pathname + url.search)}` 
            : '';
            
        throw redirect(303, `/login${redirectTo}`);
    }

    // 2. 📡 Telemetry & Metadata
    // We provide the UI with system-level context that doesn't belong in the JWT
    // but is required for the "Control Panel" feel.
    return {
        // 👤 The verified identity (ID, Role, Email)
        user: locals.user,
        
        // ⚙️ System Metadata
        meta: {
            env: env.NODE_ENV || 'production',
            version: '1.0.4-stable',
            // Provide a timestamp to help the UI detect clock skew relative to the Brain
            serverTime: new Date().toISOString()
        },

        // 🛡️ Traceability
        // Useful for correlation if the user reports a UI error
        layoutId: crypto.randomUUID().split('-')[0]
    };
};

import { fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';

// 🛡️ SLA: internalApi is a helper that automatically prepends INTERNAL_API_URL 
// and attaches the user's secure JWT session cookie.
import { internalApi } from '$lib/server/api'; 

export const load: PageServerLoad = async ({ fetch }) => {
    // Fetch the initial state when the admin navigates to the settings page
    const res = await internalApi(fetch, '/api/v1/system/profile');
    
    if (!res.ok) {
        return { profile: null, error: 'Failed to load system profile.' };
    }
    
    const profile = await res.json();
    return { profile };
};

export const actions: Actions = {
    updateProfile: async ({ request, fetch }) => {
        const data = await request.formData();
        
        // 🛡️ Zero-Trust: Parse the form data into our strictly expected shape
        const payload = {
            max_memory_per_app_mb: Number(data.get('maxMemory')),
            max_cpu_percent_per_app: Number(data.get('maxCpu')),
            // 🛡️ Stability: The crucial version token for Optimistic Locking
            version: Number(data.get('version')), 
            // ... (other fields omitted for brevity)
        };

        const res = await internalApi(fetch, '/api/v1/system/profile', {
            method: 'PUT',
            body: JSON.stringify(payload)
        });

        // 🛡️ SLA: Map the Go Brain's HTTP codes to SvelteKit form states
        if (!res.ok) {
            const errorMessage = await res.text();
            
            if (res.status === 409) {
                // OCC Conflict: Another admin modified the settings
                return fail(409, { 
                    error: 'Conflict: Another administrator has updated these settings. Please refresh the page to see the latest changes.',
                    conflict: true 
                });
            }
            
            if (res.status === 400) {
                // Domain Validation Failure (e.g., MaxMemory < 128)
                return fail(400, { error: errorMessage });
            }

            return fail(500, { error: 'An unexpected internal error occurred.' });
        }

        return { success: true };
    }
};

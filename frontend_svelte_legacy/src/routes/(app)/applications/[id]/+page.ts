// frontend/src/routes/(app)/applications/[id]/+page.ts
import type { PageLoad } from './$types';
import type { Application } from '../+page'; // Reusing our SLA type contract

export const load: PageLoad = async ({ params, fetch }) => {
    // Platform agnostic: Never hardcode the API URL
    const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';
    const appId = params.id;

    try {
        // handleFetch proxy automatically attaches the HttpOnly token
        const response = await fetch(`${apiUrl}/api/v1/applications/${appId}`);

        if (!response.ok) {
            throw new Error(`Failed to load application: ${response.statusText}`);
        }

        const application: Application = await response.json();

        return {
            application
        };
    } catch (error) {
        console.error(`[Application ${appId} Load] Error:`, error);
        // Let SvelteKit's built-in Error Boundary handle the 404/500 state
        throw error;
    }
};

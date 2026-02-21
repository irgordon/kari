// frontend/src/routes/(app)/applications/+page.ts
import type { PageLoad } from './$types';

// SLA: Define the exact shape of the data we expect from the Go API.
// This prevents the UI from guessing what properties exist on the object.
export interface Application {
    id: string;
    domain_id: string;
    app_type: 'nodejs' | 'python' | 'php' | 'ruby' | 'static';
    repo_url: string;
    branch: string;
    build_command: string;
    start_command: string;
    created_at: string;
    status?: 'running' | 'failed' | 'deploying' | 'stopped'; // Extracted dynamically
}

export const load: PageLoad = async ({ fetch }) => {
    // We use Vite's env variable, falling back to localhost for development
    const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';

    try {
        // The SvelteKit `fetch` automatically attaches the HttpOnly cookie 
        // via our handleFetch hook.
        const response = await fetch(`${apiUrl}/api/v1/applications`);

        if (!response.ok) {
            // If the Go API returns a 401 Unauthorized or 403 Forbidden,
            // SvelteKit's error boundary will catch this and render the nearest +error.svelte.
            throw new Error(`Failed to load applications: ${response.statusText}`);
        }

        const applications: Application[] = await response.json();

        // Return the strictly typed data to the +page.svelte component
        return {
            applications
        };
    } catch (error) {
        console.error('[Applications Load] Error fetching data:', error);
        // Return an empty array so the UI can gracefully render the "Empty State"
        return {
            applications: []
        };
    }
};

// frontend/src/routes/(app)/+layout.server.ts

import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';

// ==============================================================================
// 1. The Server-Side Load Function (Gatekeeper)
// ==============================================================================

/**
 * The load function executes exclusively on the server during SSR, and during 
 * client-side navigation via SvelteKit's optimized fetch architecture.
 * * It relies on the `locals` object, which is securely populated by `hooks.server.ts`
 * after validating the HttpOnly `kari_access_token` against the Go API.
 */
export const load: LayoutServerLoad = async ({ locals, url }) => {
    // 1. Check if the user identity was successfully injected by the server hook
    if (!locals.user) {
        // Security: The user is not authenticated, or their token expired and could not be refreshed.
        // We immediately halt rendering and force a 303 See Other redirect to the login page.
        
        // We also attach a `redirectTo` search parameter so the login page knows where to 
        // send the user back to after they successfully authenticate.
        const redirectTo = url.pathname !== '/' ? `?redirectTo=${encodeURIComponent(url.pathname)}` : '';
        throw redirect(303, `/login${redirectTo}`);
    }

    // 2. Pass the sanitized User object down to the UI
    // By returning this object, EVERY child page (`+page.svelte`) and layout component 
    // inside the `(app)` directory can access this user data synchronously via `data.user`.
    // 
    // Note: We NEVER pass the JWT tokens themselves into this return object. 
    // The tokens remain locked in the HttpOnly cookies and the server hooks.
    return {
        user: locals.user
    };
};

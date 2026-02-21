// frontend/src/routes/login/+page.server.ts
import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals }) => {
    // If the user is already logged in, redirect them away from the login page
    if (locals.user) {
        throw redirect(303, '/dashboard');
    }
    return {};
};

export const actions: Actions = {
    default: async ({ request, cookies, url }) => {
        // 1. Extract data from the standard HTML form submission
        const data = await request.formData();
        const email = data.get('email');
        const password = data.get('password');

        if (!email || !password) {
            return fail(400, { email, missing: true, message: 'Email and password are required.' });
        }

        // 2. Call the Go API Gateway
        const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';
        
        try {
            const response = await fetch(`${apiUrl}/api/v1/auth/login`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });

            if (!response.ok) {
                const errData = await response.json().catch(() => ({}));
                return fail(response.status, { 
                    email, 
                    invalid: true, 
                    message: errData.message || 'Invalid credentials.' 
                });
            }

            // 3. SECURE BY DESIGN: Forward the HttpOnly cookies from Go to the Browser
            // The Go API responds with multiple `Set-Cookie` headers (access & refresh tokens).
            // We must extract them from the fetch response and apply them to SvelteKit's response.
            const setCookieHeaders = response.headers.getSetCookie();
            for (const header of setCookieHeaders) {
                // SvelteKit's cookies.set() requires us to parse the raw header string.
                // An easier way is to just append it directly to the outgoing headers, 
                // but SvelteKit's `cookies` API handles this nicely if we parse it, or we can use 
                // a manual header injection in the hook. For form actions, SvelteKit allows us 
                // to set the raw header string on the response directly via the RequestEvent.
                
                // Let's parse the basic attributes to use SvelteKit's built-in cookie setter securely.
                const [nameValue, ...rest] = header.split(';');
                const [name, ...valueParts] = nameValue.split('=');
                const value = valueParts.join('=');
                
                // We know the Go API already configured Secure, HttpOnly, and SameSite,
                // so we just pass the values through.
                cookies.set(name.trim(), value.trim(), {
                    path: name.trim() === 'kari_refresh_token' ? '/api/v1/auth/refresh' : '/',
                    httpOnly: true,
                    secure: true,
                    sameSite: 'strict',
                    // Calculate maxAge based on our known Go API lifespans to keep the browser in sync
                    maxAge: name.trim() === 'kari_access_token' ? 15 * 60 : 7 * 24 * 60 * 60 
                });
            }

        } catch (error) {
            console.error('[Login Action] Internal error:', error);
            return fail(500, { email, error: true, message: 'An internal server error occurred.' });
        }

        // 4. Redirect the user securely
        // Check if they were bounced here from a protected page (e.g., /applications/123)
        const redirectTo = url.searchParams.get('redirectTo');
        if (redirectTo && redirectTo.startsWith('/')) {
            throw redirect(303, redirectTo);
        }
        
        // Otherwise, send them to the main dashboard
        throw redirect(303, '/dashboard');
    }
};

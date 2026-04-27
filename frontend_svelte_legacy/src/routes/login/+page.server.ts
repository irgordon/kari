import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { env } from '$env/dynamic/private'; // 🛡️ SLA: Strict server-only env vars

export const load: PageServerLoad = async ({ locals }) => {
    // If the user is already logged in, redirect them away
    if (locals.user) {
        throw redirect(303, '/dashboard');
    }
    return {};
};

export const actions: Actions = {
    default: async ({ request, cookies, url, fetch }) => {
        const data = await request.formData();
        const email = data.get('email');
        const password = data.get('password');

        if (!email || !password) {
            return fail(400, { email, missing: true, message: 'Email and password are required.' });
        }

        // 1. 🛡️ SLA Connectivity: Use the internal Docker network
        const apiUrl = env.INTERNAL_API_URL || 'http://api:8080';
        
        try {
            // SvelteKit's provided `fetch` automatically handles internal DNS resolution
            const response = await fetch(`${apiUrl}/api/v1/auth/login`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });

            if (!response.ok) {
                const errData = await response.json().catch(() => ({}));
                // 🛡️ Friendly Front: Show clean messages from Brain, or a safe fallback
                return fail(response.status, { 
                    email, 
                    invalid: true, 
                    message: errData.message || 'Invalid credentials.' 
                });
            }

            // 2. 🛡️ Zero-Trust Cookie Mirroring
            const setCookieHeaders = response.headers.getSetCookie();
            for (const header of setCookieHeaders) {
                const [nameValue] = header.split(';');
                const [name, ...valueParts] = nameValue.split('=');
                const value = valueParts.join('=');

                cookies.set(name.trim(), value.trim(), {
                    path: '/',
                    httpOnly: true,
                    secure: true,
                    sameSite: 'strict',
                    // Agnostic TTL matching
                    maxAge: name.includes('refresh') ? 60 * 60 * 24 * 7 : 60 * 15 
                });
            }

        } catch (error) {
            // 3. 🛡️ Privacy & Compliance: Log technically, respond safely
            console.error('🚨 [Login Action] Infrastructure Error:', error);
            return fail(502, { 
                email, 
                error: true, 
                message: 'The authentication system is temporarily unreachable.' 
            });
        }

        // 4. 🛡️ Security: Hardened Open-Redirect Prevention
        const redirectTo = url.searchParams.get('redirectTo');
        if (redirectTo && redirectTo.startsWith('/') && !redirectTo.startsWith('//')) {
            throw redirect(303, redirectTo);
        }
        
        throw redirect(303, '/dashboard');
    }
};

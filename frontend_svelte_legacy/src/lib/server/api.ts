import { env } from '$env/dynamic/private';
import { building } from '$app/environment';
import { error } from '@sveltejs/kit';

/**
 * 🛡️ SLA: The Brain Proxy
 * This utility handles the complex routing between the SvelteKit Node.js 
 * server and the Go API.
 */
export async function brainFetch(
    path: string, 
    options: RequestInit = {}, 
    cookies?: { get: (name: string) => string | undefined }
) {
    // 1. Determine the Base URL
    // If we are server-side, we use the internal Docker DNS.
    // If we are building or in the browser (though this file is server-only), 
    // we fallback to the internal URL.
    const baseUrl = env.INTERNAL_API_URL || 'http://api:8080';
    const url = `${baseUrl}${path.startsWith('/') ? path : `/${path}`}`;

    // 2. 🛡️ Credential Forwarding
    // We must manually forward the 'kari_access_token' from the browser 
    // cookies to the Go API when doing SSR.
    const headers = new Headers(options.headers);
    if (cookies) {
        const token = cookies.get('kari_access_token');
        if (token) {
            headers.set('Authorization', `Bearer ${token}`);
        }
    }

    // Ensure we send JSON by default
    if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
        headers.set('Content-Type', 'application/json');
    }

    try {
        const response = await fetch(url, {
            ...options,
            headers
        });

        if (response.status === 401) {
            // 🛡️ Zero-Trust: If the Brain says unauthorized, we halt
            throw error(401, 'Session expired or invalid');
        }

        if (response.status === 403) {
            throw error(403, 'Insufficient permissions for this action');
        }

        return response;
    } catch (err: any) {
        console.error(`🚨 Brain Communication Failure [${url}]:`, err.message);
        throw error(502, 'The Brain is currently unreachable');
    }
}

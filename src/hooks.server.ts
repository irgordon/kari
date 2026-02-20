// src/hooks.server.ts
import type { Handle } from '@sveltejs/kit';
import { jwtDecode } from 'jwt-decode';

interface KariJwtPayload {
    sub: string;
    role: 'admin' | 'tenant';
    exp: number;
}

export const handle: Handle = async ({ event, resolve }) => {
    let accessToken = event.cookies.get('kari_access_token');
    const refreshToken = event.cookies.get('kari_refresh_token');

    // 1. Silent Refresh Logic
    // If the access token is missing or expired, but we have a refresh token
    if (!accessToken && refreshToken) {
        try {
            // Ask the Go API for a new token pair
            const response = await fetch('http://127.0.0.1:8080/api/v1/auth/refresh', {
                method: 'POST',
                headers: { 'Cookie': `kari_refresh_token=${refreshToken}` }
            });

            if (response.ok) {
                // The Go API sets the new HttpOnly cookies in the Set-Cookie header.
                // We must proxy these new cookies to the user's browser.
                const setCookieHeaders = response.headers.getSetCookie();
                for (const cookieStr of setCookieHeaders) {
                    event.setHeaders({ 'Set-Cookie': cookieStr });
                }
                
                // Extract the new access token to use for this current request
                const cookieParts = setCookieHeaders.find(c => c.startsWith('kari_access_token='));
                if (cookieParts) {
                    accessToken = cookieParts.split(';')[0].split('=')[1];
                }
            } else {
                // Refresh token is dead or revoked. Clear everything.
                event.cookies.delete('kari_refresh_token', { path: '/' });
            }
        } catch (err) {
            console.error("Failed to refresh token:", err);
        }
    }

    // 2. Decode the Token and Populate Locals
    if (accessToken) {
        try {
            const decoded = jwtDecode<KariJwtPayload>(accessToken);
            // Check if token is actually expired (fallback in case it wasn't caught above)
            if (decoded.exp * 1000 > Date.now()) {
                // Inject the user object into SvelteKit's event context
                event.locals.user = {
                    id: decoded.sub,
                    role: decoded.role
                };
            }
        } catch (error) {
            // Malformed token, ignore it
            event.locals.user = null;
        }
    } else {
        event.locals.user = null;
    }

    // 3. Resolve the request (Proceeds to load functions and routing)
    return await resolve(event);
};

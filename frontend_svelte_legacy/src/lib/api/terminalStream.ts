// frontend/src/lib/api/terminalStream.ts

// ==============================================================================
// 1. Type Definitions
// ==============================================================================

/**
 * LogChunk exactly mirrors the domain.LogChunk struct sent by the Go API.
 */
export interface LogChunk {
    trace_id: string;
    timestamp: string;
    level: 'stdout' | 'stderr' | 'system';
    message: string;
    is_eof: boolean;
}

/**
 * Callbacks required by the UI component to react to the stream lifecycle.
 */
export interface TerminalStreamCallbacks {
    onMessage: (chunk: LogChunk) => void;
    onError: (error: Event) => void;
    onClose: (code: number, reason: string, wasClean: boolean) => void;
    onOpen?: () => void;
}

// ==============================================================================
// 2. The WebSocket Service Class (SLA Layer)
// ==============================================================================

export class TerminalStreamService {
    private ws: WebSocket | null = null;
    private traceId: string;
    private callbacks: TerminalStreamCallbacks;
    private isManuallyClosed: boolean = false;
    private reconnectAttempts: number = 0;
    private readonly MAX_RECONNECTS = 3;

    constructor(traceId: string, callbacks: TerminalStreamCallbacks) {
        this.traceId = traceId;
        this.callbacks = callbacks;
    }

    /**
     * Determines the correct WebSocket protocol and connects to the Go API.
     */
    public connect(): void {
        this.isManuallyClosed = false;

        // Dynamically build the WebSocket URL based on the current browser location.
        // If the SvelteKit app is loaded over HTTPS, we must use WSS to prevent Mixed Content errors.
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        
        // In local development (Vite), we might need to point to the Go API port directly if not proxied.
        // For production, the API and UI are served from the same domain.
        const host = import.meta.env.VITE_API_URL 
            ? import.meta.env.VITE_API_URL.replace(/^http/, 'ws') 
            : `${protocol}//${window.location.host}`;

        const wsUrl = `${host}/api/v1/ws/deployments/${this.traceId}`;

        console.debug(`[TerminalStream] Attempting connection to ${wsUrl}`);
        
        // The browser automatically attaches the HttpOnly 'kari_access_token' cookie here
        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = this.handleOpen.bind(this);
        this.ws.onmessage = this.handleMessage.bind(this);
        this.ws.onerror = this.handleError.bind(this);
        this.ws.onclose = this.handleClose.bind(this);
    }

    /**
     * Safely tears down the connection.
     */
    public disconnect(): void {
        this.isManuallyClosed = true;
        if (this.ws) {
            console.debug('[TerminalStream] Disconnecting manually...');
            // 1000 indicates a normal closure
            this.ws.close(1000, 'User navigated away or manually closed');
            this.ws = null;
        }
    }

    // ==============================================================================
    // 3. Internal Event Handlers
    // ==============================================================================

    private handleOpen(): void {
        console.debug('[TerminalStream] Connection established.');
        this.reconnectAttempts = 0; // Reset on successful connection
        if (this.callbacks.onOpen) {
            this.callbacks.onOpen();
        }
    }

    private handleMessage(event: MessageEvent): void {
        try {
            const chunk: LogChunk = JSON.parse(event.data);
            
            // Pass the strongly typed object back to the UI component
            this.callbacks.onMessage(chunk);

            // If the backend signals EOF (End of File), the deployment is complete.
            // We can cleanly sever the connection from the client side.
            if (chunk.is_eof) {
                console.debug('[TerminalStream] EOF received. Closing connection.');
                this.disconnect();
            }
        } catch (err) {
            console.error('[TerminalStream] Failed to parse incoming WebSocket message:', err, event.data);
        }
    }

    private handleError(error: Event): void {
        console.error('[TerminalStream] WebSocket error encountered:', error);
        this.callbacks.onError(error);
    }

    private handleClose(event: CloseEvent): void {
        console.debug(`[TerminalStream] Connection closed. Code: ${event.code}, Reason: ${event.reason}`);
        
        this.callbacks.onClose(event.code, event.reason, event.wasClean);
        this.ws = null;

        // Auto-Reconnect Logic:
        // If the connection drops unexpectedly (e.g., Nginx reloads, temporary network blip)
        // and it wasn't closed manually by the user or cleanly by an EOF signal, attempt to reconnect.
        if (!this.isManuallyClosed && event.code !== 1000 && event.code !== 1008) {
            this.attemptReconnect();
        }
    }

    private attemptReconnect(): void {
        if (this.reconnectAttempts >= this.MAX_RECONNECTS) {
            console.error('[TerminalStream] Max reconnect attempts reached. Giving up.');
            // Inject a synthetic system message so the user knows the stream died
            this.callbacks.onMessage({
                trace_id: this.traceId,
                timestamp: new Date().toISOString(),
                level: 'system',
                message: '\r\n\x1b[31m[SYSTEM] Connection to server lost. Max retries exceeded.\x1b[0m\r\n',
                is_eof: true
            });
            return;
        }

        this.reconnectAttempts++;
        const delayMs = this.reconnectAttempts * 2000; // Exponential backoff: 2s, 4s, 6s...

        console.debug(`[TerminalStream] Attempting reconnect ${this.reconnectAttempts}/${this.MAX_RECONNECTS} in ${delayMs}ms...`);
        
        // Inform the user via the terminal UI
        this.callbacks.onMessage({
            trace_id: this.traceId,
            timestamp: new Date().toISOString(),
            level: 'system',
            message: `\r\n\x1b[33m[SYSTEM] Connection dropped. Reconnecting (${this.reconnectAttempts}/${this.MAX_RECONNECTS})...\x1b[0m\r\n`,
            is_eof: false
        });

        setTimeout(() => {
            this.connect();
        }, delayMs);
    }
}

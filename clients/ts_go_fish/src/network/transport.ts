export const DEFAULT_MAX_FRAME_BYTES = 64 * 1024;

export type ServerAddress =
    | { kind: "websocket"; url: string }
    | { kind: "tcp"; host: string; port: number };

export function parseServerAddress(text: string, defaultPort = 8080): ServerAddress {
    const value = text.trim();
    if (!value) {
        throw new Error("Enter the server address");
    }

    let normalized = value;
    if (/^https?:\/\//i.test(normalized)) {
        normalized = normalized.replace(/^http/i, "ws");
    }

    if (/^wss?:\/\//i.test(normalized)) {
        const url = new URL(normalized);
        if (!url.hostname) {
            throw new Error("The WebSocket address has no host");
        }
        return { kind: "websocket", url: url.toString() };
    }

    if (/^tcp:\/\//i.test(normalized)) {
        normalized = normalized.slice("tcp://".length);
    }

    let host: string;
    let portText = "";
    if (normalized.startsWith("[") && normalized.includes("]")) {
        const end = normalized.indexOf("]");
        host = normalized.slice(1, end);
        const remainder = normalized.slice(end + 1);
        if (remainder && !remainder.startsWith(":")) {
            throw new Error("Write the address as host:port");
        }
        portText = remainder.startsWith(":") ? remainder.slice(1) : "";
    } else if ((normalized.match(/:/g) ?? []).length === 1) {
        [host, portText] = normalized.split(":");
    } else if (normalized.includes(":")) {
        throw new Error("Put an IPv6 address in brackets, like [::1]:8080");
    } else {
        host = normalized;
    }

    if (!host || host.includes("/") || host.includes("?")) {
        throw new Error("Write the address as host:port or as a ws:// URL");
    }
    if (portText) {
        const port = Number(portText);
        if (!Number.isInteger(port) || port < 1 || port > 65535) {
            throw new Error("The port must be a number from 1 to 65535");
        }
        return { kind: "tcp", host, port };
    }
    return { kind: "tcp", host, port: defaultPort };
}

export class WebSocketTransport {
    private readonly socket: WebSocket;
    private readonly onOpen: (event: Event) => void;
    private readonly onData: (bytes: Uint8Array) => void;
    private readonly onClose: (event: CloseEvent) => void;
    private readonly onError: (event: Event) => void;
    private buffer = new Uint8Array();

    constructor(
        url: string,
        handlers: {
            onOpen: (event: Event) => void;
            onData: (bytes: Uint8Array) => void;
            onClose: (event: CloseEvent) => void;
            onError: (event: Event) => void;
        },
    ) {
        this.onOpen = handlers.onOpen;
        this.onData = handlers.onData;
        this.onClose = handlers.onClose;
        this.onError = handlers.onError;
        this.socket = new WebSocket(url);
        this.socket.binaryType = "arraybuffer";
        this.socket.addEventListener("open", this.onOpen);
        this.socket.addEventListener("message", this.handleMessage);
        this.socket.addEventListener("close", this.onClose);
        this.socket.addEventListener("error", this.onError);
    }

    private handleMessage = (event: MessageEvent<unknown>): void => {
        if (typeof event.data === "string") {
            this.onError(new ErrorEvent("Text frames are not supported"));
            return;
        }

        let incoming: Uint8Array;
        if (event.data instanceof ArrayBuffer) {
            incoming = new Uint8Array(event.data);
        } else if (event.data instanceof Blob) {
            void event.data.arrayBuffer().then((buffer) => {
                this.acceptBytes(new Uint8Array(buffer));
            });
            return;
        } else {
            this.onError(new ErrorEvent("Unsupported WebSocket binary payload"));
            return;
        }
        this.acceptBytes(incoming);
    };

    private acceptBytes(incoming: Uint8Array): void {
        const combined = new Uint8Array(this.buffer.length + incoming.length);
        combined.set(this.buffer);
        combined.set(incoming, this.buffer.length);
        this.buffer = combined;

        while (this.buffer.length >= 4) {
            const view = new DataView(this.buffer.buffer, this.buffer.byteOffset, this.buffer.byteLength);
            const length = view.getUint32(0, false);
            if (length > DEFAULT_MAX_FRAME_BYTES) {
                this.onError(new ErrorEvent(`Frame is over the ${DEFAULT_MAX_FRAME_BYTES}-byte limit`));
                this.close();
                return;
            }
            const frameLength = 4 + length;
            if (this.buffer.length < frameLength) {
                return;
            }

            const frame = this.buffer.slice(4, frameLength);
            this.buffer = this.buffer.slice(frameLength);
            this.onData(frame);
        }
    }

    send(payload: Uint8Array): void {
        if (payload.length > DEFAULT_MAX_FRAME_BYTES) {
            throw new Error(`Payload is over the ${DEFAULT_MAX_FRAME_BYTES}-byte limit`);
        }
        const frame = new Uint8Array(4 + payload.length);
        new DataView(frame.buffer).setUint32(0, payload.length, false);
        frame.set(payload, 4);
        this.socket.send(frame);
    }

    close(): void {
        this.socket.removeEventListener("open", this.onOpen);
        this.socket.removeEventListener("message", this.handleMessage);
        this.socket.removeEventListener("close", this.onClose);
        this.socket.removeEventListener("error", this.onError);
        if (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING) {
            this.socket.close();
        }
        this.buffer = new Uint8Array();
    }
}
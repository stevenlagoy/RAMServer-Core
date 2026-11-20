import type { NetworkEvent, Snapshot } from "../network/connection";

export type ConnectionState = {
    connected: boolean;
    serverVersion: string;
    playerId: string;
    reconnectToken: string;
    sessionId: string;
    snapshot: Snapshot | null;
    lastError: string;
};

const initialState: ConnectionState = {
    connected: false,
    serverVersion: "",
    playerId: "",
    reconnectToken: "",
    sessionId: "",
    snapshot: null,
    lastError: "",
};

export class ConnectionStore {
    private state = initialState;
    private readonly listeners = new Set<(state: ConnectionState) => void>();

    getSnapshot(): ConnectionState {
        return this.state;
    }

    subscribe(listener: (state: ConnectionState) => void): () => void {
        this.listeners.add(listener);
        listener(this.state);
        return () => this.listeners.delete(listener);
    }

    handleEvent(event: NetworkEvent): void {
        switch (event.type) {
            case "connected":
                this.state = { ...this.state, connected: true, serverVersion: event.serverVersion, lastError: "" };
                break;
            case "authenticated":
                this.state = {
                    ...this.state,
                    playerId: event.playerId,
                    reconnectToken: event.reconnectToken,
                    sessionId: event.sessionId || this.state.sessionId,
                    lastError: "",
                };
                break;
            case "state":
                this.state = {
                    ...this.state,
                    sessionId: event.snapshot.sessionId,
                    snapshot: event.snapshot,
                    lastError: "",
                };
                break;
            case "left":
                this.state = {
                    ...this.state,
                    sessionId: event.sessionId === this.state.sessionId ? "" : this.state.sessionId,
                };
                break;
            case "error":
                this.state = { ...this.state, lastError: event.message || event.code };
                break;
            case "disconnected":
                this.state = {
                    ...initialState,
                    lastError: event.reason,
                };
                break;
        }
        for (const listener of this.listeners) {
            listener(this.state);
        }
    }
}

export const connectionStore = new ConnectionStore();
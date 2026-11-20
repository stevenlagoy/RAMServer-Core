import {
    SessionStatus,
    type LegalActionsResponse,
    type ListSessionsResponse,
    type MatchHistoryResponse,
    type MatchResult,
    type ServerMessage,
} from "../protocol/gen/ramserver/v1/ramserver_pb";
import {
    createActionRequest,
    createAuthRequest,
    createConnectRequest,
    createJoinSessionRequest,
    createLeaveSessionRequest,
    createLegalActionsRequest,
    createListSessionsRequest,
    createMatchHistoryRequest,
    createSetReadyRequest,
    decodeServerMessage,
    encodeClientMessage,
    errorCodeName,
} from "../protocol/codec";
import { parseServerAddress, WebSocketTransport, type ServerAddress } from "./transport";

export const protocolVersion = "1.2";
export const goFishGameId = "gofish";
export const hostedServer = "wss://ramserver-core.cloud.run/connect";
export const localServer = "127.0.0.1:8080";

export { SessionStatus };

export class Player {
    constructor(
        public readonly playerId: string,
        public readonly displayName = "",
        public readonly connected = true,
        public readonly ready = false,
        public readonly role = "",
    ) {}
}

export class Snapshot {
    constructor(
        public readonly sessionId: string,
        public readonly gameId: string,
        public readonly status: SessionStatus,
        public readonly players: Player[] = [],
        public readonly ownerId = "",
        public readonly version = 0,
        public readonly view: Uint8Array<ArrayBufferLike> = new Uint8Array(),
        public readonly activePlayerId = "",
        public readonly playerOrder: string[] = [],
        public readonly annotations: Record<string, string> = {},
        public readonly inReplyTo = 0,
    ) {}

    get inProgress(): boolean {
        return this.status === SessionStatus.IN_PROGRESS;
    }
}

export type ConnectedEvent = { type: "connected"; serverVersion: string };
export type AuthedEvent = { type: "authenticated"; playerId: string; reconnectToken: string; sessionId: string };
export type StateEvent = { type: "state"; snapshot: Snapshot };
export type ErrorEvent = { type: "error"; code: string; message: string; inReplyTo: number; sessionId: string };
export type MatchEndedEvent = { type: "matchEnded"; match: MatchResult };
export type LegalActionsEvent = { type: "legalActions"; response: LegalActionsResponse; inReplyTo: number };
export type SessionsEvent = { type: "sessions"; response: ListSessionsResponse; inReplyTo: number };
export type HistoryEvent = { type: "history"; response: MatchHistoryResponse; inReplyTo: number };
export type LeftEvent = { type: "left"; sessionId: string };
export type DisconnectedEvent = { type: "disconnected"; reason: string };
export type NetworkEvent =
    | ConnectedEvent
    | AuthedEvent
    | StateEvent
    | ErrorEvent
    | MatchEndedEvent
    | LegalActionsEvent
    | SessionsEvent
    | HistoryEvent
    | LeftEvent
    | DisconnectedEvent;

type Listener = (event: NetworkEvent) => void;

export class NetworkClient {
    private transport: WebSocketTransport | null = null;
    private listeners = new Set<Listener>();
    private sequence = 0;
    private generation = 0;
    private connected = false;
    private reportedClose = false;

    displayName = "";
    serverVersion = "";
    playerId = "";
    reconnectToken = "";
    sessionId = "";

    subscribe(listener: Listener): () => void {
        this.listeners.add(listener);
        return () => this.listeners.delete(listener);
    }

    private emit(event: NetworkEvent): void {
        if (event.type === "connected") {
            this.connected = true;
            this.serverVersion = event.serverVersion;
        } else if (event.type === "authenticated") {
            this.playerId = event.playerId;
            this.reconnectToken = event.reconnectToken;
            if (event.sessionId) {
                this.sessionId = event.sessionId;
            }
        } else if (event.type === "state" && event.snapshot.sessionId) {
            this.sessionId = event.snapshot.sessionId;
        } else if (event.type === "left" && event.sessionId === this.sessionId) {
            this.sessionId = "";
        } else if (event.type === "disconnected") {
            this.connected = false;
            this.transport = null;
        }
        for (const listener of this.listeners) {
            listener(event);
        }
    }

    connect(addressText: string | ServerAddress, displayName: string): void {
        this.close();
        this.displayName = displayName;
        if (!this.playerId && !this.reconnectToken && typeof localStorage !== "undefined") {
            this.playerId = localStorage.getItem("ramserver.playerId") ?? "";
            this.reconnectToken = localStorage.getItem("ramserver.reconnectToken") ?? "";
        }
        this.reportedClose = false;
        this.sequence = 0;
        this.generation += 1;
        const address = typeof addressText === "string" ? parseServerAddress(addressText) : addressText;

        let url: string;
        if (address.kind === "websocket") {
            url = address.url;
        } else {
            url = `ws://${address.host}:${address.port}/connect`;
        }

        this.transport = new WebSocketTransport(url, {
            onOpen: () => {
                this.connected = true;
                this.send(createConnectRequest(this.nextSequence()));
            },
            onData: (bytes) => {
                if (bytes.length === 0) {
                    this.transport?.send(new Uint8Array());
                    return;
                }
                this.handleServerMessage(bytes);
            },
            onClose: () => {
                if (!this.reportedClose) {
                    this.reportedClose = true;
                    this.emit({ type: "disconnected", reason: "The server closed the connection" });
                }
            },
            onError: (event) => {
                const message = event instanceof ErrorEvent ? event.message : "WebSocket connection error";
                this.emit({ type: "disconnected", reason: message });
            },
        });
    }

    private nextSequence(): number {
        this.sequence += 1;
        return this.sequence;
    }

    private send(message: Parameters<typeof encodeClientMessage>[0]): void {
        if (!this.transport) {
            return;
        }
        try {
            this.transport.send(encodeClientMessage(message));
        } catch (error) {
            this.emit({
                type: "disconnected",
                reason: error instanceof Error ? error.message : "Could not send message",
            });
        }
    }

    private handleServerMessage(bytes: Uint8Array): void {
        let message: ServerMessage;
        try {
            message = decodeServerMessage(bytes);
        } catch {
            this.emit({
                type: "error",
                code: "ERROR_CODE_MALFORMED_MESSAGE",
                message: "The server sent an undecodable message",
                inReplyTo: 0,
                sessionId: "",
            });
            return;
        }

        const inReplyTo = Number(message.inReplyTo);
        switch (message.payload.case) {
            case "connectResponse":
                this.emit({
                    type: "connected",
                    serverVersion: message.payload.value.serverProtocolVersion,
                });
                this.authenticate();
                break;
            case "authResponse":
                this.emit({
                    type: "authenticated",
                    playerId: message.payload.value.playerId,
                    reconnectToken: message.payload.value.reconnectToken,
                    sessionId: message.payload.value.sessionId,
                });
                if (typeof localStorage !== "undefined") {
                    localStorage.setItem("ramserver.playerId", message.payload.value.playerId);
                    localStorage.setItem("ramserver.reconnectToken", message.payload.value.reconnectToken);
                }
                break;
            case "stateUpdate": {
                const state = message.payload.value;
                this.emit({
                    type: "state",
                    snapshot: new Snapshot(
                        state.sessionId,
                        state.gameId,
                        state.status,
                        state.players.map((player) => new Player(
                            player.playerId,
                            player.displayName,
                            player.connected,
                            player.ready,
                            player.role,
                        )),
                        state.ownerPlayerId,
                        Number(state.stateVersion),
                        state.encodedView,
                        state.activePlayerId,
                        state.playerOrder,
                        state.annotations,
                        inReplyTo,
                    ),
                });
                break;
            }
            case "errorResponse":
                this.emit({
                    type: "error",
                    code: errorCodeName(message.payload.value.code),
                    message: message.payload.value.message,
                    inReplyTo,
                    sessionId: message.payload.value.sessionId,
                });
                break;
            case "matchResult":
                this.emit({ type: "matchEnded", match: message.payload.value });
                break;
            case "legalActionsResponse":
                this.emit({
                    type: "legalActions",
                    response: message.payload.value,
                    inReplyTo,
                });
                break;
            case "listSessionsResponse":
                this.emit({
                    type: "sessions",
                    response: message.payload.value,
                    inReplyTo,
                });
                break;
            case "matchHistoryResponse":
                this.emit({
                    type: "history",
                    response: message.payload.value,
                    inReplyTo,
                });
                break;
            case "leaveSessionResponse":
                this.emit({ type: "left", sessionId: message.payload.value.sessionId });
                break;
        }
    }

    authenticate(): void {
        this.send(createAuthRequest(
            this.nextSequence(),
            this.displayName,
            this.playerId,
            this.reconnectToken,
        ));
    }

    setResumeIdentity(playerId: string, reconnectToken: string): void {
        this.playerId = playerId;
        this.reconnectToken = reconnectToken;
    }

    joinSession(sessionId = "", createNew = false, privateSession = false): void {
        this.send(createJoinSessionRequest(this.nextSequence(), {
            gameId: goFishGameId,
            sessionId,
            createNew,
            privateSession,
        }));
    }

    leaveSession(): void {
        if (!this.sessionId) {
            return;
        }
        this.send(createLeaveSessionRequest(this.nextSequence(), this.sessionId));
    }

    setReady(ready = true): void {
        if (!this.sessionId) {
            return;
        }
        this.send(createSetReadyRequest(this.nextSequence(), this.sessionId, ready));
    }

    requestLegalActions(): void {
        if (!this.sessionId) {
            return;
        }
        this.send(createLegalActionsRequest(this.nextSequence(), this.sessionId));
    }

    requestSessions(): void {
        this.send(createListSessionsRequest(this.nextSequence(), goFishGameId));
    }

    requestHistory(): void {
        if (!this.sessionId) {
            return;
        }
        this.send(createMatchHistoryRequest(this.nextSequence(), this.sessionId));
    }

    sendAction(encodedAction: Uint8Array, expectedStateVersion = 0): void {
        if (!this.sessionId) {
            return;
        }
        this.send(createActionRequest(
            this.nextSequence(),
            this.sessionId,
            encodedAction,
            expectedStateVersion,
        ));
    }

    close(): void {
        this.generation += 1;
        this.transport?.close();
        this.transport = null;
        this.connected = false;
        this.sessionId = "";
    }

    get isConnected(): boolean {
        return this.connected;
    }
}
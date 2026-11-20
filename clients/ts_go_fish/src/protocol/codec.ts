import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import {
    ActionRequestSchema,
    AuthRequestSchema,
    ClientMessageSchema,
    ConnectRequestSchema,
    ErrorCode,
    JoinSessionRequestSchema,
    LeaveSessionRequestSchema,
    LegalActionsRequestSchema,
    ListSessionsRequestSchema,
    MatchHistoryRequestSchema,
    ServerMessageSchema,
    SetReadyRequestSchema,
    type ClientMessage,
    type ServerMessage,
} from "./gen/ramserver/v1/ramserver_pb";

export const PROTOCOL_VERSION = "1.2";
export const GO_FISH_GAME_ID = "gofish";

export function encodeClientMessage(message: ClientMessage): Uint8Array {
    return toBinary(ClientMessageSchema, message);
}

export function decodeServerMessage(bytes: Uint8Array): ServerMessage {
    return fromBinary(ServerMessageSchema, bytes);
}

export function createConnectRequest(sequence: number): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "connectRequest",
            value: create(ConnectRequestSchema, {
                protocolVersion: PROTOCOL_VERSION,
                clientName: "ts-go-fish",
            }),
        },
    });
}

export function createAuthRequest(
    sequence: number,
    displayName: string,
    playerId = "",
    reconnectToken = "",
): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "authRequest",
            value: create(AuthRequestSchema, {
                displayName,
                playerId,
                reconnectToken,
            }),
        },
    });
}

export function createJoinSessionRequest(
    sequence: number,
    options: {
        gameId?: string;
        sessionId?: string;
        createNew?: boolean;
        privateSession?: boolean;
    } = {},
): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "joinSessionRequest",
            value: create(JoinSessionRequestSchema, {
                gameId: options.gameId ?? GO_FISH_GAME_ID,
                sessionId: options.sessionId ?? "",
                createNew: options.createNew ?? false,
                privateSession: options.privateSession ?? false,
            }),
        },
    });
}

export function createLeaveSessionRequest(sequence: number, sessionId: string): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "leaveSessionRequest",
            value: create(LeaveSessionRequestSchema, { sessionId }),
        },
    });
}

export function createSetReadyRequest(sequence: number, sessionId: string, ready: boolean): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "setReadyRequest",
            value: create(SetReadyRequestSchema, { sessionId, ready }),
        },
    });
}

export function createActionRequest(
    sequence: number,
    sessionId: string,
    encodedAction: Uint8Array,
    expectedStateVersion = 0,
): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "actionRequest",
            value: create(ActionRequestSchema, {
                sessionId,
                encodedAction,
                expectedStateVersion: BigInt(expectedStateVersion),
            }),
        },
    });
}

export function createLegalActionsRequest(sequence: number, sessionId: string): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "legalActionsRequest",
            value: create(LegalActionsRequestSchema, { sessionId }),
        },
    });
}

export function createListSessionsRequest(sequence: number, gameId = GO_FISH_GAME_ID): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "listSessionsRequest",
            value: create(ListSessionsRequestSchema, { gameId }),
        },
    });
}

export function createMatchHistoryRequest(sequence: number, sessionId: string): ClientMessage {
    return create(ClientMessageSchema, {
        sequence: BigInt(sequence),
        payload: {
            case: "matchHistoryRequest",
            value: create(MatchHistoryRequestSchema, { sessionId }),
        },
    });
}

export function errorCodeName(code: ErrorCode): string {
    return ErrorCode[code] ? `ERROR_CODE_${ErrorCode[code]}` : "ERROR_CODE_UNSPECIFIED";
}
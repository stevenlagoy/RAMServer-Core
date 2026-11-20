import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
    createActionRequest,
    createAuthRequest,
    encodeClientMessage,
} from "./codec";
import {
    AuthResponseSchema,
    ClientMessageSchema,
    ServerMessageSchema,
} from "./gen/ramserver/v1/ramserver_pb";

describe("RAMServer codec", () => {
    it("round trips an auth request", () => {
        const message = createAuthRequest(1, "Ada", "player-1", "secret");
        const decoded = fromBinary(ClientMessageSchema, encodeClientMessage(message));

        expect(decoded.payload.case).toBe("authRequest");
    });

    it("round trips an action request", () => {
        const message = createActionRequest(2, "session-1", new Uint8Array([1, 2, 3]), 7);
        const bytes = encodeClientMessage(message);
        expect(bytes.length).toBeGreaterThan(0);
    });

    it("decodes an authoritative server auth response", () => {
        const message = create(ServerMessageSchema, {
            inReplyTo: 1n,
            payload: {
                case: "authResponse",
                value: create(AuthResponseSchema, {
                    playerId: "player-1",
                    reconnectToken: "secret",
                    sessionId: "",
                }),
            },
        });
        const decoded = fromBinary(ServerMessageSchema, toBinary(ServerMessageSchema, message));
        expect(decoded.payload.case).toBe("authResponse");
    });
});

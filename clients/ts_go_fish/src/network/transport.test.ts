import { describe, expect, it } from "vitest";
import { parseServerAddress } from "./transport";

describe("parseServerAddress", () => {
    it("accepts local host and port", () => {
        expect(parseServerAddress("127.0.0.1:8080")).toEqual({
            kind: "tcp",
            host: "127.0.0.1",
            port: 8080,
        });
    });

    it("accepts hosted WebSocket URLs", () => {
        expect(parseServerAddress("wss://example.com/connect")).toEqual({
            kind: "websocket",
            url: "wss://example.com/connect",
        });
    });

    it("converts http addresses to WebSocket addresses", () => {
        expect(parseServerAddress("https://example.com/connect")).toEqual({
            kind: "websocket",
            url: "wss://example.com/connect",
        });
    });
});

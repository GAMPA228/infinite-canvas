import type { NextRequest } from "next/server";
import * as http from "node:http";
import * as https from "node:https";
import { Readable } from "node:stream";
import type { RequestOptions } from "node:http";

export const runtime = "nodejs";
export const maxDuration = 1800;

const PROXY_TIMEOUT_MS = 30 * 60 * 1000;

type RouteContext = {
    params: Promise<{ path: string[] }>;
};

function proxyHeaders(request: NextRequest) {
    const headers = new Headers(request.headers);
    headers.delete("host");
    headers.delete("content-length");
    headers.delete("connection");
    return headers;
}

async function proxy(request: NextRequest, context: RouteContext) {
    const { path } = await context.params;
    const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:8080";
    const target = `${apiBaseUrl.replace(/\/$/, "")}/api/${path.map(encodeURIComponent).join("/")}${request.nextUrl.search}`;
    const hasBody = request.method !== "GET" && request.method !== "HEAD";

    try {
        return await proxyWithNodeRequest(request, target, hasBody);
    } catch (error) {
        console.error("Failed to proxy", target, error);
        return Response.json({ code: 1, data: null, msg: "接口连接失败，请确认后端服务已启动" }, { status: 502 });
    }
}

function proxyWithNodeRequest(request: NextRequest, target: string, hasBody: boolean) {
    return new Promise<Response>((resolve, reject) => {
        const targetURL = new URL(target);
        const client = targetURL.protocol === "https:" ? https : http;
        const upstream = client.request(
            targetURL,
            {
                method: request.method,
                headers: Object.fromEntries(proxyHeaders(request).entries()),
                timeout: PROXY_TIMEOUT_MS,
            } as RequestOptions,
            (upstreamResponse) => {
                const headers = new Headers();
                for (const [key, value] of Object.entries(upstreamResponse.headers)) {
                    if (!value || ["content-length", "content-encoding", "transfer-encoding"].includes(key.toLowerCase())) continue;
                    const values = Array.isArray(value) ? value : [value];
                    values.forEach((item) => headers.append(key, item));
                }
                resolve(
                    new Response(Readable.toWeb(upstreamResponse) as ReadableStream, {
                        status: upstreamResponse.statusCode || 502,
                        statusText: upstreamResponse.statusMessage,
                        headers,
                    }),
                );
            },
        );

        upstream.on("timeout", () => upstream.destroy(new Error("proxy timeout")));
        upstream.on("error", reject);

        if (hasBody && request.body) {
            Readable.fromWeb(request.body).on("error", reject).pipe(upstream);
            return;
        }
        upstream.end();
    });
}

export const GET = proxy;
export const HEAD = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const OPTIONS = proxy;


import axios from "axios";

import { buildApiUrl, type AiConfig } from "@/stores/use-config-store";
import { nanoid } from "nanoid";
import { dataUrlToFile } from "@/lib/image-utils";
import { AUTH_TOKEN_KEY } from "@/services/api/auth";
import { imageToDataUrl } from "@/services/image-storage";
import type { ReferenceImage } from "@/types/image";

export type ChatCompletionMessage = {
    role: "system" | "user" | "assistant";
    content: string | Array<{ type: "text"; text: string } | { type: "image_url"; image_url: { url: string } }>;
};

type ImageApiResponse = {
    data?: Array<Record<string, unknown>>;
    error?: { message?: string };
    code?: number;
    msg?: string;
};

type ResponsesApiResponse = {
    output_text?: string;
    output?: Array<{ content?: Array<{ text?: string; type?: string }>; type?: string }>;
    choices?: Array<{ message?: { content?: string } }>;
    error?: { message?: string };
    code?: number;
    msg?: string;
};

function resolveImageDataUrl(item: Record<string, unknown>) {
    if (typeof item.b64_json === "string" && item.b64_json) {
        return `data:image/png;base64,${item.b64_json}`;
    }
    if (typeof item.url === "string" && item.url) {
        return item.url;
    }
    return null;
}

function parseImagePayload(payload: ImageApiResponse) {
    if (typeof payload.code === "number" && payload.code !== 0) {
        throw new Error(payload.msg || "请求失败");
    }
    const images =
        payload.data
            ?.map(resolveImageDataUrl)
            .filter((value): value is string => Boolean(value))
            .map((dataUrl) => ({ id: nanoid(), dataUrl })) || [];

    if (images.length === 0) {
        throw new Error("接口没有返回图片");
    }

    return images;
}

function readAxiosError(error: unknown, fallback: string) {
    if (axios.isAxiosError<{ error?: { message?: string }; msg?: string; code?: number }>(error)) {
        const responseData = error.response?.data;
        return responseData?.msg || responseData?.error?.message || (error.response?.status ? `${fallback}：${error.response.status}` : fallback);
    }
    return error instanceof Error ? error.message : fallback;
}

function parseStreamChunk(chunk: string, onDelta: (value: string) => void) {
    let deltaText = "";
    for (const eventBlock of chunk.split("\n\n")) {
        const data = eventBlock
            .split("\n")
            .find((line) => line.startsWith("data: "))
            ?.slice(6);
        if (!data || data === "[DONE]") continue;
        const delta = (JSON.parse(data) as { choices?: Array<{ delta?: { content?: string } }> }).choices?.[0]?.delta?.content || "";
        deltaText += delta;
    }
    if (deltaText) onDelta(deltaText);
}

function imageSizeByQuality(size?: string, quality?: string) {
    const value = (size || "auto").trim().toLowerCase();
    if (/^\d+x\d+$/.test(value)) return value;
    const [rawAspect] = value.split("-");
    const aspect = value === "auto" || !value ? "1:1" : rawAspect;
    const tier = quality === "high" ? "4k" : quality === "medium" ? "2k" : "1k";
    const sizes: Record<string, Record<string, string>> = {
        "1k": { "1:1": "1024x1024", "3:2": "1536x1024", "2:3": "1024x1536", "4:3": "1344x1024", "3:4": "1024x1344", "16:9": "1536x864", "9:16": "864x1536" },
        "2k": { "1:1": "2048x2048", "3:2": "2048x1365", "2:3": "1365x2048", "4:3": "2048x1536", "3:4": "1536x2048", "16:9": "2048x1152", "9:16": "1152x2048" },
        "4k": { "1:1": "4096x4096", "3:2": "3840x2560", "2:3": "2560x3840", "4:3": "3840x2880", "3:4": "2880x3840", "16:9": "3840x2160", "9:16": "2160x3840" },
    };
    return sizes[tier]?.[aspect] || size || "1024x1024";
}

function requestImageSize(config: AiConfig) {
    return config.channelMode === "remote" ? config.size || "auto" : imageSizeByQuality(config.size, config.quality);
}

function aiApiUrl(config: AiConfig, path: string) {
    return config.channelMode === "remote" ? `/api/v1${path}` : buildApiUrl(config.baseUrl, path);
}

function aiHeaders(config: AiConfig, contentType?: string) {
    return config.channelMode === "remote"
        ? {
              ...authHeader(),
              ...(contentType ? { "Content-Type": contentType } : {}),
          }
        : {
              Authorization: `Bearer ${config.apiKey}`,
              ...(contentType ? { "Content-Type": contentType } : {}),
          };
}

function authHeader() {
    if (typeof window === "undefined") return {};
    let token = "";
    try {
        token = JSON.parse(window.localStorage.getItem(AUTH_TOKEN_KEY) || "{}")?.state?.token || "";
    } catch {
        token = "";
    }
    return token ? { Authorization: `Bearer ${token}` } : {};
}

function parseResponsesText(payload: ResponsesApiResponse) {
    if (typeof payload.code === "number" && payload.code !== 0) {
        throw new Error(payload.msg || "请求失败");
    }
    if (payload.error?.message) {
        throw new Error(payload.error.message);
    }
    if (typeof payload.output_text === "string" && payload.output_text.trim()) {
        return payload.output_text.trim();
    }
    const outputText = payload.output?.flatMap((item) => item.content || []).map((item) => item.text || "").join("").trim();
    if (outputText) return outputText;
    const chatText = payload.choices?.[0]?.message?.content?.trim();
    if (chatText) return chatText;
    throw new Error("接口没有返回优化结果");
}

export async function requestGeneration(config: AiConfig, prompt: string) {
    const n = Math.max(1, Math.min(15, Math.floor(Math.abs(Number(config.count)) || 1)));
    try {
        const response = await axios.post<ImageApiResponse>(
            aiApiUrl(config, "/images/generations"),
            {
                model: config.model,
                prompt,
                n,
                quality: config.quality || undefined,
                size: requestImageSize(config),
                response_format: "b64_json",
            },
            {
                headers: aiHeaders(config, "application/json"),
            },
        );
        return parseImagePayload(response.data);
    } catch (error) {
        throw new Error(readAxiosError(error, "请求失败"));
    }
}

export async function requestEdit(config: AiConfig, prompt: string, references: ReferenceImage[]) {
    const n = Math.max(1, Math.min(15, Math.floor(Math.abs(Number(config.count)) || 1)));
    const formData = new FormData();
    formData.set("model", config.model);
    formData.set("prompt", prompt);
    formData.set("n", String(n));
    formData.set("response_format", "b64_json");
    if (config.quality) {
        formData.set("quality", config.quality);
    }
    formData.set("size", requestImageSize(config));
    const files = await Promise.all(references.map(async (image) => dataUrlToFile({ ...image, dataUrl: await imageToDataUrl(image) })));
    files.forEach((file) => formData.append("image", file));

    try {
        const response = await axios.post<ImageApiResponse>(aiApiUrl(config, "/images/edits"), formData, { headers: aiHeaders(config) });
        return parseImagePayload(response.data);
    } catch (error) {
        throw new Error(readAxiosError(error, "请求失败"));
    }
}

export async function requestImageQuestion(config: AiConfig, messages: ChatCompletionMessage[], onDelta: (text: string) => void) {
    let buffer = "";
    let answer = "";
    let processedLength = 0;

    try {
        const response = await axios.post(
            aiApiUrl(config, "/chat/completions"),
            {
                model: config.model,
                messages,
                stream: true,
            },
            {
                headers: {
                    ...aiHeaders(config, "application/json"),
                } as Record<string, string>,
                responseType: "text",
                onDownloadProgress: (event) => {
                    const responseText = String(event.event?.target?.responseText || "");
                    const nextText = responseText.slice(processedLength);
                    processedLength = responseText.length;
                    buffer += nextText;
                    const chunks = buffer.split("\n\n");
                    buffer = chunks.pop() || "";
                    for (const chunk of chunks) {
                        parseStreamChunk(chunk, (delta) => {
                            answer += delta;
                            onDelta(answer);
                        });
                    }
                },
            },
        );
        if (typeof response.data === "object" && response.data && "code" in response.data && (response.data as { code?: number; msg?: string }).code !== 0) {
            throw new Error((response.data as { msg?: string }).msg || "请求失败");
        }
        if (typeof response.data === "string") {
            let apiError = "";
            try {
                const payload = JSON.parse(response.data) as { code?: number; msg?: string };
                if (typeof payload.code === "number" && payload.code !== 0) {
                    apiError = payload.msg || "请求失败";
                }
            } catch {
                // ignore plain text stream content
            }
            if (apiError) throw new Error(apiError);
        }
        if (buffer) {
            parseStreamChunk(buffer, (delta) => {
                answer += delta;
                onDelta(answer);
            });
        }
    } catch (error) {
        throw new Error(readAxiosError(error, "请求失败"));
    }
    return answer || "没有返回内容";
}

export async function requestPromptOptimization(config: AiConfig, prompt: string, model: string) {
    try {
        const response = await axios.post<ResponsesApiResponse>(
            aiApiUrl(config, "/responses"),
            {
                model,
                instructions: config.systemPrompt || undefined,
                input: prompt,
            },
            {
                headers: aiHeaders(config, "application/json"),
            },
        );
        return parseResponsesText(response.data);
    } catch (error) {
        throw new Error(readAxiosError(error, "提示词优化失败"));
    }
}

export async function fetchImageModels(config: AiConfig) {
    if (config.channelMode === "remote") return config.models;
    try {
        const response = await axios.get<{ data?: Array<{ id?: string }>; error?: { message?: string } }>(buildApiUrl(config.baseUrl, "/models"), {
            headers: {
                Authorization: `Bearer ${config.apiKey}`,
            },
        });
        return (response.data.data || [])
            .map((model) => model.id)
            .filter((id): id is string => Boolean(id))
            .sort((a, b) => a.localeCompare(b));
    } catch (error) {
        throw new Error(readAxiosError(error, "读取模型失败"));
    }
}

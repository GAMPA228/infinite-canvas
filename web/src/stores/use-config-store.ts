"use client";

import { useMemo } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";

import { apiGet } from "@/services/api/request";
import type { AdminPublicSettings } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

export type AiConfig = {
    channelMode: "remote" | "local";
    baseUrl: string;
    apiKey: string;
    model: string;
    imageModel: string;
    videoModel: string;
    textModel: string;
    videoSeconds: string;
    vquality: string;
    videoGenerateAudio: string;
    videoWatermark: string;
    systemPrompt: string;
    models: string[];
    quality: string;
    size: string;
    count: string;
};

export const CONFIG_STORE_KEY = "infinite-canvas:ai_config_store";
let publicSettingsRequestSeq = 0;

export const defaultConfig: AiConfig = {
    channelMode: "local",
    baseUrl: "https://api.openai.com",
    apiKey: "",
    model: "gpt-image-2",
    imageModel: "gpt-image-2",
    videoModel: "sora-2",
    textModel: "gpt-5.5",
    videoSeconds: "6",
    vquality: "auto",
    videoGenerateAudio: "true",
    videoWatermark: "false",
    systemPrompt: "",
    models: [],
    quality: "auto",
    size: "1:1",
    count: "1",
};

type ConfigStore = {
    config: AiConfig;
    publicSettings: AdminPublicSettings | null;
    isPublicSettingsLoading: boolean;
    isConfigOpen: boolean;
    shouldPromptContinue: boolean;
    updateConfig: <K extends keyof AiConfig>(key: K, value: AiConfig[K]) => void;
    loadPublicSettings: () => Promise<void>;
    isAiConfigReady: (config: AiConfig, model: string) => boolean;
    openConfigDialog: (shouldPromptContinue?: boolean) => void;
    setConfigDialogOpen: (isOpen: boolean) => void;
    clearPromptContinue: () => void;
};

function resolveEffectiveConfig(config: AiConfig, modelChannel: AdminPublicSettings["modelChannel"] | null, isAdmin: boolean) {
    const channelMode = isAdmin && modelChannel?.allowCustomChannel ? config.channelMode : "remote";
    if (channelMode === "local" || !modelChannel) return { ...config, channelMode };
    const models = modelChannel.availableModels || [];
    const fallbackModel = models[0] || modelChannel.defaultModel || config.model;
    const pickModel = (saved: string, preferred: string) => {
        if (!models.length) return preferred || saved || fallbackModel;
        if (models.includes(saved)) return saved;
        if (models.includes(preferred)) return preferred;
        return fallbackModel;
    };
    return {
        ...config,
        channelMode,
        models,
        model: pickModel(config.model, modelChannel.defaultModel),
        imageModel: pickModel(config.imageModel, modelChannel.defaultImageModel || modelChannel.defaultModel),
        videoModel: pickModel(config.videoModel, modelChannel.defaultVideoModel || modelChannel.defaultModel || "sora-2"),
        textModel: pickModel(config.textModel, modelChannel.defaultTextModel || modelChannel.defaultModel),
        systemPrompt: modelChannel.systemPrompt,
    };
}

function isAiConfigReady(config: AiConfig, model: string) {
    return Boolean(model.trim()) && (config.channelMode === "remote" || Boolean(config.baseUrl.trim() && config.apiKey.trim()));
}

export const useConfigStore = create<ConfigStore>()(
    persist(
        (set, get) => ({
            config: defaultConfig,
            publicSettings: null,
            isPublicSettingsLoading: false,
            isConfigOpen: false,
            shouldPromptContinue: false,
            updateConfig: (key, value) =>
                set((state) => ({
                    config: {
                        ...state.config,
                        [key]: value,
                    },
                })),
            loadPublicSettings: async () => {
                const requestSeq = ++publicSettingsRequestSeq;
                set({ isPublicSettingsLoading: true });
                try {
                    const publicSettings = await apiGet<AdminPublicSettings>("/api/settings");
                    if (requestSeq === publicSettingsRequestSeq) {
                        set({ publicSettings });
                    }
                } finally {
                    if (requestSeq === publicSettingsRequestSeq) {
                        set({ isPublicSettingsLoading: false });
                    }
                }
            },
            isAiConfigReady: (config, model) => isAiConfigReady(config, model),
            openConfigDialog: (shouldPromptContinue = false) => set({ isConfigOpen: true, shouldPromptContinue }),
            setConfigDialogOpen: (isConfigOpen) => set({ isConfigOpen }),
            clearPromptContinue: () => set({ shouldPromptContinue: false }),
        }),
        {
            name: CONFIG_STORE_KEY,
            partialize: (state) => ({ config: state.config }),
            merge: (persisted, current) => {
                const config = { ...defaultConfig, ...((persisted as Partial<ConfigStore>).config || {}) };
                return { ...current, config: { ...config, channelMode: config.channelMode || "remote", imageModel: config.imageModel || config.model, videoModel: config.videoModel || "sora-2", textModel: config.textModel || config.model, videoSeconds: config.videoSeconds || "6", vquality: config.vquality || "auto", videoGenerateAudio: config.videoGenerateAudio || "true", videoWatermark: config.videoWatermark || "false" } };
            },
        },
    ),
);

export function useEffectiveConfig() {
    const config = useConfigStore((state) => state.config);
    const modelChannel = useConfigStore((state) => state.publicSettings?.modelChannel || null);
    const isAdmin = useUserStore((state) => state.user?.role === "admin");
    return useMemo(() => resolveEffectiveConfig(config, modelChannel, isAdmin), [config, modelChannel, isAdmin]);
}

export function buildApiUrl(baseUrl: string, path: string) {
    const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
    const apiBaseUrl = normalizedBaseUrl.endsWith("/v1") ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`;
    return `${apiBaseUrl}${path}`;
}

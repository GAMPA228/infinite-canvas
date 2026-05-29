"use client";

import { type ReactNode } from "react";
import { Settings2 } from "lucide-react";
import { Button, ConfigProvider, Popover } from "antd";

import { canvasThemes } from "@/lib/canvas-theme";
import { canUsePremiumImageQuality, isPremiumImageQuality } from "@/lib/user-plan";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";
import type { AiConfig } from "@/stores/use-config-store";

const qualityOptions = [
    { value: "auto", label: "自动(1K)" },
    { value: "low", label: "1K" },
    { value: "medium", label: "2K" },
    { value: "high", label: "4K" },
];
const aspectOptions = [
    { value: "auto", label: "auto", width: 0, height: 0, icon: "auto" },
    { value: "1:1", label: "1:1", width: 1024, height: 1024, icon: "square" },
    { value: "3:2", label: "3:2", width: 1536, height: 1024, icon: "landscape" },
    { value: "2:3", label: "2:3", width: 1024, height: 1536, icon: "portrait" },
    { value: "4:3", label: "4:3", width: 1344, height: 1024, icon: "landscape" },
    { value: "3:4", label: "3:4", width: 1024, height: 1344, icon: "portrait" },
    { value: "16:9", label: "16:9", width: 1536, height: 864, icon: "landscape" },
    { value: "9:16", label: "9:16", width: 864, height: 1536, icon: "portrait" },
];

type CanvasImageSettingsPopoverProps = {
    config: AiConfig;
    onConfigChange: (key: keyof AiConfig, value: string) => void;
    onMissingConfig?: () => void;
    onOpenChange?: (open: boolean) => void;
    buttonClassName?: string;
    getPopupContainer?: (triggerNode: HTMLElement) => HTMLElement;
    placement?: "topLeft" | "top" | "topRight" | "bottomLeft" | "bottom" | "bottomRight";
    onRequireUpgrade?: () => void;
};

export function CanvasImageSettingsPopover({ config, onConfigChange, onOpenChange, buttonClassName, getPopupContainer, placement = "topLeft", onRequireUpgrade }: CanvasImageSettingsPopoverProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const userRole = useUserStore((state) => state.user?.role);
    const canUsePremiumQuality = canUsePremiumImageQuality(userRole);
    const quality = config.quality || "auto";
    const count = Math.max(1, Math.min(15, Math.floor(Math.abs(Number(config.count)) || 1)));
    const activeSize = config.size || "auto";
    const selectedAspect = aspectOptions.find((item) => item.value === activeSize);
    const dimensions = readSizeDimensions(activeSize, selectedAspect || aspectOptions[0], quality);
    const selectAspect = (value: string) => onConfigChange("size", value);
    const updateDimension = (key: "width" | "height", value: number | null) => {
        const next = Math.max(1, Math.floor(value || dimensions[key] || 1024));
        onConfigChange("size", `${key === "width" ? next : dimensions.width}x${key === "height" ? next : dimensions.height}`);
    };
    const changeQuality = (value: string) => {
        if (!canUsePremiumQuality && isPremiumImageQuality(value)) {
            onRequireUpgrade?.();
            return;
        }
        onConfigChange("quality", value);
    };

    return (
        <Popover
            trigger="click"
            placement={placement}
            arrow={false}
            overlayClassName="canvas-image-settings-popover"
            color={theme.toolbar.panel}
            getPopupContainer={getPopupContainer || ((triggerNode) => triggerNode.parentElement || document.body)}
            onOpenChange={onOpenChange}
            content={
                <CanvasImageSettingsTheme theme={theme}>
                    <div className="w-[360px] space-y-5 rounded-3xl px-1 py-0.5" style={{ color: theme.node.text }} onMouseDown={(event) => event.stopPropagation()}>
                        <div className="text-xl font-semibold">图像设置</div>
                        <div className="space-y-3">
                            <SettingTitle color={theme.node.muted}>质量</SettingTitle>
                            <div className="grid grid-cols-4 gap-3">
                                {qualityOptions.map((item) => (
                                    <OptionPill key={item.value} selected={quality === item.value} locked={!canUsePremiumQuality && isPremiumImageQuality(item.value)} theme={theme} onClick={() => changeQuality(item.value)}>
                                        {item.label}
                                    </OptionPill>
                                ))}
                            </div>
                        </div>
                        <div className="space-y-3">
                            <SettingTitle color={theme.node.muted}>尺寸</SettingTitle>
                            <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-3">
                                <DimensionInput prefix="W" value={dimensions.width} disabled={activeSize === "auto"} theme={theme} onChange={(value) => updateDimension("width", value)} />
                                <span className="text-lg opacity-45">↔</span>
                                <DimensionInput prefix="H" value={dimensions.height} disabled={activeSize === "auto"} theme={theme} onChange={(value) => updateDimension("height", value)} />
                            </div>
                        </div>
                        <div className="space-y-3">
                            <SettingTitle color={theme.node.muted}>宽高比</SettingTitle>
                            <div className="grid grid-cols-4 gap-3">
                                {aspectOptions.map((item) => (
                                    <button
                                        key={item.value}
                                        type="button"
                                        className="flex h-[86px] cursor-pointer flex-col items-center justify-center gap-2 rounded-2xl border bg-transparent text-sm transition hover:opacity-80"
                                        style={{ borderColor: selectedAspect?.value === item.value ? theme.node.text : theme.node.stroke, background: "transparent", color: theme.node.text }}
                                        onMouseDown={(event) => event.stopPropagation()}
                                        onClick={() => selectAspect(item.value)}
                                    >
                                        <AspectIcon type={item.icon} width={item.width} height={item.height} color={theme.node.text} />
                                        <span>{item.label}</span>
                                    </button>
                                ))}
                            </div>
                        </div>
                        <div className="space-y-3">
                            <SettingTitle color={theme.node.muted}>生成张数</SettingTitle>
                            <div className="grid grid-cols-4 gap-3">
                                {Array.from({ length: 10 }, (_, index) => index + 1).map((value) => (
                                    <OptionPill key={value} selected={count === value} theme={theme} onClick={() => onConfigChange("count", String(value))}>
                                        {value} 张
                                    </OptionPill>
                                ))}
                                <CountInput value={count} theme={theme} onChange={(value) => onConfigChange("count", String(value || 1))} />
                            </div>
                        </div>
                    </div>
                </CanvasImageSettingsTheme>
            }
        >
            <Button size="small" type="text" className={buttonClassName || "!h-8 !max-w-[180px] !justify-start !rounded-full !px-2.5"} style={{ background: theme.node.fill, color: theme.node.text }} icon={<Settings2 className="size-3.5" />}>
                <span className="truncate">
                    {qualityLabel(quality)} · {selectedAspect?.label || activeSize} · {count} 张
                </span>
            </Button>
        </Popover>
    );
}

export function CanvasImageSettingsTheme({ theme, children }: { theme: (typeof canvasThemes)[keyof typeof canvasThemes]; children: ReactNode }) {
    return (
        <ConfigProvider
            theme={{
                token: { colorBgContainer: theme.toolbar.panel, colorBgElevated: theme.toolbar.panel, colorBorder: theme.node.stroke, colorPrimary: theme.node.activeStroke, colorText: theme.node.text, colorTextLightSolid: theme.node.panel },
                components: { Button: { defaultBg: theme.toolbar.panel, defaultBorderColor: theme.node.stroke, defaultColor: theme.node.text } },
            }}
        >
            {children}
        </ConfigProvider>
    );
}

function OptionPill({ selected, locked, theme, onClick, children }: { selected: boolean; locked?: boolean; theme: (typeof canvasThemes)[keyof typeof canvasThemes]; onClick: () => void; children: ReactNode }) {
    return (
        <button
            type="button"
            className="h-10 cursor-pointer rounded-full border px-2 text-sm transition hover:opacity-80"
            style={{ background: "transparent", borderColor: selected ? theme.node.text : theme.node.stroke, color: theme.node.text, opacity: locked ? 0.45 : 1 }}
            onMouseDown={(event) => event.stopPropagation()}
            onClick={onClick}
        >
            {children}
        </button>
    );
}

function DimensionInput({ prefix, value, disabled, theme, onChange }: { prefix: string; value: number; disabled: boolean; theme: (typeof canvasThemes)[keyof typeof canvasThemes]; onChange: (value: number | null) => void }) {
    return (
        <label className="flex h-10 overflow-hidden rounded-xl text-sm" style={{ background: theme.node.fill, color: theme.node.text, opacity: disabled ? 0.55 : 1 }}>
            <span className="grid w-10 place-items-center" style={{ color: theme.node.muted }}>
                {prefix}
            </span>
            <input
                type="number"
                min={1}
                disabled={disabled}
                className="min-w-0 flex-1 bg-transparent px-2 outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                value={value || ""}
                onChange={(event) => onChange(Number(event.target.value) || null)}
                onMouseDown={(event) => event.stopPropagation()}
            />
        </label>
    );
}

function CountInput({ value, theme, onChange }: { value: number; theme: (typeof canvasThemes)[keyof typeof canvasThemes]; onChange: (value: number | null) => void }) {
    return (
        <label className="col-span-2 flex h-10 overflow-hidden rounded-full border text-sm" style={{ borderColor: theme.node.stroke, color: theme.node.text }}>
            <input
                type="number"
                min={1}
                max={15}
                className="min-w-0 flex-1 bg-transparent px-3 text-center outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                style={{ color: theme.node.text, WebkitTextFillColor: theme.node.text }}
                value={value || ""}
                onChange={(event) => onChange(Number(event.target.value) || null)}
                onMouseDown={(event) => event.stopPropagation()}
            />
        </label>
    );
}

function AspectIcon({ type, width, height, color }: { type: string; width: number; height: number; color: string }) {
    if (type === "auto") return null;
    const ratio = width / Math.max(1, height);
    const boxWidth = ratio >= 1 ? 28 : Math.max(12, 28 * ratio);
    const boxHeight = ratio >= 1 ? Math.max(12, 28 / ratio) : 28;
    return (
        <span className="grid h-8 w-10 place-items-center">
            <span className="border-2" style={{ width: boxWidth, height: boxHeight, borderColor: color }} />
        </span>
    );
}

function SettingTitle({ children, color }: { children: string; color: string }) {
    return (
        <div className="text-xs font-medium" style={{ color }}>
            {children}
        </div>
    );
}

function qualityLabel(value: string) {
    return qualityOptions.find((item) => item.value === value)?.label || value;
}

function readSizeDimensions(size: string, fallback: { value?: string; width: number; height: number }, quality: string) {
    const match = size?.match(/^(\d+)x(\d+)$/);
    if (match) return { width: Number(match[1]), height: Number(match[2]) };
    return imageSizeDimensionsByQuality(fallback.value || "1:1", quality) || { width: fallback.width, height: fallback.height };
}

function imageSizeDimensionsByQuality(size: string, quality: string) {
    const aspect = size === "auto" || !size ? "1:1" : size.split("-")[0];
    const tier = quality === "high" ? "4k" : quality === "medium" ? "2k" : "1k";
    const sizes: Record<string, Record<string, { width: number; height: number }>> = {
        "1k": { "1:1": { width: 1024, height: 1024 }, "3:2": { width: 1536, height: 1024 }, "2:3": { width: 1024, height: 1536 }, "4:3": { width: 1344, height: 1024 }, "3:4": { width: 1024, height: 1344 }, "16:9": { width: 1536, height: 864 }, "9:16": { width: 864, height: 1536 } },
        "2k": { "1:1": { width: 2048, height: 2048 }, "3:2": { width: 2048, height: 1365 }, "2:3": { width: 1365, height: 2048 }, "4:3": { width: 2048, height: 1536 }, "3:4": { width: 1536, height: 2048 }, "16:9": { width: 2048, height: 1152 }, "9:16": { width: 1152, height: 2048 } },
        "4k": { "1:1": { width: 4096, height: 4096 }, "3:2": { width: 3840, height: 2560 }, "2:3": { width: 2560, height: 3840 }, "4:3": { width: 3840, height: 2880 }, "3:4": { width: 2880, height: 3840 }, "16:9": { width: 3840, height: 2160 }, "9:16": { width: 2160, height: 3840 } },
    };
    return sizes[tier]?.[aspect];
}

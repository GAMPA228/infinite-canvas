import type { UserRole } from "@/services/api/auth";

export const premiumImageQualities = new Set(["medium", "high"]);
export const upgradePlanMessage = "请升级用户套餐";

export function canUsePremiumImageQuality(role?: UserRole) {
    return role === "vip" || role === "admin";
}

export function isPremiumImageQuality(quality?: string) {
    return premiumImageQualities.has(quality || "");
}

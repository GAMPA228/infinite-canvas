"use client";

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";

import { useUserStore } from "@/stores/use-user-store";

const protectedRoutes = ["/canvas", "/image"];

export function isProtectedUserRoute(pathname: string) {
    return protectedRoutes.some((route) => pathname === route || pathname.startsWith(`${route}/`));
}

export function loginRequiredHref(pathname: string) {
    return `/login?redirect=${encodeURIComponent(pathname)}&reason=loginRequired`;
}

export function AuthRouteGuard() {
    const pathname = usePathname();
    const router = useRouter();
    const user = useUserStore((state) => state.user);
    const isReady = useUserStore((state) => state.isReady);
    const requiresLogin = isProtectedUserRoute(pathname);

    useEffect(() => {
        if (!isReady || user || !requiresLogin) return;
        const current = `${pathname}${window.location.search || ""}`;
        router.replace(loginRequiredHref(current));
    }, [isReady, pathname, requiresLogin, router, user]);

    return null;
}

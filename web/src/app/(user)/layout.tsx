"use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";

import { AuthRouteGuard, isProtectedUserRoute } from "@/components/layout/auth-route-guard";
import { AppTopNav } from "@/components/layout/app-top-nav";
import { useUserStore } from "@/stores/use-user-store";

export default function UserLayout({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    const user = useUserStore((state) => state.user);
    const isReady = useUserStore((state) => state.isReady);
    const shouldHideProtectedPage = isProtectedUserRoute(pathname) && (!isReady || !user);

    return (
        <div className="flex h-dvh flex-col overflow-hidden bg-background text-foreground">
            <AuthRouteGuard />
            <AppTopNav />
            <div className="min-h-0 flex-1 overflow-hidden">{shouldHideProtectedPage ? null : children}</div>
        </div>
    );
}

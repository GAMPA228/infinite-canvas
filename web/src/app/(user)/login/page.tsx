"use client";

import { KeyOutlined, LockOutlined, UserOutlined } from "@ant-design/icons";
import { App, Button, Form, Input, Segmented } from "antd";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import { useUserStore } from "@/stores/use-user-store";

type LoginFormValues = {
    username: string;
    password: string;
    inviteCode?: string;
    confirmPassword?: string;
};

export default function LoginPage() {
    return (
        <Suspense fallback={null}>
            <LoginContent />
        </Suspense>
    );
}

function LoginContent() {
    const { message } = App.useApp();
    const router = useRouter();
    const searchParams = useSearchParams();
    const login = useUserStore((state) => state.login);
    const register = useUserStore((state) => state.register);
    const isLoading = useUserStore((state) => state.isLoading);
    const [mode, setMode] = useState<"login" | "register">("login");
    const redirect = searchParams.get("redirect") || "/";
    const reason = searchParams.get("reason");

    useEffect(() => {
        if (reason === "loginRequired") message.warning("请登陆使用完整功能");
    }, [message, reason]);

    const submit = async (values: LoginFormValues) => {
        try {
            if (mode === "register") {
                if (values.password !== values.confirmPassword) {
                    message.error("两次输入的密码不一致");
                    return;
                }
                await register({ username: values.username, password: values.password, inviteCode: values.inviteCode });
                message.success("注册成功");
                router.replace("/");
                router.refresh();
                return;
            }
            const user = await login({ username: values.username, password: values.password });
            message.success("登录成功");
            const nextPath = redirect.startsWith("/") ? redirect : "/";
            router.replace(user.role === "admin" ? nextPath : nextPath.startsWith("/admin") ? "/" : nextPath);
            router.refresh();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "登录失败");
        }
    };

    return (
        <main className="flex h-full min-h-0 items-center justify-center overflow-y-auto bg-background bg-[radial-gradient(#e5e7eb_1px,transparent_1px)] px-6 py-10 [background-size:16px_16px] dark:bg-[radial-gradient(rgba(245,245,244,.16)_1px,transparent_1px)]">
            <section className="w-full max-w-[420px]">
                <div className="mb-7 text-center">
                    <span
                        className="mx-auto mb-4 block size-12 bg-stone-950 dark:bg-stone-100"
                        style={{
                            mask: "url(/logo.svg) center / contain no-repeat",
                            WebkitMask: "url(/logo.svg) center / contain no-repeat",
                        }}
                        aria-label="画布"
                    />
                    <h1 className="text-3xl font-semibold tracking-normal text-stone-950 dark:text-stone-100">账号登录</h1>
                    <p className="mt-3 text-base leading-7 text-stone-500 dark:text-stone-400">使用账号登录，或通过管理员分配的邀请码注册。</p>
                </div>

                <Form<LoginFormValues> layout="vertical" size="large" requiredMark={false} onFinish={submit}>
                    <Segmented
                        block
                        className="mb-5"
                        value={mode}
                        onChange={(value) => setMode(value as "login" | "register")}
                        options={[
                            { label: "登录", value: "login" },
                            { label: "邀请码注册", value: "register" },
                        ]}
                    />
                    <Form.Item name="username" label={<span className="font-medium text-stone-800 dark:text-stone-200">用户名</span>} rules={[{ required: true, message: "请输入用户名" }]}>
                        <Input prefix={<UserOutlined />} autoComplete="username" />
                    </Form.Item>
                    <Form.Item name="password" label={<span className="font-medium text-stone-800 dark:text-stone-200">密码</span>} rules={[{ required: true, message: "请输入密码" }]}>
                        <Input.Password prefix={<LockOutlined />} autoComplete={mode === "login" ? "current-password" : "new-password"} />
                    </Form.Item>
                    {mode === "register" ? (
                        <>
                            <Form.Item name="confirmPassword" label={<span className="font-medium text-stone-800 dark:text-stone-200">确认密码</span>} rules={[{ required: true, message: "请再次输入密码" }]}>
                                <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
                            </Form.Item>
                            <Form.Item name="inviteCode" label={<span className="font-medium text-stone-800 dark:text-stone-200">邀请码</span>} rules={[{ required: true, message: "请输入邀请码" }]}>
                                <Input prefix={<KeyOutlined />} autoComplete="off" />
                            </Form.Item>
                        </>
                    ) : null}
                    <Button block type="primary" htmlType="submit" loading={isLoading}>
                        {mode === "login" ? "登录" : "注册并登录"}
                    </Button>
                </Form>
            </section>
        </main>
    );
}

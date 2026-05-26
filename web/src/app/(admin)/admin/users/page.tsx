"use client";

import { CopyOutlined, DeleteOutlined, EditOutlined, KeyOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import { ProTable, type ProColumns } from "@ant-design/pro-components";
import { Button, Card, Col, Divider, Flex, Form, Input, InputNumber, Modal, Row, Select, Space, Tag, Tooltip, Typography } from "antd";
import { useEffect, useState } from "react";

import { useCopyText } from "@/hooks/use-copy-text";
import type { AdminUser } from "@/services/api/admin";
import type { UserRole } from "@/services/api/auth";
import { useAdminUsers } from "./use-admin-users";

type UserFormValues = Partial<AdminUser> & { password?: string };

const roleOptions: { label: string; value: UserRole }[] = [
    { label: "普通用户", value: "user" },
    { label: "VIP 用户", value: "vip" },
    { label: "管理员", value: "admin" },
];

const roleFilterOptions = [{ label: "全部角色", value: "" }, ...roleOptions];

function roleLabel(role: UserRole) {
    return roleOptions.find((item) => item.value === role)?.label || role;
}

function roleColor(role: UserRole) {
    if (role === "admin") return "red";
    if (role === "vip") return "gold";
    return "blue";
}

export default function AdminUsersPage() {
    const { users, channels, keyword, role, page, pageSize, total, isLoading, searchUsers, changeRole, changePage, changePageSize, resetFilters, refreshUsers, saveUser: saveAdminUser, adjustCredits, deleteUser } = useAdminUsers();
    const copyText = useCopyText();
    const [form] = Form.useForm<UserFormValues>();
    const [editingUser, setEditingUser] = useState<Partial<AdminUser> | null>(null);
    const [deletingUser, setDeletingUser] = useState<AdminUser | null>(null);
    const selectedRole = Form.useWatch("role", form);
    const channelOptions = [
        { label: "不指定渠道", value: "" },
        ...channels.map((item) => ({ label: item.name || "未命名渠道", value: item.name })),
    ];

    useEffect(() => {
        if (!editingUser) return;
        form.setFieldsValue({ ...editingUser, password: "" });
    }, [editingUser, form]);

    const saveUser = async () => {
        const value = await form.validateFields();
        const userValue = { ...value };
        delete userValue.credits;
        const username = value.username?.trim();
        const password = value.password?.trim();
        if (username && !editingUser?.id && !password) {
            form.setFields([{ name: "password", errors: ["新建用户请填写密码"] }]);
            return;
        }
        await saveAdminUser({
            ...editingUser,
            ...userValue,
            username,
            role: value.role || "user",
            channelName: value.role === "vip" ? value.channelName || "" : "",
            password: password || undefined,
        });
        if (editingUser?.id && value.credits !== undefined && Number(value.credits) !== Number(editingUser.credits || 0)) {
            await adjustCredits(editingUser.id, Number(value.credits) || 0);
        }
        setEditingUser(null);
    };

    const saveCredits = async () => {
        if (!editingUser?.id) return;
        const credits = Number(form.getFieldValue("credits")) || 0;
        const user = await adjustCredits(editingUser.id, credits);
        setEditingUser((current) => (current ? { ...current, credits: user.credits } : current));
    };

    const columns: ProColumns<AdminUser>[] = [
        {
            title: "用户",
            dataIndex: "username",
            width: 220,
            render: (_, item) => (
                <Flex vertical gap={2}>
                    <Typography.Text strong>{item.username || "待注册用户"}</Typography.Text>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        {item.id}
                    </Typography.Text>
                </Flex>
            ),
        },
        {
            title: "角色",
            dataIndex: "role",
            width: 120,
            render: (_, item) => <Tag color={roleColor(item.role)}>{roleLabel(item.role)}</Tag>,
        },
        {
            title: "邀请码",
            dataIndex: "inviteCode",
            width: 190,
            render: (_, item) =>
                item.inviteCode ? (
                    <Space size={4}>
                        <Typography.Text code>{item.inviteCode}</Typography.Text>
                        <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => copyText(item.inviteCode)} />
                    </Space>
                ) : (
                    <Typography.Text type="secondary">未生成</Typography.Text>
                ),
        },
        {
            title: "渠道",
            dataIndex: "channelName",
            width: 180,
            render: (_, item) => (item.role === "vip" && item.channelName ? <Tag color="purple">{item.channelName}</Tag> : <Typography.Text type="secondary">默认渠道</Typography.Text>),
        },
        {
            title: "算力点",
            dataIndex: "credits",
            width: 100,
            render: (_, item) => <Typography.Text>{item.credits || 0}</Typography.Text>,
        },
        {
            title: "状态",
            dataIndex: "inviteUsedAt",
            width: 120,
            render: (_, item) => (item.inviteCode ? item.inviteUsedAt ? <Tag>已使用</Tag> : <Tag color="green">未使用</Tag> : <Tag color="blue">已建号</Tag>),
        },
        {
            title: "更新时间",
            dataIndex: "updatedAt",
            width: 190,
            render: (_, item) => <Typography.Text type="secondary">{item.updatedAt || item.createdAt}</Typography.Text>,
        },
        {
            title: "操作",
            key: "actions",
            width: 116,
            align: "right",
            render: (_, item) => (
                <Space size={4}>
                    <Tooltip title="编辑">
                        <Button type="text" size="small" icon={<EditOutlined />} onClick={() => setEditingUser(item)} />
                    </Tooltip>
                    <Tooltip title="删除">
                        <Button danger type="text" size="small" icon={<DeleteOutlined />} onClick={() => setDeletingUser(item)} />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    return (
        <main style={{ padding: 24 }}>
            <Flex vertical gap={16}>
                <Card variant="borderless">
                    <Form layout="vertical">
                        <Row gutter={16} align="bottom">
                            <Col flex="360px">
                                <Form.Item label="关键词">
                                    <Input.Search value={keyword} placeholder="搜索用户名或邀请码" allowClear enterButton={<SearchOutlined />} onSearch={searchUsers} onChange={(event) => searchUsers(event.target.value)} />
                                </Form.Item>
                            </Col>
                            <Col flex="220px">
                                <Form.Item label="角色">
                                    <Select value={role} onChange={changeRole} options={roleFilterOptions} />
                                </Form.Item>
                            </Col>
                            <Col flex="none">
                                <Form.Item>
                                    <Space>
                                        <Button onClick={resetFilters}>重置</Button>
                                        <Button type="primary" icon={<ReloadOutlined />} onClick={refreshUsers}>
                                            查询
                                        </Button>
                                    </Space>
                                </Form.Item>
                            </Col>
                        </Row>
                    </Form>
                </Card>

                <ProTable<AdminUser>
                    rowKey="id"
                    columns={columns}
                    dataSource={users}
                    loading={isLoading}
                    search={false}
                    defaultSize="middle"
                    tableLayout="fixed"
                    cardProps={{ variant: "borderless" }}
                    headerTitle={
                        <Space>
                            <Typography.Text strong>用户列表</Typography.Text>
                            <Tag>{total} 个</Tag>
                        </Space>
                    }
                    options={{ density: true, setting: true, reload: () => void refreshUsers() }}
                    toolBarRender={() => [
                        <Button key="invite" icon={<KeyOutlined />} onClick={() => setEditingUser({ role: "user" })}>
                            生成邀请码
                        </Button>,
                        <Button key="add" type="primary" icon={<PlusOutlined />} onClick={() => setEditingUser({ username: "", role: "user" })}>
                            新建用户
                        </Button>,
                    ]}
                    pagination={{
                        current: page,
                        pageSize,
                        total,
                        showSizeChanger: true,
                        pageSizeOptions: [10, 20, 50, 100],
                        showTotal: (value) => `共 ${value} 个`,
                        onChange: (nextPage, nextPageSize) => (nextPageSize !== pageSize ? changePageSize(nextPageSize) : changePage(nextPage)),
                    }}
                />
            </Flex>

            <Modal title={editingUser?.id ? "编辑用户" : editingUser?.username === "" ? "新建用户" : "生成邀请码"} open={Boolean(editingUser)} width={560} onCancel={() => setEditingUser(null)} onOk={() => void saveUser()} okText="保存" cancelText="取消" destroyOnHidden>
                <Form form={form} layout="vertical" requiredMark={false}>
                    <Form.Item name="username" label="用户名" extra="只生成邀请码时可留空，用户注册时会填写用户名。">
                        <Input placeholder="例如 zhangsan" />
                    </Form.Item>
                    <Form.Item name="password" label="密码" extra={editingUser?.id ? "不填写则保持原密码；待注册邀请码可不填。" : "新建用户需填写密码；只生成邀请码可不填。"}>
                        <Input.Password autoComplete="new-password" />
                    </Form.Item>
                    <Form.Item name="role" label="角色" rules={[{ required: true, message: "请选择角色" }]}>
                        <Select options={roleOptions} />
                    </Form.Item>
                    <Form.Item name="channelName" label="渠道" extra="仅 VIP 用户生效；不指定时继续按全局渠道权重选择。">
                        <Select disabled={selectedRole !== "vip"} options={channelOptions} />
                    </Form.Item>
                    <Form.Item name="inviteCode" label="邀请码" extra="留空时后端会自动生成；邀请码只能使用一次。">
                        <Input placeholder="自动生成" />
                    </Form.Item>
                    {editingUser?.id ? (
                        <>
                            <Divider style={{ margin: "4px 0 16px" }} />
                            <Typography.Text strong>算力点调整</Typography.Text>
                            <Row gutter={14} style={{ marginTop: 12 }}>
                                <Col span={14}>
                                    <Form.Item label="算力点">
                                        <Space.Compact style={{ width: "100%" }}>
                                            <Form.Item name="credits" noStyle>
                                                <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                                            </Form.Item>
                                            <Button onClick={() => void saveCredits()}>调整</Button>
                                        </Space.Compact>
                                    </Form.Item>
                                </Col>
                            </Row>
                        </>
                    ) : null}
                </Form>
            </Modal>

            <Modal
                title="删除用户"
                open={Boolean(deletingUser)}
                onCancel={() => setDeletingUser(null)}
                onOk={async () => {
                    if (!deletingUser) return;
                    await deleteUser(deletingUser.id);
                    setDeletingUser(null);
                }}
                okText="删除"
                okButtonProps={{ danger: true }}
                cancelText="取消"
            >
                确定删除「{deletingUser?.username || deletingUser?.inviteCode || "待注册用户"}」吗？最后一个管理员不会被允许删除。
            </Modal>
        </main>
    );
}

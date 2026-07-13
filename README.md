# Fake Komari Agent

轻量的 Komari V2 虚拟探针管理器。配置保存到 `data/agents.json`。

## 运行

```bash
go run .
```

默认监听：

```text
http://127.0.0.1:8099
```

首次运行会生成随机面板密码并输出到日志。`data/agents.json` 只保存 `panel_password_hash`，浏览器登录后由服务端下发 HttpOnly cookie。

可选参数：

```bash
go run . -listen 127.0.0.1:8099 -data data/agents.json
```

重置面板密码：

```bash
go run . -data data/agents.json -reset-password
go run . -data data/agents.json -reset-password 'your-new-password'
```

第一条会随机生成新密码并输出，第二条会把后面的内容设置为新密码。

如果当前环境不允许写 Go 默认缓存，可以临时指定：

```bash
GOCACHE=/tmp/go-build go run .
```

## 行为

- 只支持 Komari V2 JSON-RPC。
- 只使用 WebSocket：`/api/clients/v2/rpc?token=...`。
- 创建 agent 时填写 Komari 面板地址和自动发现密钥。
- 首次启用时会调用 `POST /api/clients/register?name=...` 自动创建 Komari 节点，并把返回的 token 写回 `data/agents.json`。
- 基础信息通过 `agent.basicInfo` 发送，参数为 `params.info`。
- 实时状态通过 `agent.report` 发送，参数为 `params.report`。
- CPU、RAM、Swap、Disk、Network、Load、连接数和进程数按“基准值 + 波动范围”生成。
- 收到控制端下发事件时只统计 ignored，不执行命令、不模拟 ping、不建立终端、不回传任务结果。

## JSON 数据

`data/agents.json` 是唯一数据库。可以直接编辑。容量字段使用字节，网络速率字段使用字节每秒。`panel_password_hash` 是本地管理面板密码哈希，`discovery_key` 是 Komari 自动发现密钥，`token` 是注册后用于 V2 WebSocket 上报的节点 token。

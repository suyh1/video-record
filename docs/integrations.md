# 集成

所有集成都为可选能力。TMDB 或媒体服务器不可用时，本地影库、已缓存详情和手动记录必须继续工作。前端不会接收上游原始响应或明文凭据。

## TMDB

TMDB 提供电影、剧集、季集和单集搜索与基础元数据。令牌只能通过服务端环境变量 `TMDB_READ_ACCESS_TOKEN` 注入：

```text
TMDB_READ_ACCESS_TOKEN=<TMDB read access token>
```

不要使用 `VITE_` 前缀，不要把令牌写入 Dockerfile、Compose 默认值、源码、测试或文档。修改 `.env` 后重建容器环境：

```bash
docker compose up -d --force-recreate video-record
```

设置页只显示“已配置/未配置”，不会回显令牌。搜索缓存默认 6 小时，详情与季集快照缓存默认 7 天；已经记录的本地数据不依赖 TMDB 实时可用。

TMDB 要求的归属说明：

> This product uses the TMDB API but is not endorsed or certified by TMDB.

TMDB 指 The Movie Database。产品与 TMDB 没有背书或认证关系。

## 媒体服务器账户

媒体服务器账户在“设置 -> 媒体服务器”中按当前登录用户创建。凭据提交到同源后端后立即使用 `APP_ENCRYPTION_KEY` 加密；列表和状态 API 只返回 provider、名称、启用状态、指纹和运行摘要。

不要直接编辑 `external_accounts` 表，也不要把凭据放进 URL query、反向代理配置或诊断日志。

### Jellyfin

需要：

- 可从 video-record 容器访问的 Jellyfin Base URL
- 专用 API token
- Jellyfin user ID
- 已安装并启用 Playback Reporting 插件

历史通过插件的 `/user_usage_stats/{userId}/{date}/GetItems` 读取，条目详情通过 Jellyfin 标准 API 补全。建议为集成创建权限最小化的专用 token，并验证目标用户能够读取自己的播放历史。

### Emby

需要：

- 可访问的 Emby Base URL；若部署使用 `/emby` 子路径，应包含该子路径
- 专用 API token
- Emby user ID
- 已安装并启用 Playback Reporting 插件
- 可选 IANA 时区，例如 `Asia/Shanghai`

Emby 插件时间没有时区信息，服务端会使用账户配置的 IANA 时区转换为 UTC。时区留空时使用 UTC。

### Plex

需要：

- 可访问的 Plex Base URL
- Plex token
- 正整数 account ID

历史来自 `/status/sessions/history/all`。token 只通过 `X-Plex-Token` 请求头发送；实现不会把媒体文件路径保存到候选、错误或日志。

## 同步行为

- 每个启用账户默认每 15 分钟执行增量同步。
- 每日补偿任务回看最近 24 小时。
- 任务、游标和受控摘要保存在 SQLite；进程重启后会补跑到期任务。
- 完全确定且无冲突的外部 ID 可自动落档。
- 标题/年份可能匹配、类型冲突或手工记录冲突会进入“待核对”。
- 确认、重新匹配、忽略和创建自定义条目都是幂等写操作。
- 用户手动编辑的状态、评分、日期和笔记优先于同步数据。

在“设置 -> 同步记录核对”处理候选。网络失败会保留当前选择和输入。

## 锁定与故障

缺失、丢失或更换 `APP_ENCRYPTION_KEY` 时，媒体服务器凭据进入锁定状态，同步停止，但观影记录不受影响。恢复原密钥或重新连接账户后才能继续同步。

401/403 会记录为稳定认证错误；429 遵循 `Retry-After`；超时和 5xx 按可重试错误处理。上游错误正文、token、媒体路径和远端地址不会保存到同步摘要。

媒体服务器短暂故障不会让 `/readyz` 失败，也不会阻塞 HTTP 服务。

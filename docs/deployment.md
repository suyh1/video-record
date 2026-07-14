# 部署

## 支持范围

生产容器支持 `linux/amd64`（x86_64）和 `linux/arm64`（aarch64）。最终镜像使用 distroless nonroot 运行时，以固定用户 `65532:65532` 运行，只监听容器内 `8080/tcp`，并只写入 `/data`。

当前仓库尚未执行外部发布。发布前从源码使用 Compose 构建；发布后应把 `IMAGE_REPOSITORY` 与不可变完整版本 `IMAGE_TAG` 指向已验证的镜像，不要在无人值守部署中依赖 `latest`。

## 前置条件

- Docker Engine 27 或更高版本
- Docker Compose v2
- 至少 1 GiB 可用内存
- 能持久保存 Docker named volume 的本地磁盘
- 反向代理场景下可用的 HTTPS 域名与证书

SQLite 数据与备份默认位于：

```text
/data/video-record.db
/data/backups/
```

默认 Compose 使用 `video_record_data` named volume。不要把 `/data` 放到不支持可靠文件锁、原子 rename 和 fsync 的网络文件系统。

## 配置

在仓库根目录创建权限为 `0600` 的 `.env`。生成随机 32 byte 加密密钥并以 Base64 保存：

```bash
umask 077
{
  printf 'APP_ENCRYPTION_KEY=%s\n' "$(openssl rand -base64 32)"
  printf 'APP_COOKIE_SECURE=true\n'
  printf 'TMDB_READ_ACCESS_TOKEN=\n'
  printf 'VIDEO_RECORD_PORT=8080\n'
} > .env
chmod 600 .env
```

把 `APP_ENCRYPTION_KEY` 另存到密码管理器或离线密钥库。不要只把它留在应用主机或 Docker volume 中。

| 变量 | 必需 | 说明 |
|---|---:|---|
| `APP_ENCRYPTION_KEY` | 是 | Base64 编码的随机 32 byte 值；加密媒体服务器凭据，不进入备份 |
| `TMDB_READ_ACCESS_TOKEN` | 否 | TMDB Read Access Token；只能由服务端读取 |
| `APP_COOKIE_SECURE` | 生产是 | HTTPS 设为 `true`；仅本机纯 HTTP 验证可设为 `false` |
| `VIDEO_RECORD_PORT` | 否 | Docker 发布端口，默认 `8080`；例如 `18080`，仅本机访问可用 `127.0.0.1:18080` |
| `IMAGE_REPOSITORY` | 发布后 | 已授权发布的镜像仓库名 |
| `IMAGE_TAG` | 发布后 | 建议固定完整 SemVer，例如 `v1.0.0` |

`APP_PORT`、`APP_ENV` 和 `DATA_DIR` 已由 Compose 固定为容器内部生产值，不需要在 `.env` 中覆盖。

## 首次 Compose 安装

在全新主机克隆仓库、创建 `.env` 后执行：

```bash
docker compose config
docker compose up -d --build
docker compose ps
curl --fail http://127.0.0.1:${VIDEO_RECORD_PORT:-8080}/healthz
curl --fail http://127.0.0.1:${VIDEO_RECORD_PORT:-8080}/readyz
```

`/healthz` 只表示进程存活；`/readyz` 检查数据库迁移和 SQLite 可用性。TMDB 或媒体服务器暂时不可用不会让整个服务变为不就绪。

打开站点后，初始化页只允许创建一次首位管理员。完成初始化后，未认证访问会进入登录页。

## 修改端口

宿主机端口由 `VIDEO_RECORD_PORT` 控制，容器内端口保持 8080：

```bash
printf 'VIDEO_RECORD_PORT=18080\n' >> .env
docker compose up -d
```

随后使用 `http://host.example:18080`。同一个 `.env` 中不要保留重复变量；若已有该行，应编辑原值。

只允许本机访问时可使用 `VIDEO_RECORD_PORT=127.0.0.1:18080`，并通过同一地址完成健康检查和初始化。需要由局域网或反向代理访问时使用单独端口号，并在主机防火墙限制来源。

## HTTPS 反向代理

生产部署应只把反向代理暴露到不受信任网络，并保持 `APP_COOKIE_SECURE=true`。代理必须：

- 保留原始 `Host` 与 `Origin`
- 转发到 `video-record:8080` 或宿主机发布端口
- 使用 HTTPS 并把 HTTP 重定向到 HTTPS
- 不缓存 `/api/` 响应
- 允许备份恢复上传和最长 30 秒的应用响应时间

应用会校验写请求的 Origin 与 CSRF token。反向代理改写 Origin 或跨域嵌入会导致写请求被拒绝。

## 发布镜像部署

仅在发布工作流已推送并验证双架构 manifest 后使用：

```bash
export IMAGE_REPOSITORY='<docker-hub-owner>/video-record'
export IMAGE_TAG='v1.0.0'
docker compose pull video-record
docker compose up -d --no-build
```

发布工作流会分别预检 amd64 与 arm64，再生成包含 SBOM 和 provenance 的 manifest。外部仓库、标签和 `latest` 在获得明确授权前都不会创建。

## 日常操作

```bash
docker compose ps
docker compose logs --tail=200 video-record
docker compose restart video-record
docker compose stop video-record
docker compose start video-record
```

先阅读[备份与恢复](backup-restore.md)再迁移主机，先阅读[升级与回滚](upgrading.md)再更换镜像版本。

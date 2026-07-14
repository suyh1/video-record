# video-record

video-record 是一套个人与家庭自托管的电影、电视剧观影档案。手动搜索和记录是完整主流程；TMDB 元数据以及 Jellyfin、Emby、Plex 播放历史同步都是可选能力。

项目当前处于本地 `v1.0.0` 发布候选验收阶段。仓库不会自动创建 Git 标签、推送 Docker 镜像或更新 `latest`；这些外部动作必须在发布清单签署后获得明确授权。

## 功能

- 电影、剧集、季集和单集记录
- 想看、在看、看过、弃看与未设置五态影库
- 评分、笔记、标签、片单、观看日期、重复观看和剧集进度
- 家庭成员独立记录、显式分享和共同观看事件
- 日历、筛选和统计
- JSON/CSV 导入导出
- 一致性 SQLite 备份与原子恢复
- 可选 TMDB 搜索以及 Jellyfin、Emby、Plex 播放历史同步
- 响应式亮暗主题和 WCAG 2.2 AA 核心流程

## 快速开始

需要 Git、Docker Engine 27+ 和 Docker Compose v2。首次安装从源码构建生产镜像：

```bash
git clone <repository-url> video-record
cd video-record
umask 077
{
  printf 'APP_ENCRYPTION_KEY=%s\n' "$(openssl rand -base64 32)"
  printf 'APP_COOKIE_SECURE=false\n'
  printf 'TMDB_READ_ACCESS_TOKEN=\n'
} > .env
docker compose up -d --build
docker compose ps
```

打开 `http://localhost:8080` 完成首位管理员初始化。`APP_COOKIE_SECURE=false` 只适合本机或可信局域网的纯 HTTP 验证；经 HTTPS 反向代理部署时应删除这一行或设为 `true`。

停止服务不会删除数据：

```bash
docker compose down
```

不要执行 `docker compose down -v`，除非已经完成并验证异地备份且明确要删除全部数据。

## 文档

- [部署](docs/deployment.md)
- [集成](docs/integrations.md)
- [备份与恢复](docs/backup-restore.md)
- [升级与回滚](docs/upgrading.md)
- [安全](docs/security.md)
- [性能基线](docs/performance.md)
- [发布清单](docs/release-checklist.md)

## 本机开发

需要 Go 1.26.5、Node.js 24.13.0 和 npm。后端与前端分别运行：

```bash
export APP_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export APP_COOKIE_SECURE=false
go run ./cmd/server
```

```bash
npm --prefix web ci
npm --prefix web run dev
```

Vite 开发服务器代理 `/api` 到 Go 服务。真实凭据只能存在于被 Git 忽略的本机环境或安全密钥存储中。

常用验证：

```bash
go test ./... -race -count=1
go vet ./...
npm --prefix web test -- --run
npm --prefix web run typecheck
npm --prefix web run build
./scripts/docs-acceptance-test.sh
```

## 许可证与第三方服务

仓库发布前应补充适用的开源许可证文件。TMDB 归属说明与凭据边界见[集成文档](docs/integrations.md)。

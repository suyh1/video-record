# Compose 单一配置源设计

## 目标

Docker Compose 部署只需要编辑 `docker-compose.yml`，不再要求用户同时创建或维护 `.env`。Compose 文件不得使用 `${...}` 插值。

## 配置设计

- 删除 `.env.example`，并移除文档中创建、修改 `.env` 的说明。
- `docker-compose.yml` 使用固定的本地镜像名、宿主机端口和容器运行参数。
- `APP_COOKIE_SECURE` 默认设为 `false`，保证默认的 `http://localhost:8080` 安装可以登录；HTTPS 部署由用户直接改为 `true`。
- `APP_ENCRYPTION_KEY` 和 `TMDB_READ_ACCESS_TOKEN` 以空字符串保留在 Compose 中。用户需要媒体服务器凭据加密或 TMDB 时，直接在 Compose 中填写值。
- `APP_ENCRYPTION_KEY` 为空时应用继续提供本地观影记录能力，但媒体服务器凭据保持锁定；不得内置所有部署共享的固定密钥。
- 镜像固定为 `video-record:local`，端口固定为 `8080:8080`。发布镜像、版本、端口和监听地址均通过直接编辑 Compose 文件调整。

## 文档与安全边界

README、部署、安全、集成和升级文档统一把 `docker-compose.yml` 描述为 Compose 部署的唯一配置源。文档明确说明直接写入的密钥属于本机敏感修改，不应提交到版本控制；长期生产部署仍应把密钥原值保存在独立密码管理器或离线密钥库中。

## 验证

- 添加静态验收检查，确保 `.env.example` 不存在、Compose 不包含 `${`，并包含必要的显式配置项。
- 运行 `docker compose config`，确认没有 `.env` 时 Compose 配置可解析。
- 运行文档验收、Go 测试及现有仓库验证命令，确认配置说明和应用默认行为一致。

# 升级与回滚

## 原则

- 只升级到已经通过发布清单的完整 SemVer 镜像。
- 不使用 `latest` 作为无人值守升级源。
- 升级前同时创建应用级 `.vrbackup` 和停止写入后的 Docker volume 冷快照。
- 启动检测到已有数据库存在待执行迁移时，应用会先自动创建一致性 `.vrbackup`；自动备份失败则不会开始迁移。
- 数据库迁移在服务启动时自动执行，事务失败会阻止服务启动。
- 回滚旧镜像时必须同时回滚到升级前数据库；旧二进制不保证读取新架构。

## 升级前检查

1. 阅读目标版本变更和安全说明。
2. 确认当前 `/readyz` 为成功状态。
3. 从设置页创建、下载并校验 `.vrbackup`。
4. 确认原 `APP_ENCRYPTION_KEY` 在独立密钥库中可恢复。
5. 记录 Compose 中当前完整的 `image` 值和镜像 digest。

创建用于版本回滚的冷卷快照：

```bash
docker compose stop video-record
container_id="$(docker compose ps -q video-record)"
volume_name="$(docker inspect "$container_id" --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}')"
test -n "$volume_name"
docker run --rm \
  -v "$volume_name:/source:ro" \
  -v "$PWD:/backup" \
  alpine:3.22 \
  tar -C /source -czf /backup/video-record-data-pre-upgrade.tgz .
docker compose start video-record
sha256sum video-record-data-pre-upgrade.tgz
```

冷快照文件包含私人数据，必须限制权限并转移到加密存储。辅助镜像版本应在组织的供应链策略中固定和审核。

## 自动迁移前备份

已有数据库启动新版本时，应用会先读取迁移历史。只要检测到至少一个待执行迁移，就会使用 SQLite Online Backup API 在 `/data/backups/` 创建 `video-record-*.vrbackup`，写入 manifest 和 SHA-256 后才开始迁移。首次安装和没有待执行迁移的重启不会创建重复备份。

如果自动备份无法创建、校验或原子提交，迁移不会开始，服务保持未就绪并退出启动流程。此时应先修复 `/data` 权限、可用空间或存储故障，保留现有数据库，再重新启动；不要删除数据库或绕过备份继续迁移。

恢复旧版 `.vrbackup` 时，恢复流程自身已经先保存当前数据库快照，因此内部执行 N-1 迁移不会再创建重复的升级备份。手工升级前备份和冷卷快照仍然必须执行：自动备份用于保护迁移边界，不能替代异地主机故障恢复副本。

## 使用发布镜像升级

把 Compose 的 `image` 更新为目标完整版本，例如 `owner/video-record:v1.1.0`，然后：

```bash
docker compose pull video-record
docker compose up -d --no-build video-record
docker compose ps
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

验证登录、影库、搜索、写入、备份列表和集成状态。检查日志时不要把完整日志上传到公开问题，因为它可能包含作品名、用户名或内部 URL。

## 从源码构建升级

在外部镜像尚未发布时，先确认当前提交已通过发布门禁，再更新源码：

```bash
git pull --ff-only
docker compose build --pull video-record
docker compose up -d video-record
```

仍需执行与发布镜像相同的健康和功能验证。

## 回滚

如果升级后的服务已经迁移或写入数据库，不要只切回旧镜像。完整 rollback 顺序为：

1. 停止服务并保留失败现场日志。
2. 删除当前数据 volume，重建同名空 volume。
3. 从升级前冷快照恢复数据 volume。
4. 把 Compose 的 `image` 恢复为上一完整版本。
5. 启动旧镜像并验证 `/readyz`、登录和核心记录。

```bash
docker compose down
docker volume rm "$volume_name"
docker volume create "$volume_name"
docker run --rm \
  -v "$volume_name:/target" \
  -v "$PWD:/backup:ro" \
  alpine:3.22 \
  tar -C /target -xzf /backup/video-record-data-pre-upgrade.tgz
docker compose up -d --no-build video-record
```

确认回滚成功前不要删除新版本日志、`.vrbackup` 或冷快照。若只是业务数据误操作且当前版本健康，优先使用设置页的原子恢复，而不是执行版本回滚。

## 升级后清理

至少观察一个同步周期和一次备份后，再删除旧镜像与临时冷快照。本地镜像清理不得使用会误删未验证回滚镜像的全局 prune 命令。

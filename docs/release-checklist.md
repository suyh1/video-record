# v1 发布清单

此清单同时是 `v1.0.0` 的本地验收记录和外部发布授权闸门。命令必须在待发布提交的干净 checkout 上重新执行，不能引用更早任务的结果代替。

## 候选信息

| 项目 | 记录 |
|---|---|
| 版本 | `v1.0.0` release candidate；标签未创建 |
| 源分支 | `main` |
| 源提交 | Task 27 验收提交；以本清单所在最终 `main` 提交为准 |
| 验收日期 | 2026-07-13 |
| 外部镜像仓库 | 未配置或未授权发布 |

## 文档与安装

- [x] 全新独立 Compose 项目与 named volume 能够完成管理员初始化。
- [x] `APP_ENCRYPTION_KEY` 使用 `openssl rand -base64 32` 生成，未使用固定或示例密钥。
- [x] `TMDB_READ_ACCESS_TOKEN` 只由服务端环境变量读取；设置页显示 TMDB 归属说明。
- [x] Compose `ports` 改为 `"127.0.0.1:18080:8080"` 后，健康、就绪、前端和初始化访问正常。
- [x] README、部署、集成、备份恢复、升级回滚和安全文档与当前行为一致。

## 自动化质量门禁

待执行并记录摘要：

```bash
go test ./... -race -count=1
go vet ./...
npm --prefix web ci
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web test -- --run
npm --prefix web run api:check
npm --prefix web run build
npm --prefix web run e2e
./scripts/docs-acceptance-test.sh
./scripts/perf-smoke.sh
./scripts/recovery-smoke.sh
git diff --check
```

- [x] Go race/vet、迁移、关键包精确覆盖率与 govulncheck 通过。
- [x] 前端 lint、typecheck、23 个测试文件中的 44 项组件测试、OpenAPI 生成检查和 build 通过。
- [x] 9 项 Playwright E2E 通过，WCAG 2.2 AA 阻断违规为 0。
- [x] 性能与四种中断恢复门禁通过。

## 容器与双架构

| 平台 | 镜像引用 | 镜像 digest | 大小 | 烟测 | 漏洞 | 层密钥扫描 |
|---|---|---|---:|---|---|---|
| `linux/amd64` | `local/video-record:release-amd64` | `sha256:0602f09c91dd7d52b43fb1725f43442e8d3cb81468a3816ef0d55dd75a870122` | 6,197,906 bytes | 通过 | `0C/0H/0M/0L` | 14 层 / 964 文件，无匹配 |
| `linux/arm64` | `local/video-record:release-arm64` | `sha256:dcdd4e1d49c1ec5f7b471f63fc5eb16cc1e658ab25a6b4bf01b453aefdc259f2` | 5,845,732 bytes | 通过 | `0C/0H/0M/0L` | 14 层 / 964 文件，无匹配 |

- [x] amd64 与 arm64 镜像分别运行完整 container smoke，不只检查 manifest 标签。
- [x] 每个平台镜像都以 `65532:65532`、只读根、drop ALL capabilities 运行，只有 `/data` 可写。
- [x] `scripts/verify-manifest.sh` 的合成双平台测试通过；外部 manifest 仅在获准 push 后验证。
- [x] 镜像 SBOM 与 provenance 由 release workflow 生成；本地不伪造外部 attestation。

## 数据可靠性

- [x] 使用合成数据完成 backup/restore 演练：备份前基线记录保留，备份后新增成员在恢复后消失。
- [x] `rehearsal.vrbackup` 为 15,477 bytes，SHA-256 `f3f29f985385e24364bdf9a2bf2d4da6acbccc9ffe8944b589b0134b5f3d59fb`；归档只含 manifest/数据库且未发现环境密钥名或 JWT 形态。
- [x] 普通写入、迁移、同步和 Online Backup 中途终止后，重启、迁移与 `PRAGMA integrity_check` 通过。
- [x] 升级 rollback 文档固定使用升级前冷卷快照恢复旧数据库与旧镜像，不用旧镜像直接打开新架构。

## 安全与供应链

```bash
go run github.com/zricethezav/gitleaks/v8@v8.30.1 dir . --redact --no-banner
go run github.com/zricethezav/gitleaks/v8@v8.30.1 git --redact --no-banner
./scripts/image-secret-scan.sh <local-image>
```

- [x] 工作树与完整 Git 历史 secret scan / gitleaks 无匹配。
- [x] 两个平台镜像的配置、history、所有层和二进制字符串密钥扫描无匹配。
- [x] Go 可达漏洞为 0；npm high 为 0；镜像 Critical 0、High 0。
- [x] GitHub Actions 固定到完整 SHA，stable alias 只从已验证的不可变 digest 晋级。

## 外部发布授权

- [ ] Docker Hub `IMAGE_REPOSITORY`、`DOCKERHUB_USERNAME` 与 `DOCKERHUB_TOKEN` 已由仓库管理员配置。
- [ ] 发布负责人已审阅本清单中的实际输出摘要、digest 与残余风险。
- [ ] 用户已明确授权创建并推送 `v1.0.0`。

当前状态：`v1.0.0` 未创建，外部发布未授权。即使所有本地检查通过，也不得自动创建标签、推送 main、推送镜像或更新 `latest`。

## 设计 MUST 验收映射

下表把已确认设计的每组 MUST/MUST NOT 映射到可重复自动测试或发布前人工检查。最终执行记录必须引用本表对应门禁，不能用“功能看起来可用”代替证据。

| 设计范围 | 自动测试证据 | 发布前人工/运维检查 |
|---|---|---|
| 1-4 产品边界、五项导航、全局搜索、详情信息顺序、首页最多两个横向区域 | `App.test.tsx`、`SearchDialog.test.tsx`、`MediaDetailsPage.test.tsx`、`HomePage.test.tsx`、Playwright accessibility/visual | 三个固定视口核对一级导航、搜索入口、详情个人记录顺序和首页区域数量；确认无求片、下载、公开社区入口 |
| 5 领域身份、五态、评分精度、私有标签/片单、不可变事件、共同观看、来源优先级 | `internal/media`、`internal/records`、`internal/household`、`CollectionManager/Picker`、`QuickRecordForm`、`RecordTagsEditor`、`RecordSharingEditor` 测试 | 用两个合成成员核对彼此私有记录；确认公开评分/短评只显示主动公开字段 |
| 6 快速记录、重看、单集/连续/整季、首页下一集撤销、破坏性确认 | `QuickRecordForm.test.tsx`、`EpisodeProgress.test.tsx`、`HomePage.test.tsx`、`MediaDetailsPage.test.tsx`、record/progress HTTP 测试 | 键盘执行想看、看过、重看、首页下一集、删除事件确认；确认删除事件不删除评分、笔记和标签 |
| 7 异常、幂等、ETag、表单保留、自定义条目和以后关联 TMDB | HTTP idempotency/security/contract、SearchDialog、TMDBLinker、QuickRecordForm、sync candidate 测试 | 断网/冲突时核对输入仍在；用合成自定义条目关联 TMDB 后核对个人记录不变 |
| 8 视觉系统、亮暗主题、响应式、状态非颜色、组件状态、reduced-motion | Playwright accessibility/visual、axe、组件键盘与焦点测试 | `375x812`、`768x1024`、`1440x900` 两主题检查溢出、遮挡、海报比例、按钮文字和 200% 缩放 |
| 9-10 单体架构、版本 API、认证、CSRF、Argon2id、会话、服务端 TMDB、AEAD 凭据 | `go test ./... -race`、httpapi contract/security、auth、integrations credential tests、gitleaks | 检查前端 bundle/API/日志/导出/备份不含凭据；设置页只显示 TMDB 配置状态 |
| 11 TMDB 缓存、SQLite、Provider 调度、候选核对与数据优先级 | tmdb cache/client、storage、Provider conformance、sync scheduler/matcher/candidate tests | 断开外部网络后核对本地记录可读写；观察至少一个增量同步周期不阻塞 UI |
| 12 健康、Online Backup、自动迁移前备份、原子恢复与失败回滚 | storage backup/restore/migrate、`recovery-smoke.sh`、backup E2E | 执行合成 backup/restore 演练并记录文件名、字节数、SHA-256；核对备份失败时不开始迁移 |
| 13 Docker、非 root、只读根、唯一可写卷、双架构、SBOM/provenance | container policy/smoke、image secret scan、Scout、manifest/release 脚本测试 | amd64/arm64 分别运行完整 smoke；外部发布后验证双平台 manifest、SBOM 和 provenance |
| 14-15 v1 范围与隐私 | 全部领域、HTTP、前端和 E2E 门禁；家庭可见记录策略测试 | 逐项核对 v1 功能清单；管理员不能读取其他成员私密笔记；无非目标依赖或页面 |
| 16-17 测试、性能、可靠性与发布门槛 | coverage gate、perf/recovery、race/vet、E2E/axe、npm audit、govulncheck、gitleaks、Scout | 所有 checkbox 与实际摘要完成后才可请求外部发布授权；任何 Critical/High 或阻断 a11y 违规均停止发布 |

明确的 v1 非目标同样是验收约束：最终依赖、路由和界面不得包含 Radarr、Sonarr、下载器、媒体文件管理、公开社区、OAuth、2FA、PostgreSQL、Redis、独立 worker 或 webhook 实时同步。

## 本地执行记录

Task 27 完整门禁执行后在此记录命令输出摘要、容器 digest、平台、镜像大小、备份归档 checksum 和未执行的外部步骤。所有记录使用合成数据，不粘贴真实凭据或完整私有日志。

- Go 1.26.5：`go test ./... -race -count=1`、`go vet ./...`、迁移测试和 govulncheck 通过；可达漏洞 0。
- 精确覆盖率：records 85.0622%、media 86.5546%、stats 85.1485%、household 86.6667%、storage 85.4195%、integrations 88.0000%、sync 85.1093%。
- 前端：`npm ci`/lint/typecheck/API check/build/audit 通过；23 个测试文件、44 项测试通过，npm 漏洞 0，主应用 chunk 369.45 kB。
- E2E：9/9 通过；axe 阻断违规 0，键盘、200% 缩放、reduced-motion 和三视口双主题快照通过。
- 性能：calendar p95 51.561292ms，library p95 10.752083ms，初次 10,000 条同步 4.702336959s，增量同步 46.776583ms。
- 恢复：普通写入、迁移、同步和 Online Backup 四个中断点全部通过；真实 Playwright 备份恢复演练通过。
- Compose：使用新项目、新卷、随机 32-byte key 和 `"127.0.0.1:18080:8080"` 端口映射完成 health/ready、前端访问与首位管理员初始化；临时容器和卷已清理。
- 安全与供应链：actionlint、workflow policy、release/manifest/image scan 脚本、shell syntax、工作树/29 个提交 gitleaks 均通过。
- 容器：amd64/arm64 分别完成构建、完整烟测、层密钥扫描和 Scout；两个平台均为 17 个包、`0C/0H/0M/0L`。
- 未执行：未创建 Git 标签，未 push `main`、镜像或 manifest，未生成外部 SBOM/provenance，未配置或使用 Docker Hub 凭据。

# video-record 工作进度

## 2026-07-13

- 已读取适用的调研与设计工作流。
- 已确认仓库为空、当前分支为 `main` 且尚无提交。
- 已初步列出 `invoice-manage` 的工程结构。
- 已创建持久化调研计划、发现记录和进度记录。
- 已确认参照项目的 React/Vite + FastAPI 技术栈、单应用多阶段镜像、Compose 依赖服务和容器安全基线。
- 已确认新项目的默认设计注册类型为 `product`。
- 已确认产品采用家庭多用户架构，同时保持单人默认体验。
- 已记录技术栈必须独立选型；`invoice-manage` 仅作为 Docker 交付参照。
- 已确认手动记录为主流程，Jellyfin/Emby/Plex 播放历史同步为可选能力。
- 已确认 TMDB 是电影、剧集搜索和元数据来源，并要求本地记录与其可用性解耦。
- 已确认产品只做观影记录，排除求片、下载和媒体资源管理。
- 已完成 MoviePilot v2、Jellyseerr/Seerr 的第一轮公开资料核对并写入发现记录。
- Seerr 静态图整页截图超时，已切换证据获取方式。
- 已完成已阅、Apollo、Filmo 的功能与记录逻辑核对。
- 已形成五类竞品的横向取舍结论。
- 已确认视觉方向与无障碍基线，并写入 `PRODUCT.md`。
- 已完成三种工程维护模型的比较，推荐 Go + React/Vite + SQLite 的单容器模块化单体。
- 用户已确认推荐技术方案。
- 已记录 TMDB 令牌的服务端注入、前端隔离和日志/导出脱敏规则，未保存令牌值。
- 第一章信息架构与页面边界已获用户确认。
- 已提出第二章领域数据模型，等待确认。
- 第二章领域数据模型已获用户确认。
- 已提出第三章核心交互与异常处理，等待确认。
- 第三章核心交互与异常处理已获用户确认。
- 已运行品牌种子调色流程、计算关键色彩对比度，并提出第四章视觉与响应式规范。
- 第四章视觉与响应式规范已获用户确认。
- 已提出第五章模块架构、API、安全、SQLite、缓存同步、备份与 Docker 多架构发布规范。
- 第五章技术架构、运维与发布已获用户确认。
- 已提出第六章 v1 范围、家庭隐私、测试矩阵、性能指标和发布门槛。
- 第六章和整套设计已获用户确认。
- 已生成正式设计文档 `docs/plans/2026-07-13-video-record-design.md`，等待校验和提交。
- 正式设计文档及调研上下文已提交到 `main`，提交为 `d0ee509`。
- 已生成 `docs/plans/2026-07-13-video-record-implementation.md`，等待校验和提交。
- 实施计划已完成校验：27 个任务、118 个明确步骤，不含凭据值或串联命令。

## 当前状态

设计与实施计划均已完成，正在 `main` 按实施计划执行。

## 实施记录

### Task 1：仓库工具链与最小服务器

- 已确认初始工作区为干净的 `main`，未创建分支或 worktree。
- 已先写 `TestHealthz`；补齐测试依赖后，测试按预期因 `NewRouter` 与 `Dependencies` 缺失而失败。
- 已添加 Go 1.26 模块、Chi v5、Testify、`/healthz`、带读写和空闲超时的最小服务器、Makefile、`.env.example` 与 `.gitignore`。
- `.env.example` 只有空值或合成值；`.gitignore` 覆盖本地环境、数据、构建、覆盖率和 Playwright 产物，并预留忽略 `.tmdb-token`。
- 定向测试 `go test ./internal/httpapi -run TestHealthz -v` 通过。
- 完整验证 `go test ./...`、`go vet ./...` 与 `git diff --check` 通过。

### Task 2：配置、日志与请求 ID

- 已先写配置测试，确认因 `Load` 和稳定错误缺失而失败，再实现默认值、环境覆盖、生产 Secure Cookie 和 32 字节 Base64 加密密钥校验。
- 已先写中间件测试，确认请求 ID、日志工厂、请求日志和恢复器缺失，再实现对应能力。
- 请求 ID 现在同时进入上下文、`X-Request-ID` 响应头、结构化日志和 Problem Details。
- 生产环境使用 JSON `slog`，本地使用文本 `slog`；Authorization、Cookie、已知 TMDB/媒体令牌及敏感属性会被替换为 `[REDACTED]`。
- panic 恢复返回通用 `500`，不输出 panic 内容、堆栈或秘密值；请求日志不记录查询参数。
- 服务入口已使用环境配置端口和安全 logger，`.env.example` 继续只包含空值或合成值。
- 定向验证 `go test ./internal/config ./internal/httpapi -race` 通过。
- 完整验证 `go test ./... -race`、`go vet ./...`、`git diff --check` 与通用 JWT 形态扫描通过。

### Task 3：SQLite 连接与嵌入式迁移

- 已先写真实临时 SQLite 测试，确认 `Open`、`Migrate` 和迁移校验错误缺失后再实现。
- 已添加 modernc SQLite、单写/有界多读连接池、外键、WAL、5 秒 busy timeout 和 `0700` 数据目录创建。
- 已嵌入 `0001_core.sql`，迁移使用单项事务并保存版本、名称、SHA-256 和应用时间；重复执行幂等，已应用内容变化会失败。
- 已先写 `/readyz` 测试，覆盖无存储、未迁移、已迁移和已关闭四种状态，再实现只暴露通用错误的 readiness。
- 服务启动现在先打开 `/data/video-record.db` 并执行迁移，迁移失败时不会监听业务端口。
- 定向验证 `go test ./internal/storage ./internal/httpapi -race` 通过。
- 完整验证 `go test ./... -race -count=1`、`go vet ./...`、`git diff --check` 与通用 JWT 形态扫描通过。

### Task 4：用户、密码、会话与封闭初始化

- 已先写 Argon2id 格式、正确/错误密码和畸形哈希测试，再实现固定参数、随机 salt 与常量时间验证。
- 已先写真实 SQLite 服务测试，覆盖首位管理员、初始化关闭、会话哈希、登录轮换、失败限流、过期、撤销和 last-seen 节流。
- 已添加 `0002_auth.sql` 以及用户、会话和登录失败桶仓储；会话/CSRF 明文不入库，登录桶不保存原始用户名/IP 组合。
- 已先写 HTTP 测试，再实现 setup status/admin、login/logout/me 五个端点、条件 Secure Cookie、同源 Origin 和 CSRF 防护。
- 第二次初始化返回 `409 initialization_closed`；无效会话和凭据使用稳定 Problem Details 代码，错误响应不包含内部存储信息。
- 服务启动已装配 SQLite 认证仓储，未开放自行注册入口。
- 定向验证 `go test ./internal/auth ./internal/httpapi -race` 通过。
- 完整验证 `go test ./... -race -count=1`、`go vet ./...`、`git diff --check` 与通用 JWT 形态扫描通过。

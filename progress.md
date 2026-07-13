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

设计与实施计划均已完成，正在 `main` 按实施计划执行；当前从 Task 11 继续。

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

### Task 5：前端基础与设计令牌

- 已先建立 React 19/Vite 7/Vitest/strict TypeScript 测试脚手架并写 App shell 角色测试，确认因 `App.tsx` 缺失而失败。
- 首次 npm 安装发现 plugin-react 6 仅支持 Vite 8；核对 peer dependency 后固定为支持 Vite 7 的 `5.2.0`，原命令重跑成功。
- 已实现五项主导航、全局搜索、桌面/平板侧栏、移动底栏、首页空状态和稳定尺寸的响应式应用壳。
- 已实现设计文档规定的 OKLCH 亮暗主题、固定字号/间距/圆角、可见焦点、44px 触控目标、150-200ms 反馈和 reduced-motion。
- 首次 typecheck 暴露 Vite CSS 与 Vitest 配置类型缺失；逐项补入 `vite/client` 并改用 `vitest/config` 后通过。
- 浏览器验证覆盖 `1440x900`、`768x1024`、`375x812`：无横向溢出、无布局重叠、无控制台错误，移动搜索动作正确聚焦搜索框。
- 定向测试 `npm --prefix web test -- --run src/app/App.test.tsx` 已按红绿顺序通过。
- 完整验证前端 typecheck/test/build、npm audit、全仓 Go race/vet、`git diff --check`、OKLCH 色值检查与凭据扫描全部通过。

### Task 6：TMDB 客户端、归属说明与安全缓存

- 已先写伪上游适配器测试，覆盖 Bearer 认证、日志/错误脱敏、搜索 6 小时 TTL、详情 7 天 TTL、电影/剧集/季/集、429 Retry-After 和可缩短验证的默认 8 秒超时。
- 已添加 `0003_tmdb_cache.sql`、SHA-256 缓存键和类型化缓存；缓存先于令牌检查，过期条目删除后再请求上游。
- 安全审阅发现成功响应原始 JSON 可能携带未知敏感字段；已先写失败回归测试，再改为只缓存重新编码的类型化 DTO。
- 已先写 HTTP 合约测试，再实现会话保护的状态、搜索、电影、剧集、季、集端点和 camelCase DTO；上游正文不进入响应。
- 已先写 `TmdbStatus` 和 Settings 归属声明测试，再实现图标+文字状态、服务端环境变量说明与官方归属外链。
- 生产启动已从 `TMDB_READ_ACCESS_TOKEN` 配置创建客户端，并复用脱敏 logger 与 SQLite 缓存；路由层不接触令牌字符串。
- 定向 race 测试 `go test ./internal/integrations/tmdb ./internal/httpapi -race -count=1` 通过；全部 2 个前端测试文件、4 个测试通过。
- 全仓 Go race/vet、前端 typecheck/test/build、npm audit、补丁和凭据扫描全部通过。
- gitleaks `v8.30.1` 扫描 8 个提交、约 329.56 KB，结果为 `no leaks found`。
- 浏览器实测 Settings 归属声明可见、无横向溢出、无控制台警告或错误。

### Task 7：本地影视目录与外部身份

- 已先写真实 SQLite 领域测试，覆盖本地 UUID、外部身份唯一性、电影/剧集类型区分、自定义字段保护和身份占用冲突。
- 已添加 `0004_media.sql`，包含本地影视、外部身份、季、集、类型和演职员快照表；所有后续业务关系使用本地 UUID。
- 外部刷新只更新外部标题、原名、日期、简介和图片路径，自定义标题/简介通过可空覆盖列保持最高优先级。
- 自定义条目创建、TMDB 关联和身份冲突检查均在 SQLite 单写事务边界内完成。
- 已先写 HTTP 红灯，再实现受会话/Origin/CSRF 保护的 TMDB 创建刷新、自定义创建、关联和本地读取端点。
- HTTP 层只接受 TMDB 类型/ID，外部快照由服务端客户端获取并转为受控 DTO，前端不能伪造外部字段。
- 定向测试 `go test ./internal/media ./internal/httpapi -run 'Test(Media|Upsert|External|Link)' -v` 通过。
- 全仓 Go race/vet、前端 typecheck/test/build、补丁与凭据扫描通过；gitleaks 扫描 9 个提交、约 368.48 KB，零泄漏。

### Task 4：用户、密码、会话与封闭初始化

- 已先写 Argon2id 格式、正确/错误密码和畸形哈希测试，再实现固定参数、随机 salt 与常量时间验证。
- 已先写真实 SQLite 服务测试，覆盖首位管理员、初始化关闭、会话哈希、登录轮换、失败限流、过期、撤销和 last-seen 节流。
- 已添加 `0002_auth.sql` 以及用户、会话和登录失败桶仓储；会话/CSRF 明文不入库，登录桶不保存原始用户名/IP 组合。
- 已先写 HTTP 测试，再实现 setup status/admin、login/logout/me 五个端点、条件 Secure Cookie、同源 Origin 和 CSRF 防护。
- 第二次初始化返回 `409 initialization_closed`；无效会话和凭据使用稳定 Problem Details 代码，错误响应不包含内部存储信息。
- 服务启动已装配 SQLite 认证仓储，未开放自行注册入口。
- 定向验证 `go test ./internal/auth ./internal/httpapi -race` 通过。
- 完整验证 `go test ./... -race -count=1`、`go vet ./...`、`git diff --check` 与通用 JWT 形态扫描通过。

### Task 8：个人状态、评分、标签与片单

- 已先写五态、十分制到 `0-100` 整数、字段来源优先级、乐观版本、私人标签和私人片单领域测试，并在实现前取得缺失类型/行为红灯。
- 已添加 `0005_user_records.sql`，包含个人状态、私有标签关联、私有片单和有序片单条目；所有关系使用本地用户和影视 UUID。
- 已实现状态、评分和笔记的逐字段来源优先级；低优先级输入不能覆盖手工值，相同输入不无意义递增版本。
- 片单跨用户增项测试最初暴露重复项检查绕过所有权的问题；根因是先检查了条目而非片单所有者，现已在事务入口按当前用户验证所有权。
- HTTP 红灯最初因记录依赖缺失无法编译；只补依赖边界后确认路由返回 `404`，再实现会话用户隔离、CSRF、`If-Match`/`ETag` 和稳定冲突响应。
- 新增显式 null 回归测试并观察版本未递增的失败，再实现评分/笔记“省略保留、null 清空”的契约。
- 新增片单响应契约测试并观察大写领域字段泄露的失败，再改为不含用户 ID 的 camelCase DTO。
- `internal/records` 覆盖率从 75.4% 提升到 85.5%，覆盖 nullable 更新、字段省略、无操作版本、输入校验和所有权边界。
- 定向验证 `go test ./internal/records ./internal/httpapi -race -count=1` 通过。
- 完整验证 `go test ./... -race -count=1`、`go vet ./...`、前端 typecheck/test/build、`npm audit` 与 `git diff --check` 通过。

### Task 9：不可变观看事件与幂等

- 已先写领域测试并确认事件 API 缺失，再用最小方法边界取得 `watch events not implemented` 行为红灯。
- 已添加 `0006_watch_events.sql`，包含观看事件、参与者、外部事件唯一索引和带 24 小时过期时间的幂等响应表。
- completed 首次转换现在原子保存状态、事件和参与者；wishlist 不产生事件，重复 completed 不误建重看，显式重看生成独立 UUID。
- 已验证删除最新/最早/全部事件会重算首末观看日期，同时保留评分和私人笔记；跨用户删除返回事件不存在。
- 已先写重复 HTTP 请求测试并确认事件路由 `404`，再实现按用户、方法、路径和请求体哈希绑定的 `Idempotency-Key` 重放。
- 已验证相同键相同请求精确重放首次 `201`、相同键不同请求返回 `idempotency_key_conflict`、缺失键被拒绝、过期响应被清理后可重新执行。
- 过期幂等测试夹具最初把 TEXT 字面量写入 STRICT BLOB 列；根据 SQLite 错误将夹具改为绑定 `[]byte` 后通过，未修改产品逻辑。
- 已验证重复外部事件 ID 被唯一约束拒绝，重复标签和片单增项保持幂等。
- `internal/records` 覆盖率为 85.7%。
- 定向验证 `go test ./internal/records ./internal/httpapi -race -count=1` 通过：`records` 16.057s，`httpapi` 33.365s。
- 完整验证 `go test ./... -race -count=1`、`go vet ./...`、前端 typecheck/test/build、`npm audit`、`git diff --check` 和工作树/历史 gitleaks 扫描通过。

### Task 10：搜索、详情、影库与快速记录界面

- 已按 `impeccable` 产品界面约束复用现有 OKLCH 令牌、固定字号/间距/圆角与响应式壳，没有引入第二套视觉系统。
- 已先写搜索测试并确认组件缺失，再建立最小 dialog/searchbox 取得结果缺失红灯；实现 300ms 防抖、本地先显、TMDB 后合并与明确类型/年份/原名/状态标签。
- 已先写快速记录测试并取得保存、日期、渐进字段、失败保留和冲突重试红灯；实现 RHF + Zod 草稿、默认今天、网络错误保留、ETag 重试和普通状态 10 秒撤销。
- 已先写详情/影库测试并确认占位页缺少数据；实现个人记录优先详情、观看时间线、状态筛选、2:3 海报网格、skeleton、错误和可执行空状态。
- 前端缺失的本地搜索、影库列表和记录读取 API 已先用真实 SQLite HTTP 测试取得 `405` 红灯，再补最小 records 查询和 camelCase DTO。
- completed 写入现在把观看方式传给同一事务创建的观看事件；当前记录读取投影最近事件时间和方式。
- 新增 catalog 后领域覆盖率为 77.9%；补齐当前用户隔离、状态筛选、搜索状态投影、通配符转义和输入校验后达到 86.2%。
- Vite 开发代理支持 `VITE_API_PROXY_TARGET`；浏览器验证时发现 Origin/Host 不一致导致同源保护 `403`，修正代理头后使用纯合成数据完成真实 API 验证。
- 浏览器检查覆盖 `1440x900`、`768x1024`、`375x812`：无横向溢出、无固定区重叠、搜索焦点正确、本地结果可用、控制台无警告/错误。
- 临时浏览器种子页已在验证后删除，未进入 Git；未使用或创建 TMDB 令牌文件。
- 全仓 Go race/vet、`internal/records` 86.2% 覆盖率、前端 typecheck/6 文件 13 测试/build、npm audit、补丁和 secret scan 全部通过。

### Task 11：剧集进度与系列状态投影

- 已先写领域测试并确认 `UpdateEpisodeProgress`、动作类型与进度 DTO 缺失，再实现单集、连续范围、整季、下一集和撤销。
- 已添加 `0007_episode_progress.sql`；每个用户动作在单个 SQLite 写事务内更新进度、观看事件、参与者、系列状态、版本和首末观看日期。
- 新标记单集创建不可变观看事件；重复标记不增加事件或版本；撤销只删除该进度对应事件。通用删除单集事件也会重新投影系列状态，电影事件规则不变。
- 系列投影在部分已看时建议 `watching`、全部已看时建议 `completed`；来源优先级继续生效，用户显式 `dropped` 不会被自动改回。
- 已先写 HTTP 测试并取得新端点 `404` 红灯，再实现当前用户隔离的 GET/POST、camelCase DTO、ETag、Origin、CSRF 和 `Idempotency-Key` 重放。
- 已先写组件测试并确认 `EpisodeProgress` 缺失，再实现 `S02E03` 与全剧累计集数双表达、下一集、撤销、单集、整季、连续多集、加载/空/错误状态和键盘可用的标准控件。
- `internal/records` 全包语句覆盖率为 85.1%；全仓 `go test ./... -race -count=1` 与 `go vet ./...` 通过。
- 前端 typecheck、7 个测试文件 15 个测试、build 与 npm audit 通过；工作树和 13 个提交的 gitleaks 扫描零泄漏。
- 浏览器使用合成数据验证 `375x812` 与 `1440x900`：无横向溢出，下一集 0/6 -> 1/6、撤销 1/6 -> 0/6，控制台无 warning/error；临时登录页与本地服务已删除/关闭。

### Task 12：日历与时间线查询

- 已先写领域测试并确认 `CalendarMonth` 与过滤类型缺失，再实现按请求时区计算本地月边界并转换为 UTC 半开区间的查询。
- 日历只返回当前用户作为参与者可见的事件；同日重复观看不去重，共同观看者用户名随事件返回，其他成员未参与的事件保持隔离。
- 已覆盖看完/进行中筛选、电影与单集事件、季集编号、全剧累计集数、观看方式、上海时区月初/月末边界和无效时区/月份/筛选。
- 已先写 HTTP 测试并取得 `/api/v1/calendar` 的 `404` 红灯，再实现会话保护、严格月份解析、camelCase DTO 和稳定 `invalid_calendar_query` Problem Details。
- 已先写日历组件测试并确认页面缺失，再实现桌面 6x7 语义月表、移动按日议程、月份切换、三态筛选、重复事件、共同观看文案、加载/空/错误与保留选择的重试操作。
- `internal/records` 全包语句覆盖率为 85.8%；全仓 Go race/vet、前端 typecheck、8 个测试文件 18 个测试、build、npm audit、补丁和 gitleaks 均通过。
- 浏览器合成数据验证 `1440x900` 显示 42 格月表和同日两条记录，`375x812` 切换议程并显示 `S01E03`/累计集数/共同观看者；两端无横向溢出，筛选可用，控制台无 warning/error。
- 临时浏览器登录页和本地服务已在验证后删除/关闭。

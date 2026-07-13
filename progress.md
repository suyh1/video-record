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

设计与实施计划均已完成，正在 `main` 按实施计划执行；Task 21 已完成，下一步从 Task 22 继续。

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

### Task 13：可访问统计

- 已先写真实 SQLite 统计测试并确认 `internal/stats` 服务/仓储缺失，再实现当前用户隔离的观看事件、月度、年度、类型、评分、时长、标签、观看方式和重看聚合。
- 新增 `0008_media_runtime.sql` 补齐电影分钟时长；统计优先使用单集 runtime，其次使用电影 runtime，并按请求 IANA 时区划分月/年。
- 静态审阅发现 TMDB 入库丢弃 runtime/genres；已先扩展媒体 HTTP 夹具取得响应字段缺失红灯，再实现受控 DTO、同事务 runtime/genre 替换和详情响应。
- 已先写 stats HTTP 测试并确认 Router 依赖缺失，再实现会话用户注入、时区校验、camelCase DTO 和稳定 `invalid_stats_query` Problem Details；生产入口已装配统计服务。
- 已先写 StatsPage 测试并确认组件缺失，再实现无卡片概览、六个文字/数值条形图、六张可见语义数据表、加载/空维度/错误重试和移动双列响应式布局。
- 关键领域覆盖率：`internal/stats` 85.1%、`internal/media` 86.6%、`internal/records` 85.8%。
- 全仓 Go race/vet、前端 typecheck、9 个测试文件 19 个测试、build、npm audit、补丁和 gitleaks 均通过。
- 浏览器合成数据验证 `1440x900` 与 `375x812`：6 个图表和 6 张数据表均存在，概览/条形/表格无裁切或横向溢出，控制台无 warning/error。
- 临时浏览器登录页和本地服务已在验证后删除/关闭。

### Task 14：家庭成员与隐私边界

- 已先写家庭策略与真实 SQLite 服务测试，确认成员管理、私密笔记隔离、显式评分/短评分享、共享事件最小 DTO 和会话撤销能力缺失后再实现。
- 已添加 `0009_household_privacy.sql`，为个人记录增加显式家庭分享字段，并创建不保存笔记正文、Authorization 或凭据的审计事件表。
- 活跃管理员可以创建、重置密码和停用普通成员；重置与停用会在同一事务撤销目标成员会话，管理员不能读取其他成员私人笔记或修改其分享设置。
- 已先写共同观看领域、HTTP、表单和详情页红测，再实现活跃参与者列表、创建者自动包含、参与者去空/去重及事件与参与者同事务写入。
- 快速记录完整层使用标准可访问复选框选择共同观看者；详情页通过 TanStack Query 读取当前用户之外的活跃成员，移动端固定保存区与底部导航保持分离。
- 成员设置复用现有产品视觉系统，使用 Radix Dialog 承载停用/重置确认；破坏性对话框打开后焦点落在确认按钮，普通成员看不到管理界面。
- 关键领域覆盖率：`internal/household` 85.8%、`internal/records` 85.9%。
- 完整验证 `go test ./... -race -count=1`、`go vet ./...`、前端 typecheck、10 个测试文件 21 个测试、build、npm audit、`git diff --check` 和 gitleaks 均通过。
- 浏览器使用纯合成数据验证 `1440x900` 与 `375x812`：设置页和共同观看表单无横向溢出，对话框焦点正确，移动保存区不遮挡 65px 底部导航，控制台无 warning/error。
- 临时登录页、SQLite 数据、cookie 和本地服务均已删除/关闭；未创建 `.tmdb-token`，未使用或保存真实 TMDB 凭据。
- Task 17 的集成迁移编号顺延为 `0010_integrations.sql`。

### Task 15：JSON/CSV 导出与安全导入

- 已先写 JSON 往返、CSV 公式注入、用户隔离、超大文件、非法 UTF-8、路径型文件名、重复外部身份与部分失败报告红测，再实现版本化数据包。
- JSON 导出覆盖影视快照、外部身份、类型、季集、个人状态与分享设置、标签、观看事件、单集进度和片单；查询始终从认证用户 ID 过滤，不包含其他成员私密笔记或任何认证/集成凭据。
- JSON 导入严格限制 10 MiB、UTF-8、版本和未知字段；影视记录逐项事务导入，单条失败不会回滚已成功记录，报告使用稳定错误码。
- CSV 导出对危险公式首字符统一加单引号，CSV 导入只还原本系统保护值，并以 `confirmed_import` 恢复评分、日期、笔记和标签。
- 安全复核先取得越权红测，再禁止个人导入覆盖既有全局媒体快照、外部身份或季集，并拒绝接管其他成员已拥有的片单 ID。
- HTTP 下载使用固定服务端文件名、`Content-Disposition` 与 `nosniff`；上传通过流式 multipart 读取，不调用可能把文件落盘的 `ParseMultipartForm`，并复用会话、Origin、CSRF 与幂等中间件。
- 设置页新增 JSON/CSV 下载和显式文件导入，错误时保留已选文件，部分失败以记录 ID 和中文原因显示；未选文件时按钮禁用。
- `internal/records` 语句覆盖率为 85.3%；全仓 Go race/vet、前端 typecheck、11 个测试文件 22 个测试、build、npm audit、补丁和 gitleaks 均通过。
- 浏览器合成环境验证 `1440x900` 与 `375x812`：下载/文件选择/导入控件无横向溢出，移动输入与按钮占满可用宽度，控制台无 warning/error。
- 临时登录页、SQLite 数据和本地服务已删除/关闭；未使用或创建 TMDB 令牌文件。

### Task 16：一致性备份与原子恢复

- 已先写 Online Backup 一致性、manifest/checksum、版本/空间/路径穿越、预恢复快照、强制失败回滚和缺少加密密钥红测，再实现备份管理器与恢复流程。
- 备份创建、ZIP 写入、列表 manifest 读取、下载、multipart 上传和数据库条目解压均使用流式文件 I/O；有效大于 1 MiB 的恢复归档不再被通用幂等请求体上限拒绝。
- 恢复通过数据库级锁串行；维护闸门等待活动 API 请求归零，替换阶段使用独立有界上下文，任何恢复清理和重开使用全新的恢复上下文。
- 规范数据库路径在提交期间始终存在：旧库先建立硬链接，替换库通过单次 rename 原子覆盖；新库重开与审计提交失败会原子恢复旧库，提交后的旧链接清理失败不再误报恢复失败。
- 已覆盖客户端取消、关键阶段超时、两个并发恢复、跨安装用户 ID、审计提交失败、符号链接、畸形 ZIP、迁移校验和篡改、manifest/数据库版本不一致和真实 N-1 升级。
- 管理端 API 提供创建、列表、流式下载和恢复；普通成员、缺失 Origin/CSRF/幂等键均被拒绝，恢复动作写入不含凭据的 `backup.restore` 审计事件。
- 设置页新增管理员专用备份列表、创建/下载、文件选择和破坏性恢复确认；对话框焦点落在确认按钮，错误保留文件并通过 alert 播报，缺少密钥显示集成锁定警告。
- `internal/storage` 精确语句覆盖率为 85.1351%；最终全仓 `go test ./... -race -count=1`、`go vet ./...`、前端 12 个测试文件 24 项测试、typecheck/build、npm audit、`git diff --check` 与工作树/历史 gitleaks 均通过。
- 浏览器使用合成管理员和真实 API 验证 `1440x900` 与 `375x812`：无横向溢出，移动底部导航不遮挡恢复控件，控制台无 warning/error；临时登录页、数据库和服务已清理。
- 两轮独立代码审阅确认原子性、取消恢复、并发串行、流式 I/O、空间检查和快照 manifest 修复后无剩余 Critical/Important。

### Task 17：Provider 契约、加密账户与持久化调度器

- 已先写 Provider/凭据/调度红测并确认接口、加密账户仓储、迁移和调度服务缺失，再实现统一 Provider 契约、AES-256-GCM 凭据和 `0010_integrations.sql`。
- 加密账户创建先加密再写库；只返回元数据、指纹和锁定状态。缺失/更换密钥、篡改密文和跨用户读取均有真实 SQLite 测试，基础 URL 的明文凭据旁路已通过红测封堵。
- 持久化调度包含 15 分钟增量、每日补偿、重启补跑、原子领取、默认 UUID owner、过期租约拒绝提交和新实例重新领取。
- 成功运行更新 job/run 游标与受控 JSON 摘要；失败运行不推进游标、不保存原始错误，Provider 临时失败不会终止后续轮询。
- 正式 `external_accounts` 迁移使 Task 16 两条占位表测试失败；已改用满足 STRICT/外键约束的正式合成数据，并重新验证快照时点的加密密钥要求。
- Task 17 定向 race 通过；覆盖率为 `internal/integrations 91.7%`、`internal/sync 87.1%`，均高于关键领域包 85% 基线。
- 全仓 `go test ./... -race -count=1` 与 `go vet ./...` 通过；前端 12 个测试文件 24 项测试、typecheck、build 和高危依赖审计通过。
- 未创建 `.tmdb-token`，未使用真实 TMDB 或媒体服务器凭据；所有测试数据均为显式合成值。

### Task 18：Jellyfin 播放历史 Provider

- 已先写 Jellyfin 合成 Provider 测试并确认客户端缺失，再实现认证检查、Playback Reporting 分页历史和标准 Item 元数据映射。
- 共享 conformance suite 覆盖认证、稳定分页、取消、错误脱敏与重试；额外覆盖电影、单集、同一电影重复播放、删除用户、日期范围、超时、429、5xx、畸形/null 响应和非法映射。
- 核对官方 Jellyfin OpenAPI 与 Playback Reporting 插件源码后，确认核心 API 不具备重复播放事件历史；实现使用真实 `/user_usage_stats/{userId}/{date}/GetItems`，filter 为 `Movie,Episode`，时间形状为 `h:mm tt`。
- 插件 `RowId` 生成稳定 `jellyfin:{rowId}` 事件 ID，持久游标为 `date|rowId`；同一页重复条目复用 Item 详情，类型化响应不会保存未知上游字段。
- 安全边界拒绝 null 历史、负 runtime、非法 RowId、类型/条目不匹配和 malformed cursor；请求只通过 `X-Emby-Token` 发送合成测试令牌，不使用 query token。
- 包级 `go test ./internal/integrations/jellyfin ./internal/integrations -race -count=1` 通过，`internal/integrations/jellyfin` 精确语句覆盖率为 93.1%。
- 首次全仓 race 发现调度器取消竞争可能返回 SQLite 底层错误；已让父 context 优先决定关停结果，定向 race 连续 20 次和最终全仓 race 均通过。
- 最终 `go test ./... -race -count=1`、`go vet ./...`、前端 typecheck、12 个测试文件 24 项测试、build 与高危依赖审计通过。
- 未创建 `.tmdb-token`，未接触真实 Jellyfin/TMDB 凭据；全部 JSON 夹具均为合成数据。

### Task 19：Emby 播放历史 Provider

- 已先写 Emby 专用合成测试并确认客户端缺失，再实现 `/emby` 基路径、认证、Playback Reporting wrapper 解码和标准 Item 映射。
- 共享 conformance suite 覆盖认证、稳定分页、取消、错误脱敏和重试；额外覆盖电影、单集、重复播放、日期/游标、删除用户、wrapper 用户不匹配、null activity、畸形 Item、429、5xx 和超时。
- Emby 插件响应中的 `HH:mm` 使用配置的服务器 `time.Location` 解释并转 UTC；`+08:00` 测试确认 `09:15` 正确保存为 `01:15Z`，请求日期按服务器本地日历计算。
- 稳定事件 ID 使用 `emby:{rowId}`，游标使用 `date|rowId`；typed DTO 不读取或传播历史 `RemoteAddress` 与媒体文件 `Path`。
- 包级 `go test ./internal/integrations/emby ./internal/integrations -race -count=1` 通过，`internal/integrations/emby` 精确语句覆盖率为 87.4%。
- 最终 `go test ./... -race -count=1`、`go vet ./...`、前端 typecheck、12 个测试文件 24 项测试、build 与高危依赖审计通过。
- 未创建 `.tmdb-token`，未使用真实 Emby/TMDB 凭据；全部响应和 token 均为合成测试数据。

### Task 20：Plex 播放历史 Provider

- 已先写 Plex XML/JSON、分页、稳定事件 ID、ratingKey、重复播放、重复事件拒绝、错误分类和 conformance 红测，再实现核心历史适配器。
- 认证使用 `/library/sections` MediaContainer；历史使用 `/status/sessions/history/all`，token 只走 `X-Plex-Token` header，query 中显式验证不存在 token。
- XML/JSON 两种响应映射为同一受控 DTO；容器 offset/size/totalSize、null envelope、错误根节点、格式损坏、缺失 historyKey 和重复事件均有回归测试。
- `historyKey` 生成唯一 `plex:{historyKey}` 事件 ID，`ratingKey` 保存为 ProviderItemID；现代和旧版 Plex agent GUID 均映射到 TMDB/IMDb/TVDB。
- 合成 JSON 特意包含私有 Media/Part 文件路径，typed DTO 不读取或传播该字段；上游正文和 token 不进入错误。
- 包级 `go test ./internal/integrations/plex ./internal/integrations -race -count=1` 通过，`internal/integrations/plex` 精确语句覆盖率为 91.2%。
- 最终 `go test ./... -race -count=1`、`go vet ./...`、前端 typecheck、12 个测试文件 24 项测试、build 与高危依赖审计通过。
- 未创建 `.tmdb-token`，未使用真实 Plex/TMDB 凭据；全部 XML/JSON 与 token 均为合成数据。

### Task 21：候选匹配、冲突解决与同步界面

- 已先写真实 SQLite 匹配测试并确认 matcher/candidate 服务缺失，再实现已确认映射、TMDB/IMDb/TVDB 精确匹配、标题/原名+年份可能匹配、类型冲突、手工优先级冲突和无法匹配。
- 精确且无冲突候选自动落档；确认、重匹配、自定义条目在同一事务写入映射、观看事件、参与者、剧集进度、个人投影、候选状态与审计，忽略只更新候选与审计。
- 已覆盖同账户重复外部事件、跨账户事件 ID 命名空间、重复确认、已解析改指拒绝、电影/剧集目标类型和单集归属校验；注入时钟贯穿全部同步投影时间戳。
- 安全审查先取得“一个外部 ID 匹配电影、另一个指向剧集仍自动落档”的失败测试，再改为任一类型冲突都阻止自动应用。
- 已先写生产 runner 失败测试，再实现加密凭据运行时解密、三 Provider 严格凭据工厂、分页 HistoryPage 校验、24 小时补偿窗口、持久游标与受控运行摘要。
- `cmd/server` 已通过测试装配候选服务、已有启用账户任务初始化和后台调度器；路由新增当前用户隔离的 status/candidates 与四个受 Origin/CSRF/幂等保护的写端点。
- 已先写 SyncStatus/CandidateReviewPage/App 路由红测，再实现设置摘要、独立核对页、加载/空/错误重试、证据、非颜色状态、确认/重匹配/忽略/自定义和失败保留输入。
- 浏览器验收发现同名同年 radio 名称重复；先补失败测试，再让受控匹配选项携带规范化原名，并在 UI 标签加入候选序号与可选原名。
- 真实 Go+Vite+SQLite 合成环境在 `1440x900`、`768x1024`、`375x812` 验证无横向溢出；自定义表单焦点正确，四种写入均使待核对数量递减，控制台无 warning/error，临时登录页、种子、数据库和服务已清理。
- 最终全仓 `go test ./... -race -count=1`、`go vet ./...`、`internal/sync` 85.1% 覆盖率、前端 14 文件 29 测试/typecheck/build、npm 高危审计、`git diff --check` 与工作树/23 提交 gitleaks 均通过。
- 未创建 `.tmdb-token`，未使用真实 TMDB 或媒体服务器凭据；浏览器与测试全部使用明确合成值。

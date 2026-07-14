# video-record 工作进度

## 2026-07-14：详情与记录页改造

- 已确认在 `main` 分支工作，起点 `121c6ed`，工作树无未提交改动。
- 已阅读用户提供的当前页面与 TMDB 参考截图，记录四项核心问题：单调头图、剧集季/集进度不可用、缺少演员、家庭设置抢占主流程且宽屏利用不足。
- 已确认现有项目包含 `EpisodeProgress` 以及对应领域/API，下一步审计详情页和 TMDB 数据契约后提交设计方案。
- 已定位剧集进度空态根因：TMDB 创建/关联只保存剧集级快照，从未把现成的季/分集接口结果写入 `seasons` 与 `episodes`。
- 已确认演员适合新增 TMDB credits 代理并沿用服务端缓存，不落本地；详情页布局改造可在现有 React/CSS 体系内完成。
- 用户确认已有剧集自动补齐，并要求持续获取 TMDB 更新；已将方案收敛为本地先显、约 6 小时新鲜度判定、页面打开异步刷新及失败可重试。
- 完成代码与数据链路审计：主内容容器已有 1440px 上限，不需要改应用壳；宽屏浪费来自详情页自身的单薄 header、720px 历史限制和线性 section 排布。
- 已准备三种刷新方向及推荐的 stale-while-revalidate 方案，待用户确认整体设计后进入设计文档和实施计划。
- 用户否决完整季集入库并确认最小离线兜底边界；已将设计调整为 TMDB 数据加载时按需获取、6 小时缓存，本地只保存项目数据和被标记分集的最小身份。
- 已写入 `docs/plans/2026-07-14-media-details-redesign-design.md`，覆盖数据边界、接口、页面结构、失败降级和测试策略。
- 设计提交 `afd6714` 已写入 `main`；当前正在用 writing-plans 工作流细化实施步骤。
- 已确认不需要重建 watch event 外键模型：仅在用户实际标记分集时创建最小 season/episode 身份桩，并让详情页与首页共用远端季集目录合并逻辑。
- 已写入 `docs/plans/2026-07-14-media-details-redesign-implementation.md`，拆分为 11 个 TDD、契约、界面、E2E 和最终审计任务。
- 第一实施批完成并提交：`1950210` 增加 6 小时 TMDB 实时目录/credits 契约，`89d1c59` 暴露本地可空 `tmdbId`，`9413da1` 扩展受认证 TV/season/credits HTTP 响应。
- 第一批验证：TMDB 集成包全量通过；media/records/httpapi 三包全量通过；TMDB HTTP 定向测试通过。
- 第二实施批完成并提交：`589b9ed` 实现只存被标记分集的稀疏身份/进度及累计集数迁移，`141b4d3` 发布 episodeRefs、sourceId、扩展 TMDB/credits 的 OpenAPI 契约。
- 第二批验证：storage/records/httpapi 全包通过，固定版本 OpenAPI TypeScript 重新生成并通过 `api:check`。
- 第三实施批完成并提交：`ce540d9` 增加 live TV/season/credits 客户端及纯目录合并，`52d48aa` 将剧集进度改为按需单季加载和稀疏写入。
- 第三批验证：目录/API 5 项、剧集组件/目录 7 项测试通过，TypeScript 检查通过。
- 本次会话从详情页 Task 8 的未提交状态恢复；先补齐 1100/900/767px 响应式布局并复验组件与类型，再进入首页下一集的 RED/GREEN 周期。
- 恢复审计确认：详情页组件层已切到 live hero/cast 与双栏记录布局，但新增样式尚无完整窄屏断点；首页仍只读取本地 progress 的 `nextEpisode`，还未合并 TMDB 实时季目录。
- 已补齐详情页 1100/900/767px 响应式规则：窄桌面压缩双栏，平板切个人记录优先的单栏，手机去卡片/粘性并使用 112px 海报与 118px 演员列。
- Task 8 响应式补丁后复验：`MediaDetailsPage.test.tsx` 3/3 通过，`npm --prefix web run typecheck` 退出 0。
- 提交前地标审计发现 App 壳已有 `<main>`，详情主列不可再使用嵌套 `<main>`；已先加入壳内嵌套回归断言，准备完成 RED/GREEN 修正。
- 嵌套主地标红灯已确认：定向测试 1/3 失败并准确返回 `.media-details-primary` 内层 `<main>`；最小修正仅将布局容器改为 `<div>`。
- 嵌套主地标 GREEN：同一定向测试恢复 3/3 通过，详情主列内部的进度与历史 section 语义保持不变。
- Task 8 已提交为 `ca42bf7`；Task 9 审计确认共享 helper 已覆盖默认季、远端/本地合并、下一集和总集数计算，首页只需组织 live 查询及复用稀疏写入。
- Task 9 RED 已确认：Home 定向测试 2/3 失败，分别因稀疏本地目录误显示“已全部看完”和 TMDB 失败时缺少“打开详情继续记录”；失败原因与目标行为一致。
- Task 9 GREEN：Home 定向测试 3/3 通过；live TV/单季目录识别下一集，next/undo 均发送最小 `episodeRefs + totalEpisodes`，TMDB 失败显示详情降级链接，未关联条目保留旧本地路径。
- Task 9 首次 typecheck 仅报 live/legacy 联合类型未随普通布尔值收窄；根因定位后改为在各来源分支使用其原始 episode 值，不使用强制断言。
- Task 9 类型修正后复验：`npm --prefix web run typecheck` 退出 0，Home 定向测试保持 3/3 通过。
- Task 9 提交前门禁：Home + episodeCatalog 6/6 通过，前端 lint 与 `git diff --check` 退出 0；未新增依赖或本地 TMDB 展示数据存储。
- Task 9 已提交为 `b5d1344`；Task 10 初审确认当前 E2E 仍是旧本地剧集目录 + 页面级 TMDB mock，尚不能覆盖后端 live cache、credits/season 代理和稀疏身份落库。
- Task 10 配置审计：现有 TMDB client 已支持 BaseURL，但应用 config/main 未暴露；将先以 Go 配置红测增加 `TMDB_API_BASE_URL` 覆盖，默认生产路径保持不变。
- `TMDB_API_BASE_URL` 配置 RED 已确认：config 定向测试因字段不存在编译失败；最小实现只读取可空覆盖并传给现有 TMDB client。
- `TMDB_API_BASE_URL` GREEN：config 与 `cmd/server` 定向包均通过；E2E runner 现启动带 credits/两季/图片/请求计数的本地 TMDB，上游种子剧集改为零本地季集目录。
- Task 10 新 E2E 断言已覆盖 live hero/cast/season、单集稀疏写入、切季整季、缓存、TMDB 故障记录、三视口溢出/固定元素和详情明暗快照；视口测试显式选择季，避免依赖文件执行顺序。
- Task 10 首轮完整 E2E RED：9/14 通过、4 失败、1 因项目依赖未运行。已通过三视口无溢出/固定重叠、live 单集稀疏写入、6 小时缓存与既有主要旅程；失败为 hero 按钮对比度、折叠整季测试步骤、本地故障简介断言和六张新详情基线。
- 首轮截图复核发现移动端个人记录保存栏仍继承全局 sticky，浮在状态选项上方；故障截图则因测试清理后自动重试已恢复 live 数据，后续断言将避免依赖清理后的画面。
- 768px 与 1440px 浅色详情图已人工检查：单栏/双栏断点、演员条、季选择和历史区均正确使用宽度，无需改变既定布局架构。
- 375px/768px 暗色详情图已检查并定位 hero 遮罩反转：`--ink` 在暗色主题为浅色，造成浅灰遮罩与白字低对比；准备改成主题无关深色遮罩。
- 1440px 暗色图确认同一遮罩缺陷；源码同时发现 `TMDBLinker` 未识别已有 `tmdbId`，导致保留自定义标题的已关联条目仍显示重复关联按钮。
- TMDBLinker 与移动 sticky 红灯均已确认；最小实现按 `tmdbId` 隐藏重复入口、用固定深色 alpha 遮罩 hero，并在 375px 个人记录面板内取消 form actions sticky。
- 修正后 GREEN：TMDBLinker + MediaDetails 组件测试 4/4，通过初始化 + 三视口详情 E2E 2/2；375px form actions 计算样式已为 static。
- E2E 失败项复验中 axe/live 单集/整季/缓存/TMDB 故障保存 6/6 通过；后续录制测试因前一旅程留下笔记导致表单默认展开，已改为验证保存后通过产品撤销恢复状态。
- 录制子集复验中后续正常录制已通过；故障旅程仅因误期待撤销后自动收起失败，现改为断言笔记字段清空以验证记录恢复。
- 撤销字段断言进一步定位真实 UI bug：后端/新页面已恢复空笔记，但当前表单用旧 prop 重置仍显示被撤销文本；将以组件红测要求使用 undo 响应状态重置。
- QuickRecordForm 受控父层回归测试取得 RED（1/7 失败，笔记仍为“临时笔记”）；最小修正改为用 undo 响应 `saved` 重置表单。
- QuickRecordForm GREEN：组件 7/7；初始化 + TMDB 故障保存/撤销 + 正常录制 E2E 3/3，故障恢复不再污染后续旅程。
- 视觉子集 3/3 通过并生成六张详情基线；375px 明暗图已人工复核，暗色遮罩和静态保存栏修正符合预期。
- 768px 明暗详情基线已人工复核：单栏内容顺序、控制尺寸和文字对比符合预期。
- 1440px 检查发现浅色基线因 1% 容差保留了已删除的关联按钮；将用 `--update-snapshots=all` 强制重录，不能把容差内旧图当作批准基线。
- 使用 `--update-snapshots=all` 强制重录后视觉子集 3/3；1440px 浅色旧按钮已消失，六张最终详情明暗基线全部人工复核完成。
- 全前端首轮门禁：Vitest 25 文件/54 测试、lint、api:check、diff-check 通过；typecheck/build 仅因测试 harness state 推断过窄失败，已显式标注 `RecordState`。
- 最终全仓 Go race 已退出 0。完整 E2E 首轮 11/14 通过，失败定位为电影 TMDB 缓存预热影响故障夹具，以及视觉测试在进度写入后运行；已分别改为页面 API 固定失败和 setup 后独立 visual project。
- 完整 E2E 二轮 visual/axe/剧集均通过；两条录制旅程仍假设电影未被家庭旅程标记。已移除完成记录不支持的快速撤销假设，并按字段是否可见决定是否展开。
- 完整 E2E 最终普通模式 14/14 通过（setup 1、visual 2、chromium 10、recovery 1）；全仓 `go test ./... -race -count=1` 也已退出 0。
- 浏览器验收端口 `38082/28081/25173` 均空闲；当前未提交文件均属于 Task 10 与跟踪更新，无意外外部改动。
- 实时浏览器验收已完成：桌面/手机均无横向溢出，desktop 双栏 `712px/400px`，mobile form actions static 且个人记录在进度前，hero 图片非空、季切换与家庭折叠可用、控制台/pageerror 为 0；内置浏览器插件因三次 `process` 重定义错误不可用，使用仓库 Chromium 完成同等检查。
- 最终需求审计逐项通过：全宽 hero/海报、live cast、6 小时 live 季集、仅标记集稀疏落库、季/单集/范围/整季进度、家庭折叠、桌面双栏/移动单栏、TMDB 故障本地记录均有代码与自动化证据。
- 最终前端门禁：Vitest 25 文件/54 测试、typecheck、lint、api:check、build、diff-check 全部退出 0；完整 E2E 14/14、Go race 与 vet 退出 0。

## 2026-07-13：Task 27 恢复与 v1 MUST 审计

- 已重新确认工作分支为 `main`，未创建分支或 worktree；Task 27 工作树改动保留在原位。
- 已完整重读 `PRODUCT.md`、已确认设计、实施计划及三个跟踪文件，并恢复 Task 27 的未提交上下文。
- 已按设计 MUST 重新审计用户可达功能，并以 TDD 补齐标签、家庭公开评分/短评、观看事件删除确认和自定义条目关联 TMDB。
- 已明确最终双架构镜像必须在所有功能改动后重建和独立验证，且不会创建或推送标签、镜像、manifest 或发布产物。
- 已按 RED/GREEN 补齐私有标签编辑、家庭评分/短评公开与查看、观看事件删除确认、自定义条目创建及关联 TMDB、首页下一集推进与撤销。
- 已同步 OpenAPI/生成类型；完整前端回归为 23 个测试文件、44 项测试通过，typecheck 与 API 生成一致性检查通过。
- 已在发布清单加入设计 MUST 分组映射，并在升级文档明确自动迁移前备份失败会阻止迁移开始。
- 已修复真实 API 空标签 `null`、TMDB 未配置状态对比度、200% 缩放溢出和 E2E 定位器歧义；最终 Playwright 9/9 通过。
- 已使用 Go 1.26.5、Node 24.13.0 和当前最终代码完成全部质量、安全、性能、恢复、Compose 与 amd64/arm64 容器门禁，并把实际摘要写入发布清单。
- Task 27 已完成；未创建标签、未 push main/镜像/manifest，也未执行外部发布。

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

设计与实施计划的 27 个任务均已在 `main` 完成；本地 `v1.0.0` release candidate 门禁通过，外部标签、push、manifest、SBOM/provenance 与 Docker Hub 发布仍等待明确授权。

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

### Task 22：OpenAPI 合约与完整 HTTP 安全测试

- 已先写路由/契约与安全红测，覆盖全部 `/api/v1` 操作、RFC 9457、404/405、游标、ETag、幂等重放/冲突/并发、CSRF、会话撤销、对象级授权和日志脱敏，再补齐实现。
- `api/openapi.yaml` 现在提供 46 个具体 operation、请求/响应 schema、查询参数、真实文件媒体类型、ETag、`Idempotency-Key` 与 `X-CSRF-Token`；生成的 TypeScript 不再包含空 operations 或 `unknown` 占位。
- 所有受保护写操作统一要求幂等键；SQLite 原子 reservation 在副作用前占位，相同用户/方法/路径/键的并发竞争只允许一个执行，未完成请求返回稳定 `idempotency_in_progress`。
- 幂等缓存保存原始 request ID、状态、header 和非 null 空响应体；重放 Problem Details 保持最初 request ID，恢复数据库替换后使用新连接完成幂等提交。
- 合法 10 MiB 导入绕过通用 1 MiB 请求缓存，在 handler 内流式计算文件名与内容哈希；备份恢复继续使用归档内容哈希，不缓存完整上传。
- 标签更新纳入 `If-Match` 乐观并发控制：事务内校验版本、写入标签、递增 record version 并返回新 ETag，陈旧写入返回 `412 version_conflict`。
- 前端记录更新与 TMDB 创建请求均发送随机幂等键；API 生成检查会在 OpenAPI 与提交的 `generated.ts` 不一致时失败。
- 二次代码审查确认没有剩余 Critical/Important 问题；调度器 race 测试的偶发 1 秒等待超时经定向 race 连续 20 次排除数据竞争后调整为 3 秒。
- 最终全仓 `go test ./... -race -count=1`、`go vet ./...`、`internal/records` 85.0% 覆盖率、前端 15 文件 30 测试/typecheck/API check/build、npm 高危审计、`git diff --check` 与工作树/历史 gitleaks 均通过。
- 未创建 `.tmdb-token`，未使用真实 TMDB 或媒体服务器凭据；OpenAPI、测试和生成代码均不包含令牌值。

### Task 23：E2E、无障碍与视觉回归

- 已先写 Playwright 完整旅程并取得“缺少首次初始化页”的预期红灯，再实现封闭初始化/登录 AuthGate；公开 setup status 只返回 initialized/storageReady/tmdbConfigured 布尔值，不返回 TMDB 令牌或环境变量。
- 首位管理员创建后立即关闭初始化入口并自动登录；无会话时进入登录页，用户名在失败后保留，CSRF token 只保存在当前标签的 sessionStorage。
- 已先取得组件/E2E 重复观看红灯，再增加显式“再看一次”按钮，通过既有受 Origin、CSRF、Idempotency-Key 保护的不可变事件端点写入，并只刷新观看历史。
- Playwright 使用纯合成管理员、两条影视、三集剧集、成员与同步候选；外部 TMDB/Provider 请求被本地 mock，trace/video 不含真实凭据。
- 9 项 E2E 覆盖初始化、登录、搜索记录、重复观看、剧集推进/撤销、家庭共同观看、同步候选、JSON 导出和真实系统备份恢复；恢复项最后独立执行，在快照后创建成员并验证恢复精确回滚成员名单。
- axe 覆盖 setup、login、首页、影库、日历、统计、设置、同步候选和剧集详情，选择 WCAG 2.0/2.1 A+AA 与 2.2 AA 全部 normative rules；阻断级违规为 0。
- 键盘验收覆盖认证表单焦点、跳到主要内容、五项主导航顺序/激活和搜索对话框打开/关闭；同时验证 200% zoom 无横向溢出和 reduced-motion transition 接近零。
- 浅色主色经 axe 从 4.27:1 修正到 AA；视觉基线覆盖 `375x812`、`768x1024`、`1440x900` 的亮暗主题与真实双列/海报网格，文件名不含主机 OS 后缀。
- E2E runner 每轮使用独立 SQLite 目录，等待 Go/Vite 端口完全释放后清理；审查者在最终树上连续两次独立运行均为 9/9，且没有剩余 Critical/Important 问题。
- 最终全仓 race/vet、前端 16 文件 33 测试/typecheck/API check/build、9 项 E2E、npm 高危审计、`git diff --check` 与工作树/历史 gitleaks 均通过。
- `impeccable` 约束了产品型认证页、对比度和稳定控件；in-app browser 因任务标签会话错配无法导航，按 browser 技能停止重试，真实 Chromium E2E 与六张人工检查的快照完成替代验证。
- 未创建 `.tmdb-token`，未使用真实 TMDB、媒体服务器或用户凭据；所有密码、token、事件和影视数据都是明确合成值。

### Task 24：性能与故障恢复基线

- 已先运行性能和恢复脚本并取得缺少 `internal/testutil` 非测试实现的预期红灯，再实现参数校验、一次 Argon2id 哈希、预编译语句和单事务精确种子。
- 种子覆盖 5 名用户、10,000 部合成电影、10,000 个低优先级个人状态、50,000 条不可变事件与参与者；中途唯一键冲突测试证明用户和前置影视写入全部回滚。
- 首次完整基线暴露整秒 RFC3339Nano 缺少固定九位小数、无法被 records 严格解析的问题；已先加真实日历读取红测，再统一种子时间格式。
- 性能同步使用加密合成账户、每页 200 条的合成 Provider、真实 Scheduler、ProviderRunner、HistoryPage 校验、持久游标、run summary 和候选多表事务。
- 初次 10,000 条同步期间并发测量认证日历/影库 HTTP p95；每个响应都验证精确条目数，并核对候选、事件、参与者、映射、状态来源、运行摘要和游标，空结果或 no-op 不能通过。
- 三轮结果分别为：日历 `54.148416/50.9615/50.499375ms`，影库 `10.69275/10.692542/10.815708ms`，初次同步 `4.483806125/4.586084833/4.447517792s`，增量同步 `39.80025/44.458167/40.209084ms`。
- 真实迁移恢复测试直接执行 `applyMigration`，在 DDL 与 `schema_migrations` 写入之间阻塞并 SIGKILL；恢复后探针表和版本行同时不存在。
- 真实同步恢复测试用 writer 临时触发器阻塞 `CandidateService.Ingest` 的候选插入，确保此前事件、参与者、状态、日期与映射仍在同一未提交事务；恢复后全部回到基线。
- Online Backup 在快照完成后中断；新增启动清理移除 `.snapshot-*`/WAL/SHM、归档 partial 与恢复上传临时文件，清理失败时服务拒绝启动，避免反复中断耗尽数据卷。
- 普通写入、真实迁移、真实同步和备份四个中断点均连续三轮通过；重开后迁移、readiness 与 `PRAGMA integrity_check` 均正常。
- 独立复审最初发现 5 个 Important 与 2 个 Minor，全部按上述真实流水线、精确后置条件、启动清理、子进程 cleanup 和种子回滚测试关闭；复审确认无剩余 Critical/Important。
- 最终全仓 `go test ./... -race -count=1`、`go vet ./...`、前端 16 文件 33 测试/typecheck/API check/build、9 项 E2E 和高危依赖审计通过。
- 未创建 `.tmdb-token`，未访问真实 TMDB 或媒体服务器；测试凭据均为运行时合成值，未进入日志、数据库备份或构建产物。

### Task 25：生产 Docker 镜像与 Compose

- 已先建立会因镜像不存在而失败的容器烟测，并实现 Node/Go 多阶段构建、production 前端嵌入、静态 Go 二进制、distroless nonroot 运行时、二进制健康检查和单服务 Compose。
- 镜像与 Compose 固定 `65532:65532`、只暴露 8080、只允许 `/data` 可写，并启用只读根、drop ALL capabilities 与 no-new-privileges；`APP_ENCRYPTION_KEY` 必需，TMDB token 只保留空的服务端环境变量入口。
- 完整烟测真实执行初始化、登录、写入、删除并重建容器后的持久化、时区数据、备份下载、状态变更和恢复回滚；重建后的 `local/video-record:test` 镜像为 `sha256:9c97ac12b67f...`、`linux/arm64`、5,821,658 字节。
- 初始 Go 1.26.0 镜像被 Scout 检出 12 个 High；升级容器编译器到 1.26.4 后，最终镜像 17 个包、`0C/0H/0M/0L`。默认 Scout 解析因 Docker Desktop 凭据助手挂起，直接使用空 `DOCKER_CONFIG` 的插件和 `local://` 后完成扫描。
- 独立审查发现固定用户与 history 凭据检查有两个 Important 假阴性；新增策略红测后只接受 `65532:65532`，并拒绝通用 token/secret/password/credential/key、Bearer、JWT 和运行时密钥形态。
- 审查的 Minor 指出健康路径尾斜杠会命中 SPA；定向测试先得到 `200` 红灯，再让 `/healthz/`、`/readyz/` 委派 API，修复后返回真实后端结果。
- 最终全仓 `go test ./... -race -count=1`、`go vet ./...`、前端 16 文件 33 测试、typecheck/API check/build、9 项 E2E、npm 高危审计、Compose config、策略测试、容器烟测与 `git diff --check` 全部通过。
- 工作树、27 个提交、OCI 元数据、14 个镜像层/964 个文件和二进制 strings 的 gitleaks 扫描均无匹配；独立复审确认无剩余 Critical/Important。
- 当前本机只验收 arm64；amd64 与双平台 manifest 明确留给 Task 26 的 Buildx/QEMU CI，不创建或推送镜像标签。

### Task 26：CI、双架构发布与供应链元数据

- 已先添加 workflow policy、release metadata、manifest verifier、覆盖率门禁和镜像密钥扫描测试，并确认缺少 workflow/脚本、单平台 manifest、非法 SemVer、输出注入及不安全发布顺序均按预期失败，再实现最小发布链路。
- CI 固定第三方 action 到完整提交 SHA，执行 Go format/race/vet/迁移/精确覆盖率/govulncheck/gitleaks、前端 npm ci/lint/typecheck/33 项测试/OpenAPI/build/audit、Playwright 9 项 E2E、容器烟测、镜像层密钥扫描和高危/严重漏洞扫描。
- Go 1.26.4 被 `govulncheck` 检出标准库可达漏洞 `GO-2026-5856`；Docker 与 workflow 工具链升级到 Go 1.26.5 后复测为 0 个可达漏洞，arm64 镜像仍为 17 个包且 `0C/0H/0M/0L`。
- 精确加权语句覆盖率门禁结果为 records 85.0171%、media 86.5546%、stats 85.1485%、household 85.8447%、storage 85.3835%、integrations 91.7355%、sync 85.1093%。
- release workflow 在任何 push 前分别构建、烟测、漏洞扫描和镜像层密钥扫描 amd64/arm64；先推不可变完整版本并生成 SBOM/provenance、校验双平台 digest，再仅为稳定 SemVer 推送 major.minor 与 latest，预发布版本不生成稳定别名。
- manifest verifier 要求 amd64/arm64 各且仅各一个有效 SHA-256 descriptor 且平台 digest 不同；镜像密钥扫描器同时覆盖 OCI layout 与传统 Docker save archive。
- 最终 actionlint、workflow policy、release/manifest 脚本测试、shell 语法、覆盖率门禁、`git diff --check` 与工作树/28 个提交历史 gitleaks 均通过；独立复核同时通过 Go race/vet/govulncheck、前端全套验证和两种镜像归档扫描，确认无剩余 Critical/Important。
- 未创建或推送任何 Git 标签、Docker 镜像或发布产物；外部发布仍需显式授权。

### Task 27：运维文档与 v1 发布门禁

- 已创建 README、部署、集成、备份恢复、升级回滚、安全和发布清单，并用 `docs-acceptance-test.sh` 固定 fresh install、密钥生成、TMDB 归属、端口、备份、双架构和 secret scan 要求。
- 自动迁移在任何未应用 migration 前创建 Online Backup；备份失败会阻止迁移开始，恢复与升级文档明确保留原 `APP_ENCRYPTION_KEY` 和冷卷回滚流程。
- 已按设计 MUST 审计并补齐首页下一集/撤销、私有片单与排序、私人标签、家庭共享评分/短评及可见列表、观看事件二次确认删除、TMDB 无结果自定义创建和以后关联。
- 真实 API 无标签曾返回 `null` 导致详情页白屏；新增 HTTP 红测后在仓储源头返回空数组，最终 episode、household、recording 旅程全部恢复。
- axe 红灯定位 TMDB 未配置文字对比度 3.39:1；200% 缩放诊断定位媒体服务器表单固定最小列宽。分别修正为正文色+告警图标和可收缩网格后，axe/键盘/缩放/reduced-motion 全部通过。
- 视觉基线已从当前代码强制重录并人工检查 `375x812`、`768x1024`、`1440x900` 亮暗六张；个人片单区、海报比例、筛选和移动底栏无横向溢出。
- Go 1.26.5 全仓 race/vet/迁移/govulncheck 通过；精确覆盖率最终为 records 85.0622%、media 86.5546%、stats 85.1485%、household 86.6667%、storage 85.4195%、integrations 88.0000%、sync 85.1093%。
- 前端干净安装、lint/typecheck、23 文件 44 测试、OpenAPI check、build 和 npm audit 通过；Playwright 9/9、WCAG 阻断违规 0。
- 性能结果为 calendar p95 51.561292ms、library p95 10.752083ms、初次 10,000 条同步 4.702336959s、增量 46.776583ms；四种中断恢复全部通过。
- 最终 `rehearsal.vrbackup` 为 15,477 bytes，SHA-256 `f3f29f985385e24364bdf9a2bf2d4da6acbccc9ffe8944b589b0134b5f3d59fb`；归档完整、只含 manifest/数据库，真实恢复精确移除备份后的成员。
- Compose 使用独立项目/卷、随机 32-byte key 和 `127.0.0.1:18080` 完成 health/ready、前端访问与封闭初始化；临时资源已清理。
- 最终 amd64 镜像为 `sha256:0602f09c91dd...`、6,197,906 bytes，arm64 为 `sha256:dcdd4e1d49c1...`、5,845,732 bytes；两者分别完成完整 smoke、14 层/964 文件 secret scan 和 Scout `0C/0H/0M/0L`。
- actionlint、workflow policy、coverage/release/manifest/image 脚本、shell syntax、文档验收、工作树与 29 个提交历史 gitleaks 全部通过。
- 未使用个人 TMDB token；未创建/推送 Git 标签、main、镜像或 manifest，未生成外部 SBOM/provenance，外部发布仍等待明确授权。

## 2026-07-14：剧集观看时间改造

- 已读取适用的 superpowers、TDD、文件化计划与产品 UI 工作流。
- 已确认在 `main` 上工作，并把本轮六阶段加入 `task_plan.md`。
- 已确认仓库已有剧集进度/观看事件/详情页测试基础；当前处于需求与现状探索阶段，尚未修改业务代码。
- 已确认现有 API 已携带单集 `watchedAt`，但前端未展示，后端也没有修改已有单集观看时间的路径。
- 已读取现有详情改造设计、产品注册规范、`EpisodeProgress`、`WatchHistory`、样式令牌和剧集进度事务实现；下一步等待确认底部历史的信息架构。
- 用户确认电影与剧集都移除观看历史，并把需求扩展为可归档上轮记录的“多刷”以及剧集按季隔离的个人记录。
- 已验证当前个人记录是整部影视作用域，现有重看接口只新增观看事件；本轮计划已升级为观看轮次与季级个人记录改造。
- 已确认再刷后的精确归档/重置边界；当前仍在设计阶段，尚未修改业务代码。
- 用户授权清理全部旧观看与个人记录数据，不做旧版整剧记录的迁移兼容；开始比较新模型方案。
- 已比较显式轮次、JSON 快照和事件推导三种方案；用户确认显式观看轮次架构，正在确认页面交互与错误边界。
- 页面交互设计已确认；正在提交最后的 API、事务、状态投影、错误处理和测试设计供整体确认。
- 整套设计已确认；已写入 `docs/plans/2026-07-14-viewing-rounds-season-records-design.md`，等待校验和提交。
- 设计文档已提交到 `main`，提交为 `6e5b59d`；独立新文件检查发现两处 Markdown 行尾空格，已修正并记录流程错误。
- 已按 `writing-plans` 审计轮次改造的服务端、迁移、API、前端和测试消费者，正在编写逐任务实施计划。
- 已补齐影视级 profile、家庭分享、同步/导入和 E2E 种子依赖，计划将按迁移、领域、API、消费者、前端和浏览器门禁分层执行。
- 已生成 `docs/plans/2026-07-14-viewing-rounds-season-records-implementation.md`：13 个任务，全部包含 RED/GREEN、精确文件、验证命令和提交边界。
- 开始执行 Task 1；已核对嵌入式迁移的事务边界和旧版本测试 helper，准备先加入缺少 0013 时必然失败的升级测试。
- Task 1 首次测试补丁因工具脚本反引号解析失败，未产生文件修改；已改用安全占位符方案。
- Task 1 定向 RED 符合预期；GREEN 后完整存储包暴露旧 0012 测试的相对版本漂移，已用稳定版本 11 夹具修正根因。
- Task 1 完成：观看轮次迁移测试、0012 回填回归和完整 `internal/storage` 包通过；提交为 `6ee66eb`。
- Task 2 完成：轮次空读/首次写入、电影/季作用域、用户隔离、未来时间和归档只读定向测试通过；提交为 `fbc1141`。
- Task 3 完成：profile 投影、影库/搜索和独立标签版本定向测试通过；提交为 `bb0567e`。
- Task 4 完成：当前轮次 HTTP、鉴权/并发边界、OpenAPI 与生成类型检查通过；提交为 `8971122`。
- Task 5 完成：分集进度已按所选季和当前轮次隔离；单集标记、`set_time`、撤销、未来时间回滚、季完成投影和跨季隔离领域测试通过。
- Task 5 HTTP GET/POST 强制正整数 `seasonNumber`，响应返回 `roundId`/季号，OpenAPI 生成与漂移检查通过；旧媒体级进度测试已替换为季级契约。
- Task 5 定向验证通过：records 7 个季级/稀疏进度测试、HTTP 2 个进度处理器测试、HTTP OpenAPI/contract 测试和 `npm --prefix web run api:check`。
- Task 6 完成：电影与季再刷会在单事务内归档已完成轮次并创建状态为 `watching` 的空白下一轮；注入下一轮插入失败时归档完整回滚。
- Task 6 归档列表/详情严格按用户、媒体和季作用域读取；剧集归档详情保留逐集秒级时间，其他季当前轮次不变，其他用户无法读取私人历史。
- Task 6 新增历史、详情和再刷 HTTP/OpenAPI 合约；再刷受 If-Match、同源、CSRF 和幂等保护，幂等路径已包含查询参数以隔离不同季。
- Task 7 完成：电影轮次完成会事务内创建/同步所属事件，归档后事件继续供日历、统计和共同观看读取；撤回完成状态会移除当前轮事件，避免重复事实。
- Task 7 日历过滤改为事件所属轮次状态，统计评分改读所有当前/归档轮次；家庭评分/私人笔记只取最近当前轮次，分享开关和公开短评保留在影视 Profile。
- Task 7 已从路由和 OpenAPI 删除详情页扁平观看事件 GET/POST/DELETE；日历、影库、统计、家庭和契约定向测试及 API 漂移检查通过。
- Task 8 完成：JSON 导入导出升级为 version 2 的 `profile + rounds[]`，轮次内嵌事件和分集时间；version 1 明确拒绝，不保留调试旧数据兼容。
- Task 8 CSV 按轮次逐行输出 `round_id/round_number/season_number` 及归档时间，并继续防护表格公式；JSON 往返已验证归档私人记录、逐集时间、标签、片单和分享设置。
- Task 8 同步候选改用 records 轮次事务 helper：电影重复完成自动分轮，单集事件写入所属季当前轮次，外部事件 ID 幂等和候选/映射事务边界保留；完整 sync 包通过。
- Task 9 完成：前端新增 Current/Archived Round、历史摘要/详情、再刷结果和季级 SeriesProgress 类型，以及当前轮次读写、历史、详情、再刷客户端。
- Task 9 分集客户端的新签名强制携带季号并支持 `set_time`；旧组件的临时重载保留到 Task 10-12 删除，最终页面不会调用无季号路径。
- Task 9 新增本地秒级时间工具，使用 `Intl.DateTimeFormat` 显示并通过本地日期分量转换 `datetime-local`；8 个定向测试、typecheck 和 lint 通过。
- Task 10 完成：新增电影/季级 `RoundRecordForm`，电影完成使用必填的本地秒级 `datetime-local`，剧集季状态由分集进度只读投影；评分、笔记、观看方式和电影参与者写入当前轮次。
- Task 10 保留冲突 ETag 重试、网络失败草稿保留和保存反馈；外部切换到下一轮时按媒体/季/轮次号重置表单，组件内部不再包含任何“再刷/再看一次”动作。
- Task 10 的 7 个组件测试与 3 个本地时间测试、前端 typecheck、lint 和 `git diff --check` 通过；JSDOM 会把秒级原生值规范化为 `.000`，转换器现只额外接受零毫秒形式，仍拒绝非秒级值。
- `QuickRecordForm` 的删除延后到季工作区和详情页接入新轮次表单时执行，避免 Task 10 单独提交期间让剧集详情绑定到错误的固定季。
- Task 11 完成：新增顶层 `SeasonRecordWorkspace`，先读取各季当前轮次再选择正在观看季；季选择会同步切换季级分集进度和“第 N 季个人记录”，各季使用独立缓存键。
- Task 11 重写分集列表：每行拆分为圆圈、集号、累计集数、标题和时间按钮；已看集显示本地 `YYYY-MM-DD HH:mm:ss`，未看集显示“设置观看时间”，移动端时间稳定落在第二行。
- Task 11 新增 `EpisodeTimeEditor`：原生秒级 `datetime-local`、动态 max、前端未来时间拦截、未看集 `set_time`、已看集时间修改、取消恢复焦点和目标行独立 pending 状态均有组件覆盖。
- Task 11 首页会先读取所有常规季当前轮次，选择最高的 `watching` 季后才请求该季进度；不会在轮次查询尚未稳定时误读默认季，TMDB 失败也不会被禁用 query 的 pending 状态卡住。
- Task 11 定向验证通过：5 个文件 14 项测试、前端 typecheck、lint 和 `git diff --check`；详情页已开始接入季工作区，电影旧历史将在 Task 12 统一替换。
- Task 12 完成：电影与剧集详情均移除“观看历史”，新增统一“多刷”区域；当前轮次未完成时“再刷”禁用，完成后由单次原子 API 归档旧轮并创建状态为 `watching` 的下一轮。
- Task 12 再刷成功后只更新对应电影/季的当前轮次、归档摘要和当前季进度缓存；失败时保留表单、分集和历史，页面显示内联错误。
- Task 12 多刷列表显示第 N 刷、秒级完成时间和评分摘要；点击“查看”才请求只读轮次详情，弹窗展示完成时间、评分、观看方式和私人笔记，剧集额外展示逐集秒级时间。
- Task 12 电影详情改用 `RoundRecordForm` 和 movie current round；标签、片单和分享继续使用 `profileVersion`。剧集 DOM 顺序为季选择、分集、季级个人记录、多刷。
- 已删除 `QuickRecordForm`、`WatchHistory` 及其测试，移除前端事件 GET/POST/DELETE 客户端、媒体级 update 客户端和无季号 progress 重载；源码中相关旧符号无残留。
- Task 12 完整前端验证通过：28 个测试文件 72 项测试、typecheck、lint 和 `git diff --check`。

# video-record 产品设计与竞品调研计划

## 目标

为电影与剧集观影记录项目形成一套中文、可执行、可验收的产品与 UI/UX 设计文档，并明确本机开发、单一 `main` 分支和 Docker Hub 多架构发布约束。

## 阶段

- [x] 阶段 1：核对空仓库、Git 状态与 `invoice-manage` 工程参照
- [x] 阶段 2：调研 MoviePilot v2、Jellyseerr 的公开 UI、导航和信息架构
- [x] 阶段 3：调研已阅、Apollo、Filmo 的记录逻辑、核心功能和交互模式
- [x] 阶段 4：确认目标用户、数据来源与首版边界
- [x] 阶段 5：提出 2-3 套产品方向并推荐其一
- [x] 阶段 6：分章节确认产品、交互、视觉、技术与发布设计（六章全部确认）
- [x] 阶段 7：写入 `docs/plans/2026-07-13-video-record-design.md` 并提交（提交 `d0ee509`）
- [x] 阶段 8：在设计获批后编写实施计划
- [ ] 阶段 9：按实施计划完成 27 个任务并逐任务验证、记录和提交

## 实施进度

- [x] Task 1：仓库工具链与最小服务器
- [x] Task 2：配置、日志与请求 ID
- [x] Task 3：SQLite 连接与嵌入式迁移
- [x] Task 4：用户、密码、会话与封闭初始化
- [x] Task 5：前端基础与设计令牌
- [x] Task 6：TMDB 客户端、归属说明与安全缓存
- [x] Task 7：本地影视目录与外部身份
- [x] Task 8：个人状态、评分、标签与片单
- [x] Task 9：不可变观看事件与幂等
- [x] Task 10：搜索、详情、影库与快速记录界面
- [x] Task 11：剧集进度与系列状态投影
- [x] Task 12：日历与时间线查询
- [x] Task 13：可访问统计
- [x] Task 14：家庭成员与隐私边界
- [x] Task 15：JSON/CSV 安全导入导出
- [x] Task 16：一致性备份与原子恢复
- [x] Task 17：Provider 契约、加密账户与持久化调度器
- [x] Task 18：Jellyfin 播放历史 Provider
- [x] Task 19：Emby 播放历史 Provider
- [x] Task 20：Plex 播放历史 Provider
- [x] Task 21：候选匹配、冲突解决与同步界面
- [x] Task 22：OpenAPI 合约与完整 HTTP 安全测试
- [x] Task 23：E2E、无障碍与视觉回归
- [x] Task 24：性能与故障恢复基线
- [x] Task 25：生产 Docker 镜像与 Compose
- [ ] Task 26：CI、双架构发布与供应链元数据
- [ ] Task 27：运维文档与 v1 发布门禁

## 已确认约束

- 仓库路径：`/Users/subeipo/Documents/code/video-record`
- 仅在 `main` 分支开发
- 本机调试开发
- 最终构建 Docker 镜像并发布 `linux/arm64` 与 `linux/amd64`
- `/Users/subeipo/Documents/code/invoice-manage` 只作为 Docker 项目与多架构交付方式的参照，不复用其技术栈结论
- 技术栈必须依据本项目的领域模型、同步需求、运行资源、可维护性和镜像交付重新选型
- 产品以家庭自托管、多用户独立记录与共享为架构目标，同时让单人使用保持默认且轻量
- 手动搜索与记录是完整主流程，可选同步 Jellyfin、Emby、Plex 的播放历史
- 接入 TMDB 提供电影、剧集搜索和基础元数据；本地个人记录不得依赖 TMDB 实时可用
- 产品聚焦观影记录；明确不做求片、下载、订阅、媒体文件管理或公开社区
- 视觉方向为克制、私人、具有电影质感的数字影库；默认跟随系统明暗主题
- 无障碍基线为 WCAG 2.2 AA，支持键盘、可见焦点、减弱动态效果和非颜色状态表达
- 技术方案采用 Go 模块化单体 + React/Vite + SQLite，单应用容器、单实例运行
- TMDB 读访问令牌只通过服务端环境变量注入，任何仓库文件、前端构建、日志、导出和备份均不得包含令牌值
- 当前阶段按已确认实施计划持续完成业务代码、验证、进度记录与逐任务提交

## 错误记录

| 错误 | 尝试 | 处理 |
|---|---:|---|
| Seerr 主界面静态图的浏览器整页截图超时 | 1 | 不重复整页截图；改用官方页面 DOM、静态资源或局部图核对 |
| Filmo 美国区商店详情被当前地区重定向至中国区首页 | 1 | 使用中国区精确应用 ID 搜索结果与多来源商店摘要交叉核对，不臆测不可见界面 |
| Task 1 健康测试被缺失的 Testify 及传递依赖 `go.sum` 条目阻断 | 2 | 顶层模块下载不足，改用 `go get -t video-record/internal/httpapi` 解析完整测试依赖图，再重跑以取得有效红灯 |
| Task 5 首次 npm 安装发生 Vite peer dependency 冲突 | 1 | `@vitejs/plugin-react 6.0.3` 仅支持 Vite 8；固定为明确支持 Vite 7 的 `5.2.0` 后重跑原安装命令 |
| Task 5 首次 typecheck/build 缺少 CSS 与 Vitest 配置类型 | 1 | `types` 显式列表排除了 `vite/client`，且 `defineConfig` 来源错误；分别补入 Vite 客户端类型并改用 `vitest/config` 后逐项复验 |
| Task 6 格式化命令误把 SQL 传给 `gofmt` | 1 | 实现测试已通过；改为只格式化 `internal/integrations/tmdb/*.go`，SQL 由迁移测试和补丁检查验证 |
| Task 6 首次 `go run` gitleaks 使用了仓库路径而非声明模块路径 | 1 | v8.30.1 的 module 仍声明为 `github.com/zricethezav/gitleaks/v8`；核对该路径版本后用声明路径重跑扫描 |
| Task 8 片单跨用户增项被错误当作幂等成功 | 1 | 重复检查看到了所有者已有条目；改为事务入口先按 `collection_id + user_id` 验证所有权，再执行位置与插入逻辑 |
| Task 8 初次领域覆盖率仅 75.4% | 1 | 补齐显式 null、字段省略、无操作版本、输入校验和私有片单回归测试后达到 85.5% |
| Task 9 过期幂等夹具把 TEXT 写入 STRICT BLOB 列 | 1 | 根因是 SQL 文本字面量类型错误；改为绑定 `[]byte` 后重跑同一过期清理测试 |
| Task 10 快速记录按钮残留多余 JSX 终止符 | 1 | 骨架 map 结束符在改写 ternary 后残留；删除单个多余 `)}` 并重跑 typecheck/定向测试 |
| Task 10 搜索 signal 与评分标签未满足 strict/a11y 测试 | 1 | 仅在 signal 存在时构造 RequestInit，并为带 `/10` 后缀的评分输入添加显式可访问名称 |
| Task 10 Vite 代理转发的 Origin 与目标 Host 不一致 | 1 | 开发代理将 Host/Origin 同时规范到 API target；生产同源请求路径不受影响 |
| Task 10 新增 catalog 后领域覆盖率降至 77.9% | 1 | 增加真实 SQLite 的用户隔离、状态筛选、搜索状态投影和 LIKE 转义测试后达到 86.2% |
| Task 10 顶栏搜索只依赖 focus 时浏览器点击未稳定打开 | 1 | 先加入失败点击回归测试，再补显式 `onClick`，与键盘 focus 路径并存 |
| Task 10 并行 Go race 只返回 records 包状态 | 1 | 未据此宣称通过；改为单独运行并轮询到全部包 exit 0 |
| Task 11 HTTP 红灯测试 helper 与既有 `currentUserID` 重名 | 1 | 读取既有 helper 后删除重复定义，复用真实 `/auth/me` 取当前用户，再取得端点 `404` 红灯 |
| Task 11 组件测试用纯文本定位命中单集行和两个下拉选项 | 1 | 改用单集按钮的可访问名称限定目标，并在按钮内验证季集编号与累计集数 |
| Task 11 新增领域逻辑后覆盖率先降至 82.4%，删除投影后为 84.8% | 2 | 补齐选择器、无操作、事务内版本/归属、防止覆盖高优先级状态及删除事件投影测试，最终达到 85.1% |
| Task 11 浏览器验证端口 `18080` 已占用 | 1 | 未复用未知进程，改用独立 `28080/25173` 启动合成数据验证环境 |
| Task 12 首次组件运行因测试缺少 Router、ES target 不含 `toSorted` 且严格索引报错 | 1 | 测试包裹 `MemoryRouter`，使用复制后 `sort`，并显式处理数组/月份拆分空值，再重跑原命令 |
| Task 12 新增日历查询后领域覆盖率降至 84.2% | 1 | 增加真实剧集事件的季集/累计集数/观看方式投影测试，最终达到 85.8% |
| Task 13 检查关键领域覆盖率时发现 `internal/media` 仅 68.1% | 1 | 补齐 runtime/genre 原子刷新、校验、链接冲突、事务回滚与存储故障测试，提升至 86.6% |
| Task 14 参与者测试整结构比较因 SQLite 毫秒时间精度失败 | 1 | 改为断言参与者身份、角色与活跃状态，不把无关纳秒精度作为业务契约 |
| Task 14 浏览器种子脚本误从初始化响应读取 CSRF，并把自定义影视年份传为数字 | 2 | 初始化后改用登录响应取得 CSRF；按 HTTP DTO 将 `year` 传为字符串后完成合成数据验收 |
| Task 14 全前端测试出现未处理的 `/auth/me` 请求 | 1 | 设置页壳测试补普通成员 MSW 响应并等待查询完成，避免异步请求越过测试生命周期 |
| Task 15 重复身份夹具缺少必需的空 `collections` 数组 | 1 | 补齐版本化 JSON 文档结构后再验证逐条部分失败，不放宽格式校验 |
| Task 15 导入校验错误要求自定义影视必须有外部标题 | 1 | 按现有媒体模型改为外部标题或自定义标题至少一个非空 |
| Task 15 组件测试在 MSW 中解析 multipart 后一直 pending | 1 | HTTP 集成测试保留真实文件名解析；组件测试改验 multipart Content-Type 与文件输入值 |
| Task 15 安全复核发现导入可覆盖共享媒体并接管他人片单 ID | 1 | 先写越权红测；既有媒体只附着个人数据，片单 ID 已由其他用户拥有时拒绝该片单事务 |
| Task 16 初版恢复用两次 rename，崩溃窗口会让规范数据库路径短暂消失 | 1 | 增加失败测试后改为旧库硬链接 + 单次原子覆盖；新库重开和提交回调成功前保留旧链接，失败使用独立恢复上下文回滚 |
| Task 16 通用幂等中间件把大于 1 MiB 的恢复上传提前拒绝 | 1 | 以归档内容哈希执行专用幂等路径，multipart 流式写入私有暂存文件，不缓存完整请求体 |
| Task 16 全仓 race 发现关闭数据库后仓储拿到 nil 连接并 panic | 1 | 普通 `DB.Close` 保留已关闭句柄契约；恢复内部关闭仍清空连接以支持原位重开 |
| Task 16 流式与空间校验新增分支使存储覆盖率低于 85% | 4 | 补齐暂存、畸形 ZIP、跨文件系统空间探测、超时恢复、N-1 和迁移篡改测试，精确覆盖率达到 85.1351% |
| Task 17 游标结果参数与 SQL Result 局部变量同名导致编译失败 | 1 | 保留领域参数名，将数据库执行结果改为 `execResult` 后重跑定向测试 |
| Task 17 篡改密文测试用 BLOB 拼接产生 STRICT TEXT 类型错误 | 1 | 读取真实字节后在 Go 中篡改，并以 `[]byte` 参数写回 BLOB 列 |
| Task 17 正式迁移创建 `external_accounts` 后旧备份测试重复建表 | 1 | 删除占位表夹具，改为向正式 STRICT 表插入满足外键的合成用户和账户 |
| Task 18 新增 HTTP 测试 handler 混用命名与匿名参数导致编译失败 | 1 | 将未使用请求参数显式命名为 `_` 后重跑同一红测 |
| Task 18 初版按插件注释使用 `movies,series` 且假定返回完整时间 | 2 | 核对插件实现后改用真实 `Movie,Episode`，并将日期路径与 `h:mm tt` 组合为 UTC 时间 |
| Task 18 全仓 race 发现取消期间 SQLite 错误覆盖正常关停结果 | 1 | `RunDue` 返回后优先检查父 context，定向 race 连续 20 次通过后重跑全仓 |
| Task 21 前端首次 typecheck 缺少 `MediaType` 导入且显式传递 optional `undefined` | 1 | 补齐类型导入，并只在单集 ID 存在时构造该可选属性后复验 |
| Task 21 新增生产 runner 后同步包覆盖率降至 72.3% | 4 | 补齐候选生命周期、目标归属、重复外部事件、Provider 失败与严格凭据分支，最终达到 85.1% |
| Task 21 浏览器发现同名同年候选的 radio 可访问名称重复 | 1 | 先加失败组件/领域测试，再把候选序号和规范化原名加入受控 DTO 与标签 |
| Task 22 通用幂等中间件读取 1 MiB 请求体后会提前拒绝合法的 10 MiB 导入 | 1 | 导入端点改为在 handler 内流式哈希文件名与内容，保留 10 MiB 导入上限且不把完整文件缓存在内存 |
| Task 22 并发相同幂等键可在副作用前同时通过缓存查询 | 1 | 新增 SQLite 唯一约束驱动的原子 reservation 和 `pending` 状态，竞争请求返回稳定 `idempotency_in_progress` |
| Task 22 恢复替换数据库后旧连接无法写入幂等完成结果 | 1 | 恢复端点在新数据库重开后通过专用 finalize 路径提交响应缓存，并保留原 request ID |
| Task 22 标签写入未参与 ETag 乐观并发控制 | 1 | 先加陈旧写入红测，再在事务中校验 `If-Match`、递增记录版本并返回新 ETag |
| Task 22 全仓 race 下调度器故障恢复测试偶发超过 1 秒等待窗 | 1 | 定向 race 连续 20 次确认无数据竞争，将测试等待窗调整为 3 秒后重跑全仓门禁 |
| Task 23 首次 E2E 按预期找不到封闭初始化与登录界面 | 1 | 先保留页面缺失红测，再实现 AuthGate、存储/TMDB 布尔状态、管理员初始化和会话登录 |
| Task 23 浅色主色链接在次级表面仅有 4.27:1 对比度 | 1 | axe 红测定位后降低主色明度，完整 WCAG 2.0/2.1/2.2 A/AA 扫描归零 |
| Task 23 合成导入夹具使用 version 0 导致逐条失败但 HTTP 仍为 200 | 1 | 改为合法 version 1，并强制断言导入 2 条且 failures 为空，避免静默空影库 |
| Task 23 重复提交“看过”不会新增不可变重复观看事件 | 1 | 增加组件/E2E 红测和显式“再看一次”动作，复用受 CSRF 与幂等保护的事件端点 |
| Task 23 视觉快照 1% 容差接受了旧的空影库基线 | 1 | 增加“2 部影视”结构断言，强制重录真实海报网格，并使用 OS 无关快照路径 |
| Task 23 E2E 紧邻重跑时 Playwright 子服务尚未完全释放端口 | 1 | runner 等待前后端端口拒绝连接后再清理数据并退出，独立复审连续两次 9/9 |
| Task 23 工作树 gitleaks 把长合成 password 字面量判为 generic-api-key | 1 | 不加 allowlist，改为运行时拼接短公开测试片段，工作树与历史扫描均为零匹配 |
| Task 23 in-app browser 标签列表与当前任务会话错配 | 2 | 按 browser 技能读取诊断并停止跨后端重试；保留真实 Chromium E2E、axe 与像素快照证据 |
| Task 24 性能与恢复脚本首次运行缺少 `internal/testutil` 非测试实现 | 1 | RED 阶段预期失败；保留证据并按真实迁移约束实现最小 `Seed` API |
| Task 24 首次完整性能基线在日历请求返回 `500 internal_error` | 1 | 不重复盲跑；沿 HTTP、records 与 SQLite 数据格式边界定位根因后补回归测试 |
| Task 24 独立审查发现恢复/同步门禁可被空结果或普通事务回滚绕过 | 1 | 暂停提交；用真实迁移原子边界、候选多表副作用、SIGKILL 临时快照清理、精确计数和同步期间 HTTP 采样加固 |
| Task 25 首次新增容器冒烟脚本的补丁格式无效 | 1 | `apply_patch` 在写入前整体拒绝；补齐新增文件行前缀后重新应用，未产生部分文件 |
| Task 25 Docker registry 元数据请求持续挂起 | 2 | Docker Hub 的 Go/Node 与 `mirror.gcr.io` 同时卡在 HEAD；不改版本，先验证本地 production 嵌入构建并等待 registry 通路恢复 |
| Task 25 首轮容器冒烟无法解析初始化 CSRF token | 1 | 容器和请求已成功；核对认证响应 DTO 与 Node stdin/argv 解析边界后修正脚本 |
| Task 25 初始镜像的 Go 1.26.0 标准库存在 12 个 High CVE | 1 | Scout 显示修复上限为 1.26.4；仅升级 Docker 编译器补丁版本并重建复扫，保持 Go 1.26 语言/模块契约 |
| Task 25 独立审查发现固定用户与 image history 检查可假通过 | 1 | 先新增失败策略测试，再严格要求 `65532:65532` 并覆盖通用 token/secret/password/credential/key、Bearer、JWT 与运行时密钥形态 |
| Task 25 Docker Scout 默认镜像解析无输出挂起 | 2 | 默认插件继承 `credsStore: desktop`；改为直接调用 Scout 插件、使用空 `DOCKER_CONFIG` 与 `local://`，本地镜像扫描在约 2 秒内完成 |

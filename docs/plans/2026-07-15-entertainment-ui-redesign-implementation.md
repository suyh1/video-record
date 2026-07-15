# Entertainment UI Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 把 video-record 改造成由 TMDB 真实影视图片驱动的娱乐化观影档案，加入登录与首页轮播、后端图片代理、顶部导航和全站统一视觉，同时保留现有记录功能、响应式与无障碍基线。

**Architecture:** Go 服务端新增缓存的 TMDB 热门聚合与带签名的同源图片代理，并把所有 TMDB 图片字段规范化为本站代理 URL。React 端建立可复用的 backdrop 轮播、媒体动态取色和统一控件体系，再按登录、应用框架、首页、影库、详情、辅助页面的顺序逐批迁移；每个区域独立加载和降级。

**Tech Stack:** Go 1.26、chi、SQLite TMDB cache、React 19、TanStack Query、React Router 7、TypeScript、原生 CSS token、Vitest、Testing Library、MSW、Playwright、axe、`@fontsource-variable/outfit@5.2.8`。

---

## 执行规则

- 用户明确要求在 `main` 上修改，不创建分支或 worktree；每个任务开始前确认 `git status --short --branch`。
- 每个行为变更使用 `@superpowers:test-driven-development`：先写失败测试，确认失败原因正确，再写最小实现。
- 遇到非预期失败使用 `@superpowers:systematic-debugging`，不通过扩大超时或放宽断言掩盖问题。
- 视觉验收使用 `@browser:control-in-app-browser`；完成声明前使用 `@superpowers:verification-before-completion`。
- 不改变观影记录、家庭共享、备份恢复或同步数据模型。
- 不添加本地精选影视图片、外部字体 CDN、GSAP、滚动劫持、3D 或推荐算法。
- 不直接访问 `image.tmdb.org`；最终浏览器网络检查必须证明所有 TMDB 图片均走本站代理。

## Task 1: TMDB 热门电影与剧集客户端

**Files:**
- Modify: `internal/integrations/tmdb/types.go`
- Modify: `internal/integrations/tmdb/client.go`
- Modify: `internal/integrations/tmdb/client_test.go`

**Step 1: 写热门列表失败测试**

在 `client_test.go` 增加 `TestClientPopularMoviesAndTVUseSixHourCache`。测试服务器分别响应 `/movie/popular` 和 `/tv/popular`，断言：

```go
movies, err := client.Popular(context.Background(), "movie", "zh-CN")
require.NoError(t, err)
require.Equal(t, "降临", movies.Results[0].Title)

shows, err := client.Popular(context.Background(), "tv", "zh-CN")
require.NoError(t, err)
require.Equal(t, "权力的游戏", shows.Results[0].Name)
require.Equal(t, int32(2), requests.Load())
```

再次调用两个类型时请求数仍为 2；推进 fake clock 超过 6 小时后请求数变为 4。另加子测试断言 `person` 返回稳定错误且不访问上游。

**Step 2: 运行测试并确认失败**

Run: `go test ./internal/integrations/tmdb -run TestClientPopularMoviesAndTVUseSixHourCache -count=1`

Expected: FAIL，提示 `client.Popular undefined`。

**Step 3: 增加 TMDB 热门类型**

在 `types.go` 增加：

```go
type PopularResponse struct {
	Page         int           `json:"page"`
	Results      []PopularItem `json:"results"`
	TotalPages   int           `json:"total_pages"`
	TotalResults int           `json:"total_results"`
}

type PopularItem struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	OriginalTitle string `json:"original_title"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	BackdropPath  string `json:"backdrop_path"`
	Overview      string `json:"overview"`
}
```

**Step 4: 实现带缓存的 Popular**

在 `client.go` 增加 `popularCacheTTL = 6 * time.Hour` 和：

```go
func (client *Client) Popular(ctx context.Context, mediaType, language string) (PopularResponse, error) {
	if mediaType != "movie" && mediaType != "tv" {
		return PopularResponse{}, &ClientError{Kind: ErrUpstreamUnavailable}
	}
	var response PopularResponse
	err := client.get(ctx, "/"+mediaType+"/popular", languageQuery(language), popularCacheTTL, &response)
	return response, err
}
```

**Step 5: 运行 TMDB 客户端测试**

Run: `go test ./internal/integrations/tmdb -count=1`

Expected: PASS。

**Step 6: 提交**

```bash
git add internal/integrations/tmdb/types.go internal/integrations/tmdb/client.go internal/integrations/tmdb/client_test.go
git commit -m "feat: add cached TMDB popular feeds"
```

## Task 2: 签名图片 URL 与受限图片获取

**Files:**
- Create: `internal/integrations/tmdb/images.go`
- Create: `internal/integrations/tmdb/images_test.go`
- Modify: `internal/integrations/tmdb/client.go`

**Step 1: 写签名验证失败测试**

新增表驱动测试覆盖 `w300`、`w342`、`w780`、`w1280`，并断言非法尺寸、篡改路径和过期签名被拒绝：

```go
expires := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
signature, err := client.SignImage("w1280", "/arrival.jpg", expires)
require.NoError(t, err)
require.True(t, client.VerifyImage("w1280", "/arrival.jpg", expires, signature))
require.False(t, client.VerifyImage("w1280", "/changed.jpg", expires, signature))
require.False(t, client.VerifyImage("original", "/arrival.jpg", expires, signature))
```

再覆盖空 token、非法扩展名、嵌套路径、路径穿越和超过有效期上限。

**Step 2: 写图片上游边界失败测试**

使用 `httptest.Server` 作为 `ImageBaseURL`，验证：

- 请求路径固定为 `/t/p/w1280/arrival.jpg`。
- 图片请求不携带 TMDB Bearer token。
- 只接受 `image/jpeg`、`image/png`、`image/webp`。
- 超大响应、HTML、404、超时映射为稳定错误。
- 请求 Context 取消能停止上游读取。

**Step 3: 运行测试并确认失败**

Run: `go test ./internal/integrations/tmdb -run 'TestImage|TestSignedImage' -count=1`

Expected: FAIL，提示签名和图片方法未定义。

**Step 4: 实现图片边界**

在 `ClientOptions` 与 `Client` 增加 `ImageBaseURL`，默认值为 `https://image.tmdb.org/t/p`。在 `images.go` 定义：

```go
type ImageAsset struct {
	ContentType string
	Contents    []byte
}

var allowedImageSizes = map[string]struct{}{
	"w300": {}, "w342": {}, "w780": {}, "w1280": {},
}

func (client *Client) SignImage(size, path string, expires time.Time) (string, error)
func (client *Client) VerifyImage(size, path string, expires time.Time, signature string) bool
func (client *Client) Image(ctx context.Context, size, path string) (ImageAsset, error)
```

使用 HMAC-SHA256，签名内容固定为 `size + "\n" + path + "\n" + expires.Unix()`，密钥由 TMDB token 派生。使用 `hmac.Equal` 比较；路径只允许单段 `/[A-Za-z0-9_-]+.(jpg|jpeg|png|webp)`。图片最大 12 MiB，沿用客户端 8 秒默认超时，但不设置 Authorization 请求头。

**Step 5: 运行测试**

Run: `go test ./internal/integrations/tmdb -count=1`

Expected: PASS。

**Step 6: 提交**

```bash
git add internal/integrations/tmdb/images.go internal/integrations/tmdb/images_test.go internal/integrations/tmdb/client.go
git commit -m "feat: add signed TMDB image proxy client"
```

## Task 3: 登录前热门列表与图片 HTTP 接口

**Files:**
- Create: `internal/httpapi/public_tmdb_handlers.go`
- Create: `internal/httpapi/public_tmdb_handlers_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/tmdb_handlers.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/generated.ts`

**Step 1: 写公开鉴权边界失败测试**

增加无 Cookie 请求测试：

```go
response := performJSONRequest(router, http.MethodGet,
	"http://example.test/api/v1/public/tmdb/highlights", nil, nil)
require.Equal(t, http.StatusOK, response.Code)
require.Contains(t, response.Body.String(), `"mediaType":"movie"`)
require.Contains(t, response.Body.String(), `"mediaType":"tv"`)
require.NotContains(t, response.Body.String(), "synthetic-token")
```

同一测试确认 `/api/v1/tmdb/search?q=test` 无 Cookie 仍为 401。热门上游夹带无 backdrop 项，响应必须过滤并按电影、剧集交替返回最多 10 项。

**Step 2: 写图片路由失败测试**

从热门响应解析 `backdropURL`，无 Cookie 请求该 URL，断言图片字节、Content-Type 和：

```go
require.Equal(t, "public, max-age=86400, immutable", response.Header().Get("Cache-Control"))
```

篡改 size、path、expires、signature 分别得到 400、403 或 410 的稳定问题码；上游错误使用现有 `writeTMDBError` 映射。

**Step 3: 运行测试并确认失败**

Run: `go test ./internal/httpapi -run 'TestPublicTMDB' -count=1`

Expected: FAIL，公开路由返回 404。

**Step 4: 实现热门聚合 handler**

创建 `publicTMDBHandlers`，并行调用 `Popular(movie)` 与 `Popular(tv)`。用以下响应类型输出驼峰字段：

```go
type publicTMDBHighlight struct {
	ID            int    `json:"id"`
	MediaType     string `json:"mediaType"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle"`
	Year          string `json:"year"`
	Overview      string `json:"overview"`
	BackdropURL   string `json:"backdropURL"`
}
```

签名有效期 24 小时。相对 URL 使用 `/api/v1/public/tmdb/images/{size}/{filename}?expires=...&signature=...`，不要根据请求 Host 拼绝对 URL。

**Step 5: 在鉴权组外注册只读路由**

在 `router.go` 的 `/api/v1` 路由中、`protected.Group` 之前注册：

```go
publicTMDB := publicTMDBHandlers{client: dependencies.TMDB, now: time.Now}
api.Get("/public/tmdb/highlights", publicTMDB.highlights)
api.Get("/public/tmdb/images/{size}/{filename}", publicTMDB.image)
```

公开接口不应用 CSRF、会话或幂等中间件；仍然经过请求 ID、日志、恢复和维护模式。

**Step 6: 更新 OpenAPI 并生成客户端**

在 `api/openapi.yaml` 增加两个公开 GET path、`PublicTMDBHighlight` 与 `PublicTMDBHighlights` schema。两个 operation 都显式设置 `security: []`，覆盖文档的全局 Cookie security。图片响应声明 `image/jpeg`、`image/png`、`image/webp`，签名参数为 required。然后运行：

Run: `npm --prefix web run api:generate`

Expected: `web/src/api/generated.ts` 更新且命令退出 0。

**Step 7: 验证后端与契约**

Run: `go test ./internal/httpapi ./internal/integrations/tmdb -count=1`

Run: `npm --prefix web run api:check`

Expected: 全部 PASS。

**Step 8: 提交**

```bash
git add internal/httpapi/public_tmdb_handlers.go internal/httpapi/public_tmdb_handlers_test.go internal/httpapi/router.go internal/httpapi/tmdb_handlers.go api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: expose public TMDB highlights and images"
```

## Task 4: 所有已登录 TMDB 图片改为本站签名 URL

**Files:**
- Modify: `internal/httpapi/tmdb_handlers.go`
- Modify: `internal/httpapi/tmdb_handlers_test.go`
- Modify: `internal/httpapi/media_handlers.go`
- Modify: `internal/httpapi/media_handlers_test.go`
- Modify: `internal/httpapi/record_handlers.go`
- Modify: `internal/httpapi/record_handlers_test.go`
- Modify: `internal/httpapi/router.go`

**Step 1: 写响应 URL 失败测试**

更新 handler 快照断言，所有非空 TMDB 图片字段必须以 `/api/v1/public/tmdb/images/` 开头：

```go
require.Contains(t, response.Body.String(),
	`"posterPath":"/api/v1/public/tmdb/images/w342/`)
require.Contains(t, response.Body.String(),
	`"backdropPath":"/api/v1/public/tmdb/images/w1280/`)
require.NotContains(t, response.Body.String(), "https://image.tmdb.org")
```

覆盖搜索海报、媒体详情海报与 backdrop、季海报、分集剧照、演员头像、本地影库中持久化的 TMDB poster path。空路径继续返回空字符串或 null，不生成 URL。

**Step 2: 运行目标测试并确认失败**

Run: `go test ./internal/httpapi -run 'TestTMDB|TestMedia|TestLibrary' -count=1`

Expected: FAIL，响应仍包含原始 `/poster.jpg` 路径。

**Step 3: 提取统一签名映射器**

在 `tmdb_handlers.go` 增加只负责受信任 TMDB path 的 helper：

```go
func signedTMDBImageURL(client *tmdb.Client, size, path string, now time.Time) string {
	if path == "" { return "" }
	expires := now.Add(24 * time.Hour)
	signature, err := client.SignImage(size, path, expires)
	if err != nil { return "" }
	return buildPublicTMDBImageURL(size, path, expires, signature)
}
```

把 `now func() time.Time` 注入 handlers，测试使用固定时钟。`recordHandlers` 注入只用于签名的 TMDB client；不让 records domain 依赖集成层。

**Step 4: 映射全部尺寸**

- search、library、media、season poster → `w342`
- credits profile → `w300`
- episode still → `w780`
- movie/tv/media backdrop → `w1280`

保留完整自定义 `http(s)` 图片 URL，不把任意 URL送入 TMDB 签名器。

**Step 5: 运行测试**

Run: `go test ./internal/httpapi -count=1`

Expected: PASS。

**Step 6: 提交**

```bash
git add internal/httpapi/tmdb_handlers.go internal/httpapi/tmdb_handlers_test.go internal/httpapi/media_handlers.go internal/httpapi/media_handlers_test.go internal/httpapi/record_handlers.go internal/httpapi/record_handlers_test.go internal/httpapi/router.go
git commit -m "feat: proxy authenticated TMDB image responses"
```

## Task 5: 前端热门 API、图片解析和可复用轮播

**Files:**
- Create: `web/src/features/highlights/BackdropCarousel.tsx`
- Create: `web/src/features/highlights/BackdropCarousel.test.tsx`
- Create: `web/src/lib/mediaAccent.ts`
- Create: `web/src/lib/mediaAccent.test.ts`
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/api/client.test.ts`

**Step 1: 写热门 API 失败测试**

MSW 返回两项热门内容，断言 `getTMDBHighlights()` 保留本站 `backdropURL`，并且错误不会被转换为虚构数据。

```ts
const items = await getTMDBHighlights()
expect(items[0]).toMatchObject({ mediaType: 'movie', title: '降临' })
expect(items[0]?.backdropURL).toMatch(/^\/api\/v1\/public\/tmdb\/images\/w1280\//)
```

**Step 2: 实现类型与 API**

在 `types.ts` 增加：

```ts
export type TMDBHighlight = {
  id: number
  mediaType: MediaType
  title: string
  originalTitle: string
  year: string
  overview: string
  backdropURL: string
}
```

在 `client.ts` 增加 `getTMDBHighlights(signal?)`，只请求本站公开接口。

**Step 3: 写轮播失败测试**

使用 fake timers 验证：第一张 decode 成功后显示、7 秒后切换、手动下一张后暂停、`document.hidden` 时不推进、全部 `error` 后添加 `is-empty` 状态、减少动态效果时不建立 interval。

组件公开接口固定为：

```ts
type BackdropCarouselProps = {
  items: TMDBHighlight[]
  intervalMs: number
  showControls?: boolean
  onActiveItemChange?: (item: TMDBHighlight | null) => void
}
```

**Step 4: 实现轮播**

组件同时保留当前与下一图层，通过 `opacity` 完成交叉淡入；只有 `img.decode()` 成功的图片进入 ready 队列。背景图片使用空 `alt` 和 `aria-hidden="true"`。控制按钮使用 `ChevronLeft`、`ChevronRight`、`Pause`，并提供中文 `aria-label`。

**Step 5: 写并实现动态取色纯函数**

先测试 `selectMediaAccent(Uint8ClampedArray)` 忽略近黑、近白和低饱和像素，输出受限的 OKLCH 或 null。再实现 `sampleMediaAccent(img)`：把同源代理图缩到 24×14 canvas，读取像素，失败时返回 null。动态色只写 `--media-accent`。

**Step 6: 运行前端目标测试**

Run: `npm --prefix web test -- --run src/api/client.test.ts src/features/highlights/BackdropCarousel.test.tsx src/lib/mediaAccent.test.ts`

Expected: PASS。

**Step 7: 提交**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.test.ts web/src/features/highlights web/src/lib/mediaAccent.ts web/src/lib/mediaAccent.test.ts
git commit -m "feat: add TMDB backdrop carousel primitives"
```

## Task 6: 基础视觉 token、字体与控件状态

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `web/src/main.tsx`
- Modify: `web/src/styles/tokens.css`
- Modify: `web/src/styles/global.css`
- Modify: `web/src/app/App.test.tsx`

**Step 1: 安装本地展示字体**

Run: `npm --prefix web install @fontsource-variable/outfit@5.2.8`

Expected: `package.json` 与 lockfile 只新增该依赖。

在 `main.tsx` 引入 `@fontsource-variable/outfit`，不添加外部 `<link>`。

**Step 2: 写基础控件行为测试**

在 `App.test.tsx` 增加主记录按钮和导航当前项的可访问断言；避免用 class 快照验证视觉。CSS 视觉由 Playwright 覆盖。

**Step 3: 重构 token**

在 `tokens.css` 建立：

```css
--canvas: oklch(0.99 0.002 240);
--surface: oklch(0.965 0.005 240);
--surface-raised: oklch(1 0 0);
--ink: oklch(0.18 0.012 240);
--muted: oklch(0.48 0.016 240);
--border: oklch(0.86 0.008 240);
--brand: oklch(0.55 0.18 25);
--media-accent: var(--brand);
--focus: oklch(0.62 0.16 35);
--font-display: "Outfit Variable", var(--font-sans);
--control-sm: 36px;
--control-md: 44px;
--control-lg: 48px;
--duration-fast: 140ms;
--duration-normal: 240ms;
--duration-cinematic: 800ms;
```

深色覆盖使用 `--canvas: oklch(0.08 0.006 240)`、`--surface: oklch(0.12 0.008 240)`、`--surface-raised: oklch(0.16 0.01 240)`、`--ink: oklch(0.95 0.004 240)`、`--muted: oklch(0.68 0.012 240)` 与 `--border: oklch(0.27 0.01 240)`。登录与首页的无图降级单独使用纯白，不被深色主题覆盖。

保留旧 token alias，直到所有页面迁移完成，避免一次性破坏未改造组件。

**Step 4: 统一基础按钮与表单**

在 `global.css` 增加 `.button-primary`、`.button-secondary`、`.button-text`、`.icon-button`，并让现有 `.primary-button`、`.record-button` 组合复用这些声明。补齐 hover、active、disabled、loading 和 `:focus-visible`。所有半径不超过 8px，字距保持 0。

统一 `input`、`select`、`textarea` 的 44px 最小高度、错误边框和 focus ring。减少动态效果媒体查询把 transform 与 cinematic transition 归零。

**Step 5: 运行验证**

Run: `npm --prefix web test -- --run src/app/App.test.tsx`

Run: `npm --prefix web run typecheck`

Run: `npm --prefix web run build`

Expected: 全部退出 0。

**Step 6: 提交**

```bash
git add web/package.json web/package-lock.json web/src/main.tsx web/src/styles/tokens.css web/src/styles/global.css web/src/app/App.test.tsx
git commit -m "style: establish cinematic UI foundations"
```

## Task 7: 登录、初始化和认证异常页

**Files:**
- Modify: `web/src/features/auth/AuthGate.tsx`
- Modify: `web/src/features/auth/AuthGate.test.tsx`
- Modify: `web/src/styles/global.css`
- Modify: `web/e2e/setup.spec.ts`
- Create: `web/e2e/auth-visual.spec.ts`

**Step 1: 写登录轮播与白底失败测试**

在 `AuthGate.test.tsx` 增加 MSW 热门响应，断言：

- 登录表单不等待热门请求即可出现。
- 热门成功时渲染装饰性 backdrop。
- 热门 502 或所有图片 error 时 `.auth-page` 具有 `is-empty-backdrop`。
- 用户名、密码标签始终可见。
- “显示密码”按钮在 `password` 与 `text` 之间切换。
- 失败登录保留用户名和密码可重新输入。

**Step 2: 运行测试并确认失败**

Run: `npm --prefix web test -- --run src/features/auth/AuthGate.test.tsx`

Expected: FAIL，找不到显示密码按钮与轮播。

**Step 3: 实现共享 AuthBackdrop**

`AuthGate` 顶层认证状态页都使用同一个 `AuthBackdrop`，内部独立 `useQuery({ queryKey: ['tmdb-highlights'], retry: false })`。表单永远先渲染，轮播只是绝对定位装饰层。登录轮播 `intervalMs={7000}` 且不显示可见标题。

为密码输入增加 `Eye`/`EyeOff` 图标按钮；按钮保持 44×44px，使用 `aria-pressed` 与动态 `aria-label`。

**Step 4: 实现认证视觉**

桌面 `.auth-panel` 最大宽 420px，初始化最大宽 520px，8px 圆角、半透明白表面和中性遮罩。空 backdrop 使用纯白页面与深色表单；移动端提高面板不透明度并为软键盘保留底部空间。

**Step 5: 更新浏览器测试**

在 `auth-visual.spec.ts` 覆盖 375×812、768×1024、1440×900；测试可见图片与图片失败白底两种场景。`setup.spec.ts` 继续验证初始化安全流程。

**Step 6: 运行验证**

Run: `npm --prefix web test -- --run src/features/auth/AuthGate.test.tsx`

Run: `npm --prefix web run e2e -- auth-visual.spec.ts setup.spec.ts`

Expected: PASS；截图经人工检查无重叠和裁切。

**Step 7: 提交**

```bash
git add web/src/features/auth/AuthGate.tsx web/src/features/auth/AuthGate.test.tsx web/src/styles/global.css web/e2e/setup.spec.ts web/e2e/auth-visual.spec.ts web/e2e/auth-visual.spec.ts-snapshots
git commit -m "feat: redesign authentication with TMDB backdrops"
```

## Task 8: 顶部导航、移动导航与 404

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Create: `web/src/app/NotFoundPage.tsx`
- Create: `web/src/app/NotFoundPage.test.tsx`
- Modify: `web/src/styles/global.css`
- Modify: `web/e2e/accessibility.spec.ts`

**Step 1: 写应用框架失败测试**

断言桌面主导航与品牌位于 banner 内，不再渲染 complementary sidebar；未知路由显示“没有找到这份档案”、返回首页与搜索按钮。保留主导航可访问名称与当前页状态。

**Step 2: 运行测试并确认失败**

Run: `npm --prefix web test -- --run src/app/App.test.tsx src/app/NotFoundPage.test.tsx`

Expected: FAIL，仍存在 sidebar 且未知路由为空。

**Step 3: 重构 ApplicationShell**

结构固定为：

```tsx
<div className="app-shell">
  <a className="skip-link" href="#main-content">跳到主要内容</a>
  <header className="app-header">...</header>
  <main id="main-content" className="main-content">...</main>
  <MobileNavigation ... />
</div>
```

桌面 header 内放品牌、主导航、搜索和记录；设置保留在主导航。首页与详情路由通过 pathname 添加 `immersive-header`，使用 IntersectionObserver 或滚动阈值添加 `is-scrolled`。组件卸载时移除监听。

**Step 4: 增加 404 路由**

Routes 最后添加 `<Route path="*" element={<NotFoundPage onSearch={...} />} />`。404 使用 BrandMark、返回首页和搜索操作，不使用插图。

**Step 5: 更新 CSS 与键盘测试**

移除 sidebar 固定宽度和 `.app-column` 左外边距；主内容默认最大 1440px，immersive 页面允许 hero 全宽。移动底栏保持现有六项并增加 `env(safe-area-inset-bottom)`。

更新 `accessibility.spec.ts` 的 Tab 顺序，使它匹配顶部导航；继续验证 skip link、200% zoom 与减少动态效果。

**Step 6: 运行验证**

Run: `npm --prefix web test -- --run src/app/App.test.tsx src/app/NotFoundPage.test.tsx`

Run: `npm --prefix web run e2e -- accessibility.spec.ts`

Expected: PASS。

**Step 7: 提交**

```bash
git add web/src/app/App.tsx web/src/app/App.test.tsx web/src/app/NotFoundPage.tsx web/src/app/NotFoundPage.test.tsx web/src/styles/global.css web/e2e/accessibility.spec.ts
git commit -m "feat: replace sidebar with cinematic top navigation"
```

## Task 9: 个性化首页主视觉与内容布局

**Files:**
- Create: `web/src/features/home/HomeHero.tsx`
- Create: `web/src/features/home/HomeHero.test.tsx`
- Modify: `web/src/features/home/HomePage.tsx`
- Modify: `web/src/features/home/HomePage.test.tsx`
- Modify: `web/src/styles/global.css`
- Modify: `web/e2e/visual.spec.ts`

**Step 1: 写首页数据优先级失败测试**

覆盖：

1. 正在观看且有 TMDB backdrop 时优先进入 hero。
2. 最近记录补足到最多 6 项并去重。
3. 个人内容没有成功 backdrop 时使用热门列表。
4. 两类数据都失败时显示纯白 hero，继续观看和最近记录仍可操作。
5. 继续剧集主按钮文案包含下一季集；电影或已完成内容使用“查看记录”。

**Step 2: 运行测试并确认失败**

Run: `npm --prefix web test -- --run src/features/home/HomePage.test.tsx src/features/home/HomeHero.test.tsx`

Expected: FAIL，HomeHero 尚不存在。

**Step 3: 实现 HomeHero 数据组合**

从现有 watching 与 all library 查询取最多 6 个带 `tmdbId` 的唯一项目，使用 `useQueries` 请求对应 movie/tv details；把成功的 `backdropPath` 组合为 hero item。只在不足时读取共享的 `['tmdb-highlights']` 查询。

不要把 TMDB 错误合并进首页总体错误。Hero 组件接受可执行的本地 item 与只读热门 item，热门项只提供“搜索此片”或无主操作，不创建本地记录。

**Step 4: 实现首页布局**

- Hero 桌面高度 `min(72dvh, 720px)` 且最小 520px。
- 移动端高度 520–600px，标题不使用 viewport 字号。
- 继续观看改为海报轨道，保留现有推进与撤销逻辑。
- 最近记录改为一项 featured + 紧凑列表，不复制编辑表单。
- 三个区域分别显示 skeleton 与错误。

**Step 5: 更新视觉快照**

在 `visual.spec.ts` 新增首页浅色、深色、三个 viewport 快照；测试图片全部失败时再保存一张白底移动端快照。检查首屏能看到下一段内容。

**Step 6: 运行验证**

Run: `npm --prefix web test -- --run src/features/home`

Run: `npm --prefix web run e2e -- visual.spec.ts`

Expected: PASS；无横向溢出和按钮遮挡。

**Step 7: 提交**

```bash
git add web/src/features/home web/src/styles/global.css web/e2e/visual.spec.ts web/e2e/visual.spec.ts-snapshots
git commit -m "feat: add personalized cinematic home"
```

## Task 10: 影库、片单栏与搜索体验

**Files:**
- Modify: `web/src/features/library/LibraryPage.tsx`
- Modify: `web/src/features/library/LibraryPage.test.tsx`
- Modify: `web/src/features/collections/CollectionManager.tsx`
- Modify: `web/src/features/collections/CollectionManager.test.tsx`
- Modify: `web/src/features/search/SearchDialog.tsx`
- Modify: `web/src/features/search/SearchDialog.test.tsx`
- Modify: `web/src/features/media/MediaPoster.tsx`
- Create: `web/src/features/media/MediaPoster.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: 写海报 URL 与失败状态测试**

断言本站 `/api/v1/public/tmdb/images/...` URL原样进入 `img.src`，代码中不再拼接 `image.tmdb.org`。触发 error 后图片被隐藏，显示标题首字占位且保留准确 accessible name。

**Step 2: 写影库和搜索交互测试**

覆盖状态 segmented control、片单选择、片单创建折叠、空态搜索，以及 SearchDialog 的本地/TMDB 分组、ArrowDown/ArrowUp/Enter/Escape 操作。最近搜索只存在 sessionStorage，不写后端。

**Step 3: 运行测试并确认失败**

Run: `npm --prefix web test -- --run src/features/library src/features/collections src/features/search src/features/media/MediaPoster.test.tsx`

Expected: 至少海报直连与搜索键盘测试 FAIL。

**Step 4: 实现响应式海报墙**

桌面使用 `repeat(auto-fill, minmax(168px, 1fr))` 并限制最大列宽；手机使用稳定两列。海报框固定 `aspect-ratio: 2 / 3`。状态、标题、年份和进度有稳定行高，hover 层不改变布局；触屏不依赖 hover。

**Step 5: 收紧片单与搜索**

CollectionManager 默认显示横向片单选择与创建图标按钮，创建表单按需展开。SearchDialog 使用本地结果、TMDB 结果两个 section，结果项使用稳定 48–64px 行高和代理海报。

**Step 6: 运行验证**

Run: `npm --prefix web test -- --run src/features/library src/features/collections src/features/search src/features/media/MediaPoster.test.tsx`

Run: `npm --prefix web run typecheck`

Expected: PASS。

**Step 7: 提交**

```bash
git add web/src/features/library web/src/features/collections web/src/features/search web/src/features/media/MediaPoster.tsx web/src/features/media/MediaPoster.test.tsx web/src/styles/global.css
git commit -m "feat: redesign library and search discovery"
```

## Task 11: 详情页代理图片与视觉层次

**Files:**
- Modify: `web/src/features/media/MediaHero.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.tsx`
- Modify: `web/src/features/media/MediaDetailsPage.test.tsx`
- Modify: `web/src/features/media/CastStrip.tsx`
- Create: `web/src/features/media/CastStrip.test.tsx`
- Modify: `web/src/styles/global.css`
- Modify: `web/e2e/visual.spec.ts`

**Step 1: 写图片代理与失败测试**

断言 MediaHero、CastStrip 和季集组件只使用服务端返回的本站 URL；图片 error 后 backdrop 切换中性头部、头像切换姓名首字，不显示破图图标。动态取色失败时 `--media-accent` 回落品牌色。

**Step 2: 运行测试并确认失败**

Run: `npm --prefix web test -- --run src/features/media`

Expected: FAIL，MediaHero/CastStrip 仍拼接 `image.tmdb.org`。

**Step 3: 移除前端 TMDB URL 拼接**

删除 `MediaPoster`、`MediaHero`、`CastStrip` 中所有 `https://image.tmdb.org` 字符串。只接受本站相对 URL或已有自定义绝对 URL；TMDB 原始 path 被视为无可用图片并显示占位。

**Step 4: 完成详情视觉**

Hero 延伸到顶部导航下方；桌面海报、标题和简介形成叠层，移动端改为单列。演员轨道保持 2:3 和固定列宽。现有剧集工作区、双栏个人记录、重看和家庭整理 DOM 顺序不变，只调整表面、间距和操作权重。

**Step 5: 更新视觉与功能回归**

更新 details 三尺寸浅深截图。继续运行现有剧集、记录、重看 E2E，确保视觉改造没有破坏保存与批量操作。

**Step 6: 运行验证**

Run: `npm --prefix web test -- --run src/features/media src/features/episodes src/features/records`

Run: `npm --prefix web run e2e -- visual.spec.ts episodes.spec.ts recording.spec.ts`

Expected: PASS。

**Step 7: 提交**

```bash
git add web/src/features/media web/src/styles/global.css web/e2e/visual.spec.ts web/e2e/visual.spec.ts-snapshots
git commit -m "style: deepen cinematic media details"
```

## Task 12: 日历、统计、设置与统一状态

**Files:**
- Modify: `web/src/features/calendar/CalendarPage.tsx`
- Modify: `web/src/features/calendar/CalendarPage.test.tsx`
- Modify: `web/src/features/calendar/MonthGrid.tsx`
- Modify: `web/src/features/calendar/AgendaList.tsx`
- Modify: `web/src/features/stats/StatsPage.tsx`
- Modify: `web/src/features/stats/StatsPage.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: 写响应式语义测试**

保持日历筛选、月份切换和统计 accessible chart 的现有断言；新增设置章节导航的链接/按钮断言，以及所有页面统一空态与重试操作的 accessible name。

**Step 2: 运行测试并确认新增断言失败**

Run: `npm --prefix web test -- --run src/features/calendar src/features/stats src/app/App.test.tsx`

Expected: FAIL，设置章节导航尚不存在。

**Step 3: 改造辅助页面**

- Calendar：桌面月历 + 日程；移动端日程优先，月历可切换。
- Stats：大数字 summary band、无障碍图表和统一 tabular numbers。
- Settings：账户、TMDB 与媒体服务器、家庭成员、数据、备份五个锚点章节；不嵌套卡片。
- 空态：BrandMark、短文案、一个操作。
- 错误：区域内重试；不扩大到全页。

**Step 4: 运行验证**

Run: `npm --prefix web test -- --run src/features/calendar src/features/stats src/features/settings src/features/household src/app/App.test.tsx`

Run: `npm --prefix web run typecheck`

Expected: PASS。

**Step 5: 提交**

```bash
git add web/src/features/calendar web/src/features/stats web/src/app/App.tsx web/src/app/App.test.tsx web/src/styles/global.css
git commit -m "style: unify calendar stats and settings"
```

## Task 13: 全站视觉、网络、无障碍与最终验证

**Files:**
- Modify: `web/e2e/visual.spec.ts`
- Modify: `web/e2e/accessibility.spec.ts`
- Modify: `web/e2e/support.ts`
- Create: `web/e2e/image-proxy.spec.ts`
- Modify: `web/e2e/visual.spec.ts-snapshots/*`
- Modify: `web/e2e/auth-visual.spec.ts-snapshots/*`
- Modify: `docs/release-checklist.md`

**Step 1: 增加网络边界测试**

`image-proxy.spec.ts` 监听 request：

```ts
const directTMDBImages: string[] = []
page.on('request', (request) => {
  if (new URL(request.url()).hostname === 'image.tmdb.org') directTMDBImages.push(request.url())
})
```

依次访问登录、首页、影库和详情，等待可见图片完成加载，最后断言数组为空且本站 `/api/v1/public/tmdb/images/` 请求存在。

**Step 2: 扩大视觉矩阵**

固定覆盖：

- 登录：有图、全部失败白底。
- 首页：个人 hero、热门补充、白底。
- 影库：常规、空态。
- 详情：backdrop、无 backdrop。
- 375×812、768×1024、1440×900。
- light、dark（登录白底降级保持白色，不被系统深色覆盖）。

每张截图前等待字体、图片 decode 和 query settle；禁用动画，但不能用隐藏元素绕过布局问题。

**Step 3: 运行浏览器验收并人工检查**

Run: `npm --prefix web run e2e -- auth-visual.spec.ts visual.spec.ts image-proxy.spec.ts accessibility.spec.ts`

Expected: PASS。逐张检查：文字不溢出、按钮不遮挡、首屏可见下一段、海报比例稳定、移动底栏不覆盖内容。

**Step 4: 运行全量后端验证**

Run: `go test ./... -race -count=1`

Run: `go vet ./...`

Expected: PASS，无 race 和 vet 输出。

**Step 5: 运行全量前端验证**

Run: `npm --prefix web test -- --run`

Run: `npm --prefix web run lint`

Run: `npm --prefix web run typecheck`

Run: `npm --prefix web run build`

Run: `npm --prefix web run api:check`

Expected: 全部退出 0。

**Step 6: 运行文档与发布检查**

在 `docs/release-checklist.md` 增加 TMDB 图片代理、登录白底降级、三尺寸视觉与直接图片域名检查项。

Run: `./scripts/docs-acceptance-test.sh`

Expected: PASS。

**Step 7: 检查工作区与提交最终验收**

Run: `git diff --check`

Run: `git status --short`

Expected: 只有本任务预期文件，无构建产物、Playwright report 或临时文件。

```bash
git add web/e2e docs/release-checklist.md
git commit -m "test: verify entertainment UI redesign"
```

## 完成标准

- 登录页在未认证状态读取 TMDB 热门电影和剧集，图片与 API 都经过 Compose 所在服务端网络。
- TMDB 或全部图片失败时，登录页与首页使用纯白背景且核心操作立即可用。
- 浏览器不直接请求 `image.tmdb.org`。
- 桌面使用顶部导航，移动端底栏不遮挡内容。
- 首页、影库和详情由真实影视画面主导，辅助页面保持高效清晰。
- 375×812、768×1024、1440×900 浅深主题无横向溢出、重叠或文本裁切。
- 自动轮播可暂停，并在 `prefers-reduced-motion` 下停止。
- Go、Vitest、lint、typecheck、build、API contract、Playwright、axe 和文档验收全部通过。

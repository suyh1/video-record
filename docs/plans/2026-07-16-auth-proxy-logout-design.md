# HTTPS 代理认证与可靠退出设计

## 问题

登录和退出都是需要同源校验的 `POST`。当前后端只通过 `r.TLS` 判断请求协议；HTTPS 在反向代理终止后，应用收到的是 HTTP，因此会把正常的 `Origin: https://...` 拒绝为 `invalid_origin`。登录页把该响应显示为连接失败，退出页显示退出失败。

退出还有第二个脆弱点：页面可通过持久 HttpOnly 会话 cookie 恢复登录，但 CSRF token 只保存在标签页级 `sessionStorage`。恢复 cookie 而没有恢复该 token，或另一次登录已经轮换旧会话时，用户仍可能停留在页面中却无法执行当前退出请求。

## 决策

同源校验继续要求 Origin 的 scheme 和 host 都与原请求一致。直连 TLS 时以 `r.TLS` 为准；TLS 未在应用终止时，读取标准 `Forwarded` 的首个 `proto`，并兼容常见的 `X-Forwarded-Proto` 首值。只接受 `http` 或 `https`；两个代理头冲突、值非法或 Origin 携带用户信息、路径、查询、片段时都拒绝。代理头不改变可信 Host，浏览器 Origin 仍必须与 `r.Host` 精确匹配。

退出改为只要求同源，不再要求 CSRF 或预先通过会话认证。Handler 在存在 cookie 时尝试撤销对应会话，并始终下发同路径的过期 HttpOnly cookie；没有 cookie、cookie 已失效或会话已撤销都返回 `204`。数据库故障仍返回 `500`，避免谎报已完成服务端撤销。

其他所有业务写请求继续要求有效会话、同源 Origin、CSRF token，并在适用处要求幂等键。退出的豁免只降低被同源保护挡住的“强制登出”风险，不授予读取或写入用户数据的能力。

## 数据流

1. 反向代理保留原始 Host 和 Origin，并设置 `Forwarded: proto=https` 或 `X-Forwarded-Proto: https`。
2. `RequireSameOrigin` 解析有效请求协议，比较 Origin scheme 与该协议、Origin host 与 `r.Host`。
3. 登录通过后继续创建不透明数据库会话，设置 HttpOnly/SameSite cookie，并把 CSRF token 返回给前端。
4. 退出只在同源校验通过后进入 handler；有 cookie 则撤销，无 cookie 也继续清理浏览器 cookie，最后返回 `204`。
5. 前端收到 `204` 后移除本地 CSRF token 并重置当前用户查询，返回登录页。

## 错误边界

- 缺失、跨站、冲突或非法 Origin/代理协议：`403 invalid_origin`。
- 登录凭据不正确和限流语义保持不变。
- 退出时会话不存在或已撤销：`204`，并清除 cookie。
- 退出时数据库撤销失败：`500 internal_error`，前端保留错误提示。

## 测试

- HTTP 路由测试先证明 HTTPS Origin + 代理协议当前被拒，再覆盖 `Forwarded` 和 `X-Forwarded-Proto` 的成功登录。
- 覆盖代理头冲突、非法协议、错误 Host 和缺失 Origin仍返回 `403`。
- 退出回归覆盖无 CSRF、有有效会话、已撤销会话和无 cookie；全部在同源时返回 `204` 并下发过期 cookie，跨站请求仍被拒。
- OpenAPI 和生成类型移除退出的 CSRF 参数；前端客户端不再从 `sessionStorage` 读取退出 token。
- 定向认证测试、全仓 race/vet、前端测试/lint/typecheck/build、API 漂移、真实浏览器登录-退出-重新登录流程全部通过后才完成。

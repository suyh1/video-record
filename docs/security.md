# 安全

## 部署边界

video-record 面向家庭自托管，不是面向公网匿名用户的多租户服务。生产部署应位于可信网络或经过认证、限流和 TLS 的反向代理之后。不要把 Docker socket、SQLite 文件或 `/data` volume 暴露给应用容器以外的进程。

容器使用只读根文件系统、固定 nonroot 用户 `65532:65532`、drop ALL capabilities 和 `no-new-privileges`。只有 `/data` 可写。

## 认证与浏览器安全

- 密码使用 Argon2id。
- 会话是数据库保存的随机不透明 token。
- Cookie 使用 HttpOnly 与 SameSite=Lax；生产环境保持 `APP_COOKIE_SECURE=true`。
- 写请求校验 Origin、CSRF token 和幂等键。
- 登录限流，管理员可通过成员停用或密码重置撤销会话。
- API 错误使用带稳定 code 和 requestId 的 Problem Details，不回传内部错误或上游正文。

纯 HTTP 只能用于本机或隔离的可信网络验证。面向局域网其他设备或公网时必须使用 HTTPS。

## 凭据

`TMDB_READ_ACCESS_TOKEN` 只能由服务端环境变量读取。真实值不得进入源码、前端环境、Dockerfile、Compose 默认值、测试夹具、日志、错误、导出、备份、SBOM 或镜像层。

`APP_ENCRYPTION_KEY` 必须是 Base64 编码的随机 32 byte 值。媒体服务器凭据使用该密钥进行 AES-256-GCM 加密；数据库只保存密文、nonce、版本和不可逆指纹。密钥本身不进入数据库或备份。

Compose 部署把凭据直接填写在 `docker-compose.yml` 中；这些本机敏感修改不得提交到仓库。生产密钥原值还应保存在密码管理器、主机密钥存储或受保护的部署 secret 中，不能只依赖 Compose 工作副本。

## 数据与备份

评分、笔记、标签和片单默认属于用户私有数据。共享必须显式开启；共同观看者只能看到共享事件的最小字段。审计事件不保存笔记正文、Authorization 头或凭据。

`.vrbackup` 包含数据库快照，可能包含私人记录和加密凭据。归档没有使用 `APP_ENCRYPTION_KEY` 再加密，必须存放在访问受控的加密介质。恢复前校验来源与 SHA-256。

## 网络集成

TMDB、Jellyfin、Emby 和 Plex 请求都由服务端发起。只允许为可信媒体服务器创建账户；媒体服务器 Base URL 可能指向内网地址，因此能够配置集成的用户不应是不受信任的访客。Provider token 只通过请求头发送，不应出现在 URL、代理访问日志或截图中。

## 密钥扫描

提交前扫描工作树与 Git 历史：

```bash
go run github.com/zricethezav/gitleaks/v8@v8.30.1 dir . --redact --no-banner
go run github.com/zricethezav/gitleaks/v8@v8.30.1 git --redact --no-banner
```

构建镜像后扫描 OCI/Docker archive 的配置、历史、层文件和二进制字符串：

```bash
./scripts/image-secret-scan.sh local/video-record:release-candidate
```

`image-secret-scan` 与 gitleaks 都必须无匹配。扫描输出不得关闭 redaction，也不要把命中的真实值粘贴到 issue 或 CI 日志。

## 漏洞与供应链

- Go 使用 `govulncheck` 扫描可达漏洞。
- npm 使用高危阈值审计。
- 镜像使用 Docker Scout 阻断 Critical/High。
- GitHub Actions 固定到完整 commit SHA。
- 发布镜像生成 SBOM 与最大 provenance。
- amd64 与 arm64 在任何 push 前分别烟测、漏洞扫描和层密钥扫描。

发现高危或严重漏洞时停止发布，优先升级受影响的最小依赖或基础镜像补丁版本，并重新执行完整门禁。

## 安全报告

公开仓库启用后，应在仓库安全策略中提供私密报告渠道。报告中不要包含真实令牌、数据库、备份、家庭成员信息或可访问的内网地址；使用合成值和最小复现。

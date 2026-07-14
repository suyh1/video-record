#!/bin/sh
set -eu

failures=0

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

require_file() {
  file=$1
  if [ ! -s "$file" ]; then
    fail "$file is missing or empty"
  fi
}

require_pattern() {
  file=$1
  pattern=$2
  description=$3

  if [ ! -f "$file" ] || ! grep -Eiq "$pattern" "$file"; then
    fail "$file must document $description"
  fi
}

require_file README.md
require_file docs/deployment.md
require_file docs/backup-restore.md
require_file docs/upgrading.md
require_file docs/security.md
require_file docs/integrations.md
require_file docs/release-checklist.md

require_pattern README.md 'docker compose (up|安装|install)' 'a fresh-machine Compose installation'
require_pattern docs/deployment.md '(openssl rand -base64 32|随机.*32.*byte|32.*byte.*随机)' 'generation of a random 32-byte encryption key'
require_pattern docs/deployment.md 'VIDEO_RECORD_PORT' 'host port changes'
require_pattern docs/deployment.md '(amd64|x86_64)' 'amd64 deployment support'
require_pattern docs/deployment.md '(arm64|aarch64)' 'arm64 deployment support'
require_pattern docs/integrations.md 'TMDB_READ_ACCESS_TOKEN' 'the server-only TMDB token environment variable'
require_pattern docs/integrations.md '(The Movie Database|TMDB)' 'TMDB attribution'
require_pattern docs/integrations.md '(Jellyfin|Emby|Plex)' 'supported playback-history providers'
require_pattern docs/backup-restore.md '(演练|rehears)' 'a backup and restore rehearsal'
require_pattern docs/backup-restore.md 'APP_ENCRYPTION_KEY' 'encryption-key retention'
require_pattern docs/backup-restore.md '(丢失|loss|lose|lost).*(锁定|lock).*集成|集成.*(锁定|lock).*(丢失|loss|lose|lost)' 'the effect of losing the encryption key on integrations'
require_pattern docs/backup-restore.md '(观影|viewing).*(仍|remain|不会).*(访问|可用|available)|记录.*(不受影响|仍可)' 'continued access to viewing records after key loss'
require_pattern docs/upgrading.md '(回滚|rollback)' 'upgrade rollback'
require_pattern docs/upgrading.md '(自动|automatic).*(迁移|migration).*(备份|backup)|(迁移|migration).*(前|before).*(备份|backup)' 'automatic pre-migration backup'
require_pattern docs/upgrading.md '(备份|backup).*(失败|fail).*(不|not).*(迁移|migration)|(迁移|migration).*(不会|not).*(开始|start)' 'migration refusal when the automatic backup fails'
require_pattern docs/security.md '(gitleaks|image-secret-scan)' 'repository or image secret scanning'
require_pattern docs/security.md '(TMDB_READ_ACCESS_TOKEN).*(服务端|server)' 'the TMDB server-only credential boundary'
require_pattern docs/release-checklist.md '(linux/amd64|amd64)' 'an amd64 smoke result'
require_pattern docs/release-checklist.md '(linux/arm64|arm64)' 'an arm64 smoke result'
require_pattern docs/release-checklist.md '(backup|备份).*(restore|恢复).*(演练|rehears)' 'a backup and restore rehearsal result'
require_pattern docs/release-checklist.md '(gitleaks|secret scan|密钥扫描)' 'secret-scan evidence'
require_pattern docs/release-checklist.md '(High|高危).*(0|zero)|(0|zero).*(High|高危)' 'zero high-severity vulnerabilities'
require_pattern docs/release-checklist.md '(Critical|严重).*(0|zero)|(0|zero).*(Critical|严重)' 'zero critical-severity vulnerabilities'
require_pattern docs/release-checklist.md '(v1\.0\.0).*(授权|authorization|未创建|not created)' 'the explicit-authorization boundary for the v1 tag'
require_pattern docs/release-checklist.md '(MUST|必须).*(映射|mapping)|设计 MUST 验收映射' 'the approved-design requirement mapping'
require_pattern docs/release-checklist.md '(44).*(测试|test)|(测试|test).*(44)' 'the current frontend test count'

if grep -ERq 'eyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}' \
  README.md docs/deployment.md docs/backup-restore.md docs/upgrading.md docs/security.md docs/integrations.md docs/release-checklist.md 2>/dev/null; then
  fail 'operator documentation must not contain JWT-shaped credentials'
fi

if [ "$failures" -ne 0 ]; then
  printf '%s documentation acceptance check(s) failed\n' "$failures" >&2
  exit 1
fi

printf 'documentation acceptance tests: passed\n'

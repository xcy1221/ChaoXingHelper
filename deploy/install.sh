#!/usr/bin/env bash
set -euo pipefail
# 首次安装；自动执行保持停用，账号核验通过后再启用。
[[ $(id -u) == 0 ]] || { echo "需要管理员运行此部署脚本"; exit 1; }
repo=$(cd "$(dirname "$0")/.." && pwd)
target=/home/docker/services/chaoxing-helper
readme=/home/docker/services/server-readme
caddy=/home/docker/services/caddy/Caddyfile
[[ -f "$repo/services/aigc/go.mod" ]] || { echo "缺少服务源码"; exit 1; }
[[ ! -e "$target" ]] || { echo "服务目录已存在，停止以避免覆盖"; exit 1; }
for file in panel.html panel.css panel.js; do
  [[ ! -e "$readme/docs/$file" ]] || { echo "面板资源已存在，停止以避免覆盖"; exit 1; }
done
for file in "$readme/nginx.conf" "$readme/docs/README.md" "$readme/docs/services.md"; do
  [[ -f "$file" ]] || { echo "服务器文档或配置缺失"; exit 1; }
done
cmp -s "$caddy" "$repo/deploy/chaoxing-helper/Caddyfile.before" || { echo "Caddy 配置已变化，请重新核对"; exit 1; }
private_config=${AIGC_PRIVATE_CONFIG:-}
if [[ -n "$private_config" ]]; then
  [[ -f "$private_config" ]] || { echo "私有配置文件不存在"; exit 1; }
fi
docker compose version
docker network inspect caddy-proxy >/dev/null
docker inspect caddy server-readme >/dev/null
for name in chaoxing-helper chaoxing-chromium chaoxing-cdp-bridge chaoxing-chromium-login; do
  if docker inspect "$name" >/dev/null 2>&1; then
    echo "目标容器名称已被占用：$name"; exit 1
  fi
done
mountpoint -q /home || { echo "/home 独立挂载未就绪"; exit 1; }
df -h /home
[[ $(df -Pk /home | awk 'NR==2 {print $4}') -gt 8388608 ]] || { echo "/home 可用空间不足 8 GB"; exit 1; }
[[ $(awk '/MemAvailable:/ {print $2}' /proc/meminfo) -gt 2097152 ]] || { echo "可用内存不足 2 GB"; exit 1; }
backup="$target/backups/$(date +%Y%m%d-%H%M%S)"
install -d -m 700 "$backup"
cp -p "$caddy" "$backup/Caddyfile"
cp -p "$readme/nginx.conf" "$backup/nginx.conf"
cp -p "$readme/docs/README.md" "$backup/README.md"
cp -p "$readme/docs/services.md" "$backup/services.md"
changed=0
app_attempted=0
manager_attempted=0
rollback() {
  failure_status=$?
  trap - ERR
  set +e
  if [[ $manager_attempted == 1 ]]; then systemctl disable --now chaoxing-browser-manager.service; fi
  if [[ $app_attempted == 1 ]]; then
    docker compose -f "$target/compose.yml" --profile worker --profile login stop || true
  fi
  if [[ $changed == 1 ]]; then
    cat "$backup/Caddyfile" > "$caddy"
    cat "$backup/nginx.conf" > "$readme/nginx.conf"
    cat "$backup/README.md" > "$readme/docs/README.md"
    cat "$backup/services.md" > "$readme/docs/services.md"
    rm -f -- "$readme/docs/panel.html" "$readme/docs/panel.css" "$readme/docs/panel.js"
    docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile || true
    docker exec server-readme nginx -s reload || true
  fi
  echo "部署未完成。配置备份：$backup；未删除应用数据。"
  exit "$failure_status"
}
trap rollback ERR
install -d "$target/source/services"
cp -a "$repo/services/aigc" "$target/source/services/aigc"
install -d -m 700 -o 65532 -g 65532 "$target/data" "$target/data/uploads" "$target/data/reports" "$target/data/account" "$target/browser-state"
install -m 644 "$repo/deploy/chaoxing-helper/compose.yml" "$target/compose.base.yml"
install -m 644 "$repo/deploy/chaoxing-helper/compose.browser.yml" "$target/compose.browser.yml"
for file in browser-manager.py Dockerfile.browser chaoxing-browser-manager.service; do
  install -m 644 "$repo/deploy/chaoxing-helper/$file" "$target/$file"
done
if [[ -z "$private_config" ]]; then
  private_config="$repo/services/aigc/platform/config.example.json"
fi
install -m 600 -o 65532 -g 65532 "$private_config" "$target/data/account/platform.json"
# 合成标准 compose.yml，后续管理员可直接使用 docker compose。
docker compose --project-name chaoxing-helper -f "$target/compose.base.yml" -f "$target/compose.browser.yml" config --output "$target/compose.yml"
docker compose -f "$target/compose.yml" config --quiet
docker compose -f "$target/compose.yml" build chaoxing-helper chromium
app_attempted=1
docker compose -f "$target/compose.yml" create --no-build chromium cdp-bridge
docker compose -f "$target/compose.yml" --profile login create --no-build chromium-login
docker compose -f "$target/compose.yml" up -d --no-deps --wait --wait-timeout 180 chaoxing-helper
install -m 644 "$target/chaoxing-browser-manager.service" /etc/systemd/system/chaoxing-browser-manager.service
systemctl daemon-reload
manager_attempted=1
systemctl enable --now chaoxing-browser-manager.service
docker cp "$repo/deploy/chaoxing-helper/Caddyfile" caddy:/tmp/chaoxing-Caddyfile
docker exec caddy caddy validate --config /tmp/chaoxing-Caddyfile --adapter caddyfile
changed=1
for file in panel.html panel.css panel.js; do
  install -m 644 "$repo/deploy/server-panel/$file" "$readme/docs/$file"
  chcon --reference="$readme/docs/README.md" "$readme/docs/$file"
done
printf '\n## Server panel\n\nPanel: /server-readme/; Agent entry: /server-readme/README.md. AIGC: /chaoxing/. Read operations.md before server changes.\n' >> "$readme/docs/README.md"
printf '\n## ChaoXingHelper\n\nGo service: /home/docker/services/chaoxing-helper/; data: data/; browser profile: browser-state/. UI: /chaoxing/. On-demand headless Chromium: idle stop after 30 seconds, managed by chaoxing-browser-manager.service. GUI is for administrator login only: HTTPS 127.0.0.1:43001, SSH tunnel only. CDP: internal browser-control network, no published port. No mihomo-proxy attachment. Check api/health ready before use. Server panel uses existing server-readme Nginx; Markdown paths remain available.\n' >> "$readme/docs/services.md"
# 保持现有绑定文件的 inode，容器可立即读到新内容。
cat "$repo/deploy/server-panel/nginx.conf" > "$readme/nginx.conf"
docker exec server-readme nginx -t
docker exec server-readme nginx -s reload
cat "$repo/deploy/chaoxing-helper/Caddyfile" > "$caddy"
docker exec caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
curl --insecure --fail --silent --show-error --retry 5 --retry-delay 1 --max-time 15 https://192.168.1.71/server-readme/ -o "$backup/panel-response.html"
grep -q '服务器面板' "$backup/panel-response.html"
curl --insecure --fail --silent --max-time 15 https://192.168.1.71/server-readme/README.md >/dev/null
curl --insecure --fail --silent --max-time 15 https://192.168.1.71/chaoxing/api/health
printf '\n页面和按需管理器已启动；完成原生登录、配置及真实流程验收后再启用检测。\n'
trap - ERR

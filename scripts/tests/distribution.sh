#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT HUP INT TERM
mkdir -p "$test_root/bin"
# 所有服务操作由夹具记录，绝不调用宿主机 systemd 或切换账户。
cat > "$test_root/bin/systemctl" <<'EOF'
#!/bin/sh
printf 'systemctl %s\n' "$*" >> "$EVENTS"
exit 0
EOF
cat > "$test_root/bin/runuser" <<'EOF'
#!/bin/sh
printf 'runuser %s\n' "$*" >> "$EVENTS"
shift 3
exec "$@"
EOF
chmod +x "$test_root/bin/"*
PATH="$test_root/bin:$PATH"
export PATH
for scenario in success health-failure missing-launcher agent; do
    root="$test_root/$scenario/installed"
    source="$test_root/$scenario/new"
    mkdir -p "$root/data" "$source/web" "$source/agents"
    cp "$script_dir/../distribution/start.sh" "$root/start.sh"
    printf 'original-identity\n' > "$root/data/identity"
    printf 'original-ca\n' > "$root/control-plane-ca.pem"
    binary=control-plane
    if [ "$scenario" = agent ]; then
        binary=agent
        printf 'original-config\n' > "$root/agent-config.json"
    fi
    printf 'old-binary\n' > "$root/$binary"
    cat > "$source/$binary" <<'EOF'
#!/bin/sh
printf 'binary %s\n' "$*" >> "$EVENTS"
if [ "$1" = -health-check ]; then exit "$HEALTH_EXIT"; fi
EOF
    chmod +x "$source/$binary"
    printf 'new-page\n' > "$source/web/index.html"
    # 与旧脚本长度不同，验证替换正在运行的入口不会破坏后续解释。
    printf '#!/bin/sh\nprintf "new launcher\\n"\n' > "$source/start.sh"
    if [ "$scenario" = missing-launcher ]; then rm "$source/start.sh"; fi
    EVENTS="$test_root/$scenario/events"
    HEALTH_EXIT=0
    if [ "$scenario" = health-failure ]; then HEALTH_EXIT=9; fi
    export EVENTS HEALTH_EXIT
    status=0
    printf '3\n%s\n' "$source" | sh "$root/start.sh" > "$test_root/$scenario/output" 2>&1 || status=$?
    if [ "$scenario" = missing-launcher ]; then
        [ "$status" -ne 0 ]
        ! grep -q 'systemctl stop' "$EVENTS"
        [ "$(cat "$root/$binary")" = old-binary ]
    else
        cmp "$source/start.sh" "$root/start.sh"
        cmp "$source/$binary" "$root/$binary"
        grep -q 'systemctl start' "$EVENTS"
        if [ "$scenario" = health-failure ]; then [ "$status" -ne 0 ]; else [ "$status" -eq 0 ]; fi
        if [ "$scenario" = agent ]; then
            ! grep -q 'binary -health-check' "$EVENTS"
            [ "$(cat "$root/agent-config.json")" = original-config ]
        else
            grep -q 'binary -health-check' "$EVENTS"
        fi
    fi
    [ "$(cat "$root/data/identity")" = original-identity ]
    [ "$(cat "$root/control-plane-ca.pem")" = original-ca ]
    printf 'PASS: %s\n' "$scenario"
done

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
case "$1" in
    cat) exit "${SERVICE_MISSING:-0}" ;;
    show)
        case "$3" in
            WorkingDirectory) printf 'WorkingDirectory=%s\n' "$SERVICE_ROOT" ;;
            User) printf 'User=%s\n' "${SERVICE_USER:-ob-data-orch-agent}" ;;
            ActiveState) printf 'ActiveState=%s\n' "${SERVICE_STATE:-inactive}" ;;
        esac ;;
    stop) exit "${STOP_EXIT:-0}" ;;
    start) exit "${START_EXIT:-0}" ;;
esac
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
    SERVICE_ROOT="$root"
    SERVICE_STATE=inactive
    HEALTH_EXIT=0
    if [ "$scenario" = health-failure ]; then HEALTH_EXIT=9; fi
    export EVENTS HEALTH_EXIT SERVICE_ROOT SERVICE_STATE
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

# 重新注册与恢复只在合成服务中验证，不能创建宿主机账户或写入 systemd 目录。
cat > "$test_root/bin/id" <<'EOF'
#!/bin/sh
[ "$1" = -u ] || exit 1
printf '%s\n' "${USER_ID:-0}"
EOF
chmod +x "$test_root/bin/id"
for scenario in register-success register-failure stop-failure still-active wrong-directory wrong-account uninstalled non-root control-plane start-retry start-failure already-running start-transition start-failed-service unknown-state start-service-failure start-wrong-directory start-wrong-account; do
    root="$test_root/$scenario/installed"
    mkdir -p "$root/data"
    cp "$script_dir/../distribution/start.sh" "$root/start.sh"
    printf 'original-config\n' > "$root/agent-config.json"
    printf 'original-ca\n' > "$root/control-plane-ca.pem"
    printf 'original-identity\n' > "$root/data/identity"
    cat > "$root/agent" <<'EOF'
#!/bin/sh
printf 'binary %s\n' "$*" >> "$EVENTS"
case "$*" in
    '-register -initialize') exit "$REGISTER_EXIT" ;;
    '-initialize') exit "$INIT_EXIT" ;;
    *) exit 99 ;;
esac
EOF
    chmod +x "$root/agent"
    EVENTS="$test_root/$scenario/events"
    SERVICE_ROOT="$root"
    SERVICE_USER=ob-data-orch-agent
    SERVICE_STATE=inactive
    SERVICE_MISSING=0
    STOP_EXIT=0
    START_EXIT=0
    REGISTER_EXIT=0
    INIT_EXIT=0
    USER_ID=0
    action=5
    case "$scenario" in
        register-failure) REGISTER_EXIT=9 ;;
        stop-failure) STOP_EXIT=9 ;;
        still-active) SERVICE_STATE=active ;;
        wrong-directory) SERVICE_ROOT="$test_root/other" ;;
        wrong-account) SERVICE_USER=other-service ;;
        uninstalled) SERVICE_MISSING=1 ;;
        non-root) USER_ID=1001 ;;
        control-plane) rm "$root/agent-config.json" ;;
        start-retry) action=1 ;;
        start-failure) action=1; INIT_EXIT=9 ;;
        already-running) action=1; SERVICE_STATE=active ;;
        start-transition) action=1; SERVICE_STATE=activating ;;
        start-failed-service) action=1; SERVICE_STATE=failed ;;
        unknown-state) action=1; SERVICE_STATE=unknown ;;
        start-service-failure) START_EXIT=9 ;;
        start-wrong-directory) action=1; SERVICE_ROOT="$test_root/other" ;;
        start-wrong-account) action=1; SERVICE_USER=other-service ;;
    esac
    export EVENTS SERVICE_ROOT SERVICE_USER SERVICE_STATE SERVICE_MISSING STOP_EXIT START_EXIT REGISTER_EXIT INIT_EXIT USER_ID
    : > "$EVENTS"
    status=0
    printf '%s\n' "$action" | sh "$root/start.sh" > "$test_root/$scenario/output" 2>&1 || status=$?
    case "$scenario" in
        register-success)
            [ "$status" -eq 0 ]
            stop_line=$(awk '/systemctl stop / { print NR; exit }' "$EVENTS")
            register_line=$(awk '/^binary -register -initialize$/ { print NR; exit }' "$EVENTS")
            start_line=$(awk '/systemctl start / { print NR; exit }' "$EVENTS")
            [ "$stop_line" -lt "$register_line" ] && [ "$register_line" -lt "$start_line" ]
            [ "$(grep -c '^binary ' "$EVENTS")" -eq 1 ]
            grep -q '重新注册完成' "$test_root/$scenario/output" ;;
        start-retry|start-failed-service)
            [ "$status" -eq 0 ]
            grep -q '^binary -initialize$' "$EVENTS"
            ! grep -q '^binary -register' "$EVENTS"
            grep -q 'systemctl start' "$EVENTS" ;;
        already-running|start-transition)
            [ "$status" -eq 0 ]
            ! grep -q '^binary ' "$EVENTS"
            grep -q 'systemctl start' "$EVENTS" ;;
        start-service-failure)
            [ "$status" -ne 0 ]
            grep -q '^binary -register -initialize$' "$EVENTS"
            ! grep -q '重新注册完成' "$test_root/$scenario/output" ;;
        *)
            [ "$status" -ne 0 ]
            ! grep -q 'systemctl start' "$EVENTS"
            ! grep -q '重新注册完成' "$test_root/$scenario/output"
            if [ "$scenario" != register-failure ] && [ "$scenario" != start-failure ]; then
                ! grep -q '^binary ' "$EVENTS"
            fi
            case "$scenario" in
                wrong-directory|wrong-account|uninstalled|non-root|control-plane|start-wrong-directory|start-wrong-account) ! grep -q 'systemctl stop' "$EVENTS" ;;
            esac ;;
    esac
    [ "$(cat "$root/control-plane-ca.pem")" = original-ca ]
    [ "$(cat "$root/data/identity")" = original-identity ]
    if [ "$scenario" != control-plane ]; then [ "$(cat "$root/agent-config.json")" = original-config ]; fi
    printf 'PASS: %s\n' "$scenario"
done

# 审计首次授予只走原控制面专用账户，所有终端和授权命令均为合成夹具。
cat > "$test_root/bin/stty" <<'EOF'
#!/bin/sh
printf 'stty %s\n' "$*" >> "$EVENTS"
if [ "$1" = -g ]; then printf 'synthetic-terminal-state\n'; fi
EOF
chmod +x "$test_root/bin/stty"
for scenario in audit-success audit-failure audit-active audit-unknown audit-directory audit-account audit-uninstalled audit-non-root audit-agent audit-cancel; do
    root="$test_root/$scenario/installed"
    mkdir -p "$root/data"
    cp "$script_dir/../distribution/start.sh" "$root/start.sh"
    printf 'original-identity\n' > "$root/data/identity"
    if [ "$scenario" = audit-agent ]; then printf 'original-agent-config\n' > "$root/agent-config.json"; fi
    cat > "$root/control-plane" <<'EOF'
#!/bin/sh
printf 'audit-binary %s\n' "$*" >> "$EVENTS"
[ "$*" = -bootstrap-audit-access ] || exit 99
IFS= read -r password
IFS= read -r confirmation
[ "$password" = synthetic-audit-password ] || exit 98
[ "$confirmation" = 'GRANT_AUDIT_LOG admin' ] || exit 97
exit "$AUDIT_EXIT"
EOF
    chmod +x "$root/control-plane"
    EVENTS="$test_root/$scenario/events"
    SERVICE_ROOT="$root"
    SERVICE_USER=ob-data-orch
    SERVICE_STATE=inactive
    SERVICE_MISSING=0
    USER_ID=0
    AUDIT_EXIT=0
    confirmation='GRANT_AUDIT_LOG admin'
    case "$scenario" in
        audit-failure) AUDIT_EXIT=9 ;;
        audit-active) SERVICE_STATE=active ;;
        audit-unknown) SERVICE_STATE=unknown ;;
        audit-directory) SERVICE_ROOT="$test_root/other" ;;
        audit-account) SERVICE_USER=other-service ;;
        audit-uninstalled) SERVICE_MISSING=1 ;;
        audit-non-root) USER_ID=1000 ;;
        audit-cancel) confirmation=cancel ;;
    esac
    export EVENTS SERVICE_ROOT SERVICE_USER SERVICE_STATE SERVICE_MISSING USER_ID AUDIT_EXIT
    : > "$EVENTS"
    status=0
    printf '6\n%s\nsynthetic-audit-password\n' "$confirmation" | sh "$root/start.sh" > "$test_root/$scenario/output" 2>&1 || status=$?
    if [ "$scenario" = audit-success ] || [ "$scenario" = audit-failure ]; then
        grep -q '^audit-binary -bootstrap-audit-access$' "$EVENTS"
        grep -q 'runuser -u ob-data-orch --' "$EVENTS"
        if [ "$scenario" = audit-success ]; then [ "$status" -eq 0 ]; else [ "$status" -ne 0 ]; fi
    else
        [ "$status" -ne 0 ]
        ! grep -q '^audit-binary' "$EVENTS"
    fi
    ! grep -q 'systemctl start\|systemctl stop' "$EVENTS"
    ! grep -q synthetic-audit-password "$test_root/$scenario/output"
    [ "$(cat "$root/data/identity")" = original-identity ]
    printf 'PASS: %s\n' "$scenario"
done

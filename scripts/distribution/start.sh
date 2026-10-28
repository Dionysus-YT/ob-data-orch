#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$root"
if [ -f agent-config.json ]; then name=ob-data-orch-agent; binary=agent; else name=ob-data-orch; binary=control-plane; fi
agent_initialized=
check_agent_installation() {
    systemctl cat "$name" >/dev/null 2>&1 || { printf 'Agent 尚未安装，请选择“1 启动”完成首次注册与安装。\n'; return 1; }
    # 前台关联必须使用原目录和专用账户，不能为其他安装位置创建第二份身份。
    [ "$(systemctl show -p WorkingDirectory "$name")" = "WorkingDirectory=$root" ] &&
      [ "$(systemctl show -p User "$name")" = "User=$name" ] || { printf '已有服务的目录或账户不匹配，请使用原安装目录的启动入口。\n'; return 1; }
}
start_and_check() {
    # 停止的 Agent 先在专用账户下完成前台关联，不能让后台服务读取注册码。
    if [ "$binary" = agent ] && [ "${1:-}" != initialized ]; then
        case "$(systemctl show -p ActiveState "$name")" in
            ActiveState=inactive|ActiveState=failed)
                check_agent_installation
                if ! runuser -u "$name" -- "$root/$binary" -initialize; then
                    printf 'Agent 关联未完成，未启动服务；请检查上方错误或身份目录中的 service.log。\n'
                    return 1
                fi ;;
            ActiveState=active|ActiveState=activating|ActiveState=deactivating|ActiveState=reloading) ;;
            *) printf '无法确认 Agent 服务状态，未初始化或启动服务。\n'; return 1 ;;
        esac
    fi
    systemctl start "$name"
    systemctl is-active "$name"
    if [ "$binary" = control-plane ]; then runuser -u "$name" -- "$root/$binary" -health-check; fi
}
printf '1 启动（首次自动安装）  2 停止  3 升级  4 查看状态\n'
if [ "$binary" = agent ]; then printf '5 重新注册（关联到新节点）\n'; fi
if [ "$binary" = control-plane ]; then printf '6 首次授权 admin 审计读取（需先停止控制面）\n'; fi
read -r action
case "$action" in
  2) systemctl stop "$name"; exit ;;
  4) systemctl status "$name"; exit ;;
  6)
    [ "$binary" = control-plane ] || { printf '首次审计授权仅适用于控制面。\n'; exit 1; }
    [ "$(id -u)" = 0 ] || { printf '首次审计授权请使用 sudo 运行同一个启动入口。\n'; exit 1; }
    systemctl cat "$name" >/dev/null 2>&1 || { printf '请先完成控制面安装与首次启动。\n'; exit 1; }
    [ "$(systemctl show -p WorkingDirectory "$name")" = "WorkingDirectory=$root" ] &&
      [ "$(systemctl show -p User "$name")" = "User=$name" ] || { printf '已有服务目录或账户不匹配，请使用原安装目录。\n'; exit 1; }
    [ "$(systemctl show -p ActiveState "$name")" = ActiveState=inactive ] || { printf '请先选择“2 停止”并确认控制面停止。\n'; exit 1; }
    printf '仅为固定 admin 授予 CAP_AUDIT_LOG 审计读取，不增加业务对象权限。入口只可使用一次，撤权后不能重授。\n'
    printf '确认请输入 GRANT_AUDIT_LOG admin，其他输入取消：'; read -r confirmation
    [ "$confirmation" = 'GRANT_AUDIT_LOG admin' ] || { printf '已取消首次审计授权。\n'; exit 1; }
    printf '现有 admin 密码（不会重置密码）：'
    terminal_state=$(stty -g)
    trap 'stty "$terminal_state"; unset password' EXIT HUP INT TERM
    stty -echo
    read -r password
    stty "$terminal_state"
    trap - EXIT HUP INT TERM
    printf '\n'
    status=0
    printf '%s\n%s\n' "$password" "$confirmation" | runuser -u "$name" -- "$root/$binary" -bootstrap-audit-access || status=$?
    unset password
    exit "$status" ;;
  5)
    [ "$binary" = agent ] || { printf '重新注册仅适用于 Agent，请使用 Agent 原安装目录的启动入口。\n'; exit 1; }
    [ "$(id -u)" = 0 ] || { printf '重新注册请使用 sudo 运行同一个启动入口。\n'; exit 1; }
    check_agent_installation
    printf '请先在平台删除旧节点，再新建节点并生成未使用的注册码。此操作会停止服务并替换本机节点身份，保留安装目录、配置、信任和日志。\n'
    systemctl stop "$name"
    [ "$(systemctl show -p ActiveState "$name")" = ActiveState=inactive ] || { printf 'Agent 服务尚未停止，未替换身份；请检查服务状态后重试。\n'; exit 1; }
    if ! runuser -u "$name" -- "$root/$binary" -register -initialize; then
      printf '重新注册未完成，服务保持停止；请选择“1 启动”继续已有待关联请求。不要删除身份目录。\n'
      exit 1
    fi
    start_and_check initialized
    printf '重新注册完成，Agent 服务已启动；请在新节点页面确认心跳、完成环境检查并启用节点。\n'
    exit ;;
  3)
    printf '输入新版安装包解压目录：'; read -r source
    source=$(CDPATH= cd -- "$source" && pwd)
    [ "$source" != "$root" ] && [ -f "$source/$binary" ] || exit 1
    "$source/$binary" -version
    [ -f "$source/start.sh" ] || { printf '新版安装包缺少启动入口，未停止原服务。\n'; exit 1; }
    [ "$binary" = agent ] || { [ -f "$source/web/index.html" ] && [ -d "$source/agents" ]; }
    systemctl stop "$name"
    rm -- "$root/$binary"
    cp -- "$source/$binary" "$root/$binary"
    cmp -s "$source/$binary" "$root/$binary"
    if [ "$binary" = control-plane ]; then cp -R -- "$source/web" "$source/agents" "$root/"; fi
    # 通过原子重命名更新入口，正在解释的旧脚本文件保持完整。
    launcher_tmp=$(mktemp "$root/.start.sh.XXXXXX")
    trap 'rm -f -- "$launcher_tmp"' EXIT HUP INT TERM
    cp -- "$source/start.sh" "$launcher_tmp"
    chmod 755 "$launcher_tmp"
    cmp -s "$source/start.sh" "$launcher_tmp"
    mv -f -- "$launcher_tmp" "$root/start.sh"
    trap - EXIT HUP INT TERM
    start_and_check; exit ;;
esac
if ! systemctl cat "$name" >/dev/null 2>&1; then
    [ "$(id -u)" = 0 ] || { printf '首次安装请使用 sudo 运行同一个启动入口。\n'; exit 1; }
    # 固定专用账户与安装路径；不能通过复制 data 到新路径创建第二份身份。
    id "$name" >/dev/null 2>&1 || useradd --system --no-create-home --shell /sbin/nologin "$name"
    mkdir -p "$root/data"
    chown "$name:$name" "$root/data"
    chmod 700 "$root/data"
    chmod 755 "$root" "$root/$binary"
    if [ "$binary" = control-plane ]; then chmod -R a+rX "$root/web" "$root/agents"; fi
    if [ "$binary" = agent ]; then chmod 644 "$root/agent-config.json" "$root/control-plane-ca.pem"; fi
    if [ "$binary" = control-plane ] && [ ! -f "$root/data/settings.json" ]; then
        printf '控制面访问地址：'; read -r address
        printf '设置 admin 密码（至少 8 个字符）：'
        stty -echo
        trap 'stty echo' EXIT HUP INT TERM
        read -r password
        stty echo
        trap - EXIT HUP INT TERM
        printf '\n'
        printf '%s\n%s\n' "$address" "$password" | runuser -u "$name" -- "$root/$binary" -initialize
        unset password
    else
        runuser -u "$name" -- "$root/$binary" -initialize
        if [ "$binary" = agent ]; then agent_initialized=initialized; fi
    fi
    # systemd 的路径引号和控制字符需要拒绝，不能把安装目录解释为 unit 指令。
    case "$root" in *'"'*|*'%'*|*'\'* ) printf '安装目录含不支持的字符。\n'; exit 1;; esac
    printf '[Unit]\nDescription=OB Data Orch\nAfter=network-online.target\nWants=network-online.target\n[Service]\nType=simple\nUser=%s\nWorkingDirectory="%s"\nExecStart="%s/%s"\nRestart=on-failure\nRestartSec=15\nTimeoutStopSec=60\nUMask=0077\n[Install]\nWantedBy=multi-user.target\n' "$name" "$root" "$root" "$binary" > "/etc/systemd/system/$name.service"
    systemctl daemon-reload
    systemctl enable "$name"
fi
start_and_check "$agent_initialized"
printf '服务已启动；配置与身份保留在原安装目录。\n'

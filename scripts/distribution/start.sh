#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$root"
if [ -f agent-config.json ]; then name=ob-data-orch-agent; binary=agent; else name=ob-data-orch; binary=control-plane; fi
start_and_check() {
    systemctl start "$name"
    systemctl is-active "$name"
    if [ "$binary" = control-plane ]; then runuser -u "$name" -- "$root/$binary" -health-check; fi
}
printf '1 启动（首次自动安装）  2 停止  3 升级  4 查看状态\n'
read -r action
case "$action" in
  2) systemctl stop "$name"; exit ;;
  4) systemctl status "$name"; exit ;;
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
    fi
    # systemd 的路径引号和控制字符需要拒绝，不能把安装目录解释为 unit 指令。
    case "$root" in *'"'*|*'%'*|*'\'* ) printf '安装目录含不支持的字符。\n'; exit 1;; esac
    printf '[Unit]\nDescription=OB Data Orch\nAfter=network-online.target\nWants=network-online.target\n[Service]\nType=simple\nUser=%s\nWorkingDirectory="%s"\nExecStart="%s/%s"\nRestart=on-failure\nRestartSec=15\nTimeoutStopSec=60\nUMask=0077\n[Install]\nWantedBy=multi-user.target\n' "$name" "$root" "$root" "$binary" > "/etc/systemd/system/$name.service"
    systemctl daemon-reload
    systemctl enable "$name"
fi
start_and_check
printf '服务已启动；配置与身份保留在原安装目录。\n'

# 部署指南

> 生产环境部署和配置

---

## 1. 系统要求

### 1.1 硬件要求

| 角色 | CPU | 内存 | 磁盘 | 网络 |
|------|-----|------|------|------|
| 普通节点 | 2 核 | 4 GB | 20 GB SSD | 100 Mbps |
| Relay 节点 | 4 核 | 8 GB | 50 GB SSD | 1 Gbps |
| Bootstrap 节点 | 2 核 | 4 GB | 20 GB SSD | 100 Mbps |

### 1.2 操作系统

| 系统 | 版本 | 推荐 |
|------|------|:----:|
| Ubuntu | 22.04 LTS | ✅ |
| Debian | 12 | ✅ |
| CentOS | Stream 9 | ⚪ |

---

## 2. 安装

### 2.1 二进制安装

```bash
# 下载
curl -LO https://github.com/dep2p/dep2p/releases/download/v1.0.0/dep2p-linux-amd64

# 安装
chmod +x dep2p-linux-amd64
sudo mv dep2p-linux-amd64 /usr/local/bin/dep2p

# 验证
dep2p version
```

### 2.2 从源码构建

```bash
git clone https://github.com/dep2p/dep2p.git
cd dep2p
go build -o /usr/local/bin/dep2p ./cmd/dep2p
```

### 2.3 Docker 部署

```bash
docker pull ghcr.io/dep2p/dep2p:latest

docker run -d \
  --name dep2p \
  -p 4001:4001 \
  -v /data/dep2p:/data \
  ghcr.io/dep2p/dep2p:latest
```

---

## 3. 配置

### 3.1 配置文件

```yaml
# /etc/dep2p/config.yaml

node:
  # 监听地址
  listen_addrs:
    - "/ip4/0.0.0.0/tcp/4001"
    - "/ip4/0.0.0.0/udp/4001/quic"
  
  # 数据目录
  data_dir: "/var/lib/dep2p"

identity:
  # 密钥文件路径
  key_file: "/etc/dep2p/identity.key"

connection:
  # 最大连接数
  max_connections: 1000
  
  # 连接超时
  dial_timeout: "30s"
  
  # 空闲超时
  idle_timeout: "60s"

relay:
  # 启用 Relay
  enabled: true
  
  # 中继地址
  addresses:
    - "/ip4/relay.example.com/tcp/4001/p2p/12D3..."

logging:
  level: "info"
  format: "json"
  output: "/var/log/dep2p/dep2p.log"
```

### 3.2 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DEP2P_CONFIG` | 配置文件路径 | `/etc/dep2p/config.yaml` |
| `DEP2P_LOG_LEVEL` | 日志级别 | `info` |
| `DEP2P_DATA_DIR` | 数据目录 | `/var/lib/dep2p` |

---

## 4. Systemd 服务

### 4.1 服务文件

```ini
# /etc/systemd/system/dep2p.service

[Unit]
Description=DeP2P Node
After=network.target

[Service]
Type=simple
User=dep2p
Group=dep2p
ExecStart=/usr/local/bin/dep2p --config /etc/dep2p/config.yaml
Restart=on-failure
RestartSec=10

# 资源限制
LimitNOFILE=65535
LimitNPROC=4096

# 安全设置
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/dep2p /var/log/dep2p

[Install]
WantedBy=multi-user.target
```

### 4.2 管理命令

```bash
# 启用服务
sudo systemctl enable dep2p

# 启动
sudo systemctl start dep2p

# 状态
sudo systemctl status dep2p

# 日志
sudo journalctl -u dep2p -f
```

---

## 5. 防火墙配置

### 5.1 端口

| 端口 | 协议 | 用途 |
|------|------|------|
| 4001 | TCP | 主连接端口 |
| 4001 | UDP | QUIC 端口 |
| 8080 | TCP | HTTP API（可选） |
| 6060 | TCP | pprof（调试） |

### 5.2 UFW 配置

```bash
sudo ufw allow 4001/tcp
sudo ufw allow 4001/udp
```

### 5.3 iptables 配置

```bash
iptables -A INPUT -p tcp --dport 4001 -j ACCEPT
iptables -A INPUT -p udp --dport 4001 -j ACCEPT
```

---

## 6. 监控

### 6.1 Prometheus 指标

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'dep2p'
    static_configs:
      - targets: ['localhost:8080']
```

### 6.2 关键指标

| 指标 | 说明 | 告警阈值 |
|------|------|----------|
| `dep2p_connections_total` | 总连接数 | > 900 |
| `dep2p_bandwidth_bytes` | 带宽使用 | - |
| `dep2p_errors_total` | 错误计数 | > 0 |
| `dep2p_latency_seconds` | 延迟 | P99 > 1s |

### 6.3 健康检查

```bash
# HTTP 健康检查
curl http://localhost:8080/health

# 响应
{"status": "ok", "version": "1.0.0"}
```

---

## 7. 日志管理

### 7.1 日志轮转

```
# /etc/logrotate.d/dep2p

/var/log/dep2p/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 dep2p dep2p
    postrotate
        systemctl reload dep2p
    endscript
}
```

### 7.2 日志级别

```bash
# 动态调整日志级别
curl -X POST http://localhost:8080/admin/log-level -d 'level=debug'
```

---

## 8. 备份和恢复

### 8.1 备份

```bash
# 备份数据目录
tar -czf dep2p-backup-$(date +%Y%m%d).tar.gz /var/lib/dep2p

# 备份密钥（重要！）
cp /etc/dep2p/identity.key /backup/identity.key
```

### 8.2 恢复

```bash
# 恢复数据
tar -xzf dep2p-backup-20260111.tar.gz -C /

# 恢复密钥
cp /backup/identity.key /etc/dep2p/identity.key

# 重启服务
sudo systemctl restart dep2p
```

---

## 9. 故障排查

### 9.1 常见问题

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| 无法启动 | 端口被占用 | 检查端口，修改配置 |
| 连接失败 | 防火墙阻止 | 开放端口 |
| 性能下降 | 资源不足 | 增加资源，调整配置 |

### 9.2 诊断命令

```bash
# 检查进程
ps aux | grep dep2p

# 检查端口
netstat -tlnp | grep 4001

# 检查连接
ss -s

# 检查日志
tail -f /var/log/dep2p/dep2p.log
```

---

## 10. 安全加固

### 10.1 系统安全

```bash
# 创建专用用户
useradd -r -s /bin/false dep2p

# 设置文件权限
chown -R dep2p:dep2p /var/lib/dep2p
chmod 700 /etc/dep2p
chmod 600 /etc/dep2p/identity.key
```

### 10.2 网络安全

- 启用 TLS（默认启用）
- 限制入站连接
- 使用 Realm PSK 认证

---

## 11. 云服务器场景配置

云服务器（如 AWS EC2、阿里云 ECS）通常具有以下特点：
- 有公网 IP 但无法通过常规方式发现
- 可能没有预配置的 Bootstrap 节点
- NAT 网关可能阻止入站连接探测

### 11.1 已知节点直连

当两个云服务器需要直接通信，但没有 Bootstrap 节点时，使用 `known_peers` 配置：

```yaml
# /etc/dep2p/config.yaml

node:
  # 已知节点列表 - 直接连接这些节点，绕过 Bootstrap 发现
  known_peers:
    - peer_id: "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
      addrs:
        - "/ip4/1.2.3.4/udp/4001/quic-v1"
        - "/ip4/1.2.3.4/tcp/4001"
    - peer_id: "12D3KooWyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"
      addrs:
        - "/ip4/5.6.7.8/udp/4001/quic-v1"
```

使用场景：
- 私有集群中的节点互联
- 无公共 Bootstrap 节点的环境
- 确保特定节点始终连接

### 11.2 STUN 信任模式

在云服务器环境中，STUN 探测到的公网地址通常是可信的。启用信任模式跳过地址验证等待：

```yaml
# /etc/dep2p/config.yaml

reachability:
  # 信任 STUN 探测地址 - 跳过入站连接验证
  trust_stun_addresses: true
  
  # STUN 服务器（保持默认即可）
  stun_servers:
    - "stun:stun.l.google.com:19302"
    - "stun:stun1.l.google.com:19302"
```

**注意**：仅在以下场景启用 `trust_stun_addresses`：
- 云服务器有真实公网 IP
- 网络配置确保入站流量可达
- 无复杂 NAT 层（如 Carrier-grade NAT）

### 11.3 云服务器完整配置示例

```yaml
# /etc/dep2p/config.yaml - 云服务器生产配置

node:
  listen_addrs:
    - "/ip4/0.0.0.0/udp/4001/quic-v1"  # 优先 QUIC
    - "/ip4/0.0.0.0/tcp/4001"
  
  data_dir: "/var/lib/dep2p"
  
  # 已知节点直连
  known_peers:
    - peer_id: "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
      addrs: ["/ip4/peer1.example.com/udp/4001/quic-v1"]

identity:
  key_file: "/etc/dep2p/identity.key"

reachability:
  # 云服务器场景：信任 STUN 地址
  trust_stun_addresses: true

connection:
  max_connections: 500
  dial_timeout: "30s"
  idle_timeout: "60s"

# 断开检测配置（见下一节）
disconnect_detection:
  quic:
    keep_alive_period: "3s"
    max_idle_timeout: "6s"
  reconnect_grace_period: "15s"
  witness:
    enabled: true

logging:
  level: "info"
  format: "json"
  output: "/var/log/dep2p/dep2p.log"
```

---

## 12. 快速断开检测配置

DeP2P 采用多层次断开检测架构，确保在 6 秒内检测到节点离线。

### 12.1 QUIC 传输层配置

```yaml
# /etc/dep2p/config.yaml

disconnect_detection:
  quic:
    # Keep-Alive 探测间隔（推荐 3s）
    keep_alive_period: "3s"
    
    # 最大空闲超时（推荐 6s = 2 × keep_alive_period）
    max_idle_timeout: "6s"
```

原理：
- QUIC 每 3 秒发送 PING 帧
- 如果 6 秒内未收到任何数据，判定连接断开
- 这是最快的检测层，延迟约 3-6 秒

### 12.2 重连宽限期

```yaml
disconnect_detection:
  # 重连宽限期（默认 15s）
  # 在此期间断线的节点不触发离线事件
  reconnect_grace_period: "15s"
  
  # 断开保护期（防止重复添加刚断开的成员）
  disconnect_protection: "30s"
```

配置建议：
| 场景 | grace_period | disconnect_protection |
|------|--------------|----------------------|
| 稳定网络 | 10s | 20s |
| 移动网络 | 20s | 40s |
| 跨区域网络 | 15s（默认） | 30s（默认） |

### 12.3 见证人机制

```yaml
disconnect_detection:
  witness:
    # 启用见证人机制
    enabled: true
    
    # 见证人数量（发送给最近的 N 个节点）
    count: 3
    
    # 确认法定人数（需要 K 个见证人确认）
    quorum: 2
    
    # 见证报告超时
    timeout: "5s"
```

见证人机制用于：
- 非正常断开时（崩溃、网络中断）快速通知
- 防止单点误判（需要多个见证人确认）
- 加速离线事件传播

### 12.4 震荡检测（移动网络）

```yaml
disconnect_detection:
  flapping:
    # 启用震荡检测
    enabled: true
    
    # 检测窗口
    window: "60s"
    
    # 震荡阈值（窗口内断线次数）
    threshold: 3
    
    # 冷却时间（触发后暂停重连）
    cooldown: "120s"
```

---

## 13. 监控与告警

### 13.1 扩展 Prometheus 指标

```yaml
# prometheus.yml

scrape_configs:
  - job_name: 'dep2p'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 15s
```

### 13.2 关键指标及告警阈值

| 指标 | 说明 | 健康值 | 告警阈值 |
|------|------|--------|---------|
| `dep2p_connections_total` | 当前连接数 | 10-100 | > 90% 上限 |
| `dep2p_connections_direct_ratio` | 直连比例 | > 60% | < 30% |
| `dep2p_disconnect_detection_latency_seconds` | 断开检测延迟 | < 6s | > 15s |
| `dep2p_witness_quorum_success_ratio` | 见证确认率 | > 80% | < 50% |
| `dep2p_reconnect_success_ratio` | 重连成功率 | > 90% | < 60% |
| `dep2p_message_e2e_latency_seconds` | 消息端到端延迟 | P99 < 1s | P99 > 2s |
| `dep2p_message_loss_ratio` | 消息丢失率 | < 1% | > 5% |

### 13.3 Alertmanager 告警规则

```yaml
# alerting_rules.yml

groups:
  - name: dep2p
    rules:
      - alert: HighRelayUsage
        expr: dep2p_connections_relay_ratio > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "中继连接比例过高"
          description: "当前中继连接比例 {{ $value | humanizePercentage }}，建议检查 NAT 穿透配置"
      
      - alert: SlowDisconnectDetection
        expr: dep2p_disconnect_detection_latency_seconds > 15
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "断开检测延迟过高"
          description: "断开检测延迟 {{ $value }}s，可能影响成员状态同步"
      
      - alert: LowWitnessQuorum
        expr: dep2p_witness_quorum_success_ratio < 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "见证人确认率过低"
          description: "见证人确认率 {{ $value | humanizePercentage }}，可能影响快速离线检测"
      
      - alert: HighMessageLoss
        expr: dep2p_message_loss_ratio > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "消息丢失率过高"
          description: "消息丢失率 {{ $value | humanizePercentage }}"
```

### 13.4 日志分析监控

结合日志分析框架（参见[调试指南](../development/debugging_guide.md#9-p2p-日志分析)）进行监控：

```bash
# 定期执行日志分析
0 */6 * * * /opt/dep2p/scripts/p2p-log-analyze.sh /var/log/dep2p/dep2p.log >> /var/log/dep2p/analysis.log

# 实时监控关键错误
tail -f /var/log/dep2p/dep2p.log | grep --line-buffered "level=ERROR\|level=WARN" | \
  while read line; do
    # 发送到告警系统
    curl -X POST http://alertmanager:9093/api/v1/alerts -d "[{\"labels\":{\"alertname\":\"DEP2PLogError\"}}]"
  done
```

### 13.5 周期性健康检查

```bash
#!/bin/bash
# health-check.sh - DEP2P 健康检查脚本

# 检查服务状态
if ! systemctl is-active --quiet dep2p; then
    echo "ERROR: dep2p 服务未运行"
    exit 1
fi

# 检查 HTTP 健康端点
HEALTH=$(curl -s http://localhost:8080/health)
if [ "$(echo $HEALTH | jq -r '.status')" != "ok" ]; then
    echo "ERROR: 健康检查失败: $HEALTH"
    exit 1
fi

# 检查连接数
CONNS=$(curl -s http://localhost:8080/metrics | grep dep2p_connections_total | awk '{print $2}')
if [ "$CONNS" -lt 1 ]; then
    echo "WARN: 无活跃连接"
fi

# 检查最近错误
RECENT_ERRORS=$(grep "level=ERROR" /var/log/dep2p/dep2p.log | tail -10 | wc -l)
if [ "$RECENT_ERRORS" -gt 5 ]; then
    echo "WARN: 最近有 $RECENT_ERRORS 个错误"
fi

echo "OK: dep2p 运行正常, 连接数: $CONNS"
```

---

**最后更新**：2026-01-29

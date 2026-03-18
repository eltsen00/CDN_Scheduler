# CDN Scheduler

一个基于 **加权一致性哈希** 的 CDN 调度器示例项目。  
调度器周期性采集各 ATS 节点状态，动态计算权重并重建哈希环，对客户端请求返回 `302` 重定向到目标节点。

## 功能特性

- 基于一致性哈希做请求路径到节点的映射
- 支持按节点实时负载动态调整权重（虚拟节点数量）
- 周期性健康/状态采集（并发抓取）
- 节点不可用时自动降权（权重为 0）
- `HTTP(:80)` 自动升级到 `HTTPS`
- `HTTPS(:443)` 提供 302 调度服务

## 项目结构

```text
CDN_Scheduler/
├── cmd/
│   ├── scheduler/         # 调度器入口
│   │   └── main.go
│   └── mock_ats/          # 模拟 ATS 节点服务
│       └── main.go
├── config/
│   ├── config.yaml        # 调度器配置（监听地址/端口/TLS/ATS 节点）
│   └── scheduler_config.go # viper 加载与配置结构定义
├── pkg/
│   ├── hashring/          # 一致性哈希环
│   │   └── ring.go
│   ├── model/             # ATS 节点模型与状态采集
│   │   └── node.go
│   └── monitor/           # 调度器核心逻辑（周期更新 + HTTP Handler）
│       └── dispatcher.go
└── go.mod
```

## 调度算法说明

### 1) 节点指标采集

每个节点通过 `StatsURL` 拉取以下指标：

- `proxy.process.http.current_client_connections`
- `proxy.process.http.total_transactions_time`
- `proxy.process.http.incoming_requests`

并计算：

- 利用率：`ρ = current_client_connections / max_conns`
- 窗口平均响应时间：`T = Δtotal_transactions_time / Δincoming_requests`

> 代码里 `total_transactions_time` 按秒上报，内部会转换为毫秒用于日志和计算。

### 2) 服务时间基准 `S`

周期性校准：

- `S = T × (1 - ρ)`

该值用于估算不同利用率下的响应时间。

### 3) 权重计算

- 期望响应时间：`expected = S / (1 - ρ)`
- 权重：`weight = int((1 / expected) * 1000)`
- 权重边界：`[1, 1000]`
- 节点不可用或无效时权重为 `0`

权重越大，分配到的一致性哈希虚拟节点越多。

### 4) 请求调度

- 根据 `URL Path` 计算哈希值
- 在有序哈希环上二分查找目标虚拟节点
- 返回 `302` 到目标节点域名（保留原始 URI）

## 运行环境

- Go `1.25.6`（见 `go.mod`）
- Linux/macOS（Windows 也可运行，但端口与证书处理方式不同）

## 快速开始

### 0) 配置监听地址与 ATS 节点（Viper）

调度器通过 `viper` 读取配置，优先级如下：

1. 环境变量（带前缀 `SCHEDULER_`）
2. 配置文件（默认查找 `./config/config.yaml` 或 `./config.yaml`）
3. 代码内默认值

默认配置文件：`config/config.yaml`

```yaml
server:
  http:
    host: 0.0.0.0
    port: 80
  https:
    host: 0.0.0.0
    port: 443
    cert_file: server.crt
    key_file: server.key

ats_nodes:
  - name: ats1
    domain: 127.0.0.1:8081
    stats_url: http://127.0.0.1:8081/stats
    max_conns: 1000
  - name: ats2
    domain: 127.0.0.1:8082
    stats_url: http://127.0.0.1:8082/stats
    max_conns: 1000
  - name: ats3
    domain: 127.0.0.1:8083
    stats_url: http://127.0.0.1:8083/stats
    max_conns: 1000
```

环境变量覆盖示例（生产常用）：

```bash
export SCHEDULER_SERVER_HTTP_HOST=127.0.0.1
export SCHEDULER_SERVER_HTTP_PORT=8081
export SCHEDULER_SERVER_HTTPS_HOST=127.0.0.1
export SCHEDULER_SERVER_HTTPS_PORT=8080
export SCHEDULER_SERVER_HTTPS_CERT_FILE=/etc/letsencrypt/live/scheduler.example.com/fullchain.pem
export SCHEDULER_SERVER_HTTPS_KEY_FILE=/etc/letsencrypt/live/scheduler.example.com/privkey.pem
```

也可以指定配置文件路径：

```bash
export SCHEDULER_CONFIG=/opt/cdn-scheduler/config.yaml
```

### 1) 启动 3 个模拟 ATS 节点

在项目根目录执行（建议开 3 个终端）：

```bash
go run ./cmd/mock_ats -port 8081 -latency 20 -conns 80
go run ./cmd/mock_ats -port 8082 -latency 60 -conns 120
go run ./cmd/mock_ats -port 8083 -latency 120 -conns 160
```

每个节点会在 `/stats` 返回模拟数据。

### 2) 准备 TLS 证书

仓库已将 `server.crt` / `server.key` 加入 `.gitignore`，可本地生成自签名证书：

```bash
openssl req -x509 -newkey rsa:2048 -sha256 -days 365 -nodes \
  -keyout server.key -out server.crt \
  -subj "/CN=localhost"
```

### 3) 启动调度器

```bash
go run ./cmd/scheduler
```

> 默认监听 `0.0.0.0:80` 和 `0.0.0.0:443`。在 Linux 上绑定低位端口通常需要 root 权限；可通过配置改为高位端口。

## 本地验证

### 1) 使用 `curl` 查看重定向

```bash
curl -k -I https://localhost/test/path
```

预期响应头包含：

- `HTTP/1.1 302 Found`
- `Location: https://127.0.0.1:808X/test/path`

### 2) 查看日志

调度器会打印：

- 时间窗口更新日志
- 节点响应时间/服务时间计算日志
- 哈希环重建日志
- 请求重定向日志

## 注意事项

- ATS 节点列表已配置化（`config/config.yaml` 的 `ats_nodes`）。
- 首次采样需要累积一次窗口数据，之后权重会逐步稳定。
- 若某节点 `/stats` 不可达，会被标记为不可用并从哈希环中移除（权重为 0）。
- 若请求 Host 已是目标节点，服务会返回 `400`（避免重复重定向）。

## 可执行文件构建

```bash
go build -o build/cdn_scheduler ./cmd/scheduler
go build -o build/mock_ats ./cmd/mock_ats
```

## 生产部署最小清单

> 调度器默认监听 `:80/:443`，与常见反向代理（Nginx/Caddy）会发生端口冲突。  
> 生产环境请通过配置将调度器改为仅监听内网高位端口（如 `127.0.0.1:8080`），TLS 与公网入口交给反向代理处理。

### 1) systemd（守护进程 + 开机自启）

- [ ] 创建专用运行用户（无登录权限）
- [ ] 部署二进制到固定目录（如 `/opt/cdn-scheduler`）
- [ ] 配置 `Restart=always`、资源限制、工作目录

示例：`/etc/systemd/system/cdn-scheduler.service`

```ini
[Unit]
Description=CDN Scheduler Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cdn
Group=cdn
WorkingDirectory=/opt/cdn-scheduler
ExecStart=/opt/cdn-scheduler/cdn_scheduler
Restart=always
RestartSec=3
LimitNOFILE=65535
StandardOutput=append:/var/log/cdn-scheduler/app.log
StandardError=append:/var/log/cdn-scheduler/error.log

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin cdn
sudo mkdir -p /opt/cdn-scheduler /var/log/cdn-scheduler
sudo chown -R cdn:cdn /opt/cdn-scheduler /var/log/cdn-scheduler
sudo systemctl daemon-reload
sudo systemctl enable --now cdn-scheduler
sudo systemctl status cdn-scheduler
```

### 2) 反向代理（Nginx 终止 TLS）

- [ ] 由 Nginx 监听 `80/443`
- [ ] 反向代理到调度器内网端口（如 `127.0.0.1:8080`）
- [ ] 保留 `Host`、`X-Forwarded-*` 头

示例：`/etc/nginx/conf.d/cdn-scheduler.conf`

```nginx
server {
  listen 80;
  server_name scheduler.example.com;
  return 301 https://$host$request_uri;
}

server {
  listen 443 ssl http2;
  server_name scheduler.example.com;

  ssl_certificate /etc/letsencrypt/live/scheduler.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/scheduler.example.com/privkey.pem;

  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_http_version 1.1;
  }
}
```

校验并重载：

```bash
sudo nginx -t && sudo systemctl reload nginx
```

### 3) 日志落盘（应用日志 + 轮转）

- [ ] 将 `systemd` 输出落盘到 `/var/log/cdn-scheduler/*.log`
- [ ] 配置 `logrotate`，避免日志无限增长

示例：`/etc/logrotate.d/cdn-scheduler`

```conf
/var/log/cdn-scheduler/*.log {
  daily
  rotate 14
  compress
  delaycompress
  missingok
  notifempty
  copytruncate
}
```

### 4) 证书自动更新（Let's Encrypt）

- [ ] 安装 `certbot` 与 Nginx 插件
- [ ] 首次签发证书
- [ ] 启用自动续期并验证

```bash
sudo apt-get update
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d scheduler.example.com
sudo systemctl enable --now certbot.timer
sudo certbot renew --dry-run
```

### 5) 上线前最小验收

- [ ] `systemctl status cdn-scheduler` 为 `active (running)`
- [ ] `curl -I https://scheduler.example.com/test/path` 返回 `302`
- [ ] `Location` 指向调度目标节点域名
- [ ] `/var/log/cdn-scheduler/` 有持续写入
- [ ] `certbot renew --dry-run` 成功

## 后续可扩展方向

- 节点配置改为外部配置文件/服务发现
- 增加 Prometheus 指标与可视化监控
- 支持更多调度策略（最少连接、地理就近等）
- 引入更完善的健康检查与熔断机制

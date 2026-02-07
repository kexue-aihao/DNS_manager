# Cloudflare DNS 动态更新系统

一个使用 Go 语言开发的交互式命令行 DNS 管理系统，支持自动检测公网 IP 并更新 Cloudflare DNS 记录。

## ⚠️ 重要说明：代码缺陷

**本代码存在以下已知缺陷，使用前请注意：**

1. **多机器场景限制**：
   - 当前实现支持多机器共享同一域名，每个机器会创建独立的A记录
   - 但DNS更新逻辑可能存在竞争条件，多个机器同时更新时可能产生冲突
   - 建议：同一域名最多支持2-3台机器，超过此数量可能出现问题

2. **IP检测服务不稳定**：
   - 使用多个IP检测服务作为备用，但不同服务可能返回不同IP
   - IP确认机制（3秒延迟）可能在某些网络环境下不够准确
   - 建议：在网络稳定的环境下使用

3. **守护进程管理**：
   - 自动守护进程功能在某些系统上可能不稳定
   - PID文件管理可能存在竞态条件
   - 建议：生产环境使用 systemd 服务而非自动守护进程

4. **配置管理**：
   - 检测到已有服务时会删除配置，可能导致配置丢失
   - 多机器共享配置时可能出现冲突
   - 建议：每台机器使用独立的配置文件或不同的记录名称

5. **错误处理不完善**：
   - 某些网络错误可能没有充分的重试机制
   - API调用失败时的回退策略不够完善
   - 建议：定期检查日志，确保程序正常运行

**使用建议**：本代码适合测试和小规模使用，生产环境建议进行充分测试后再部署。

---

## 功能特性

- ✅ 自动检测公网IP变化（每5秒检测一次）
- ✅ 自动更新 Cloudflare DNS 记录
- ✅ 支持多机器共享同一域名（每个机器创建独立A记录）
- ✅ 交互式命令行界面
- ✅ 后台守护进程运行
- ✅ 完整的守护进程管理功能
- ✅ 日志记录和错误重试机制
- ✅ 配置热重载支持

## 运行环境

- **操作系统**: Debian 10/11/12/13（兼容其他 Linux 发行版）
- **Go 版本**: Go 1.21 或更高版本（仅编译时需要）
- **网络要求**: 需要能够访问 Cloudflare API (api.cloudflare.com) 和公网 IP 检测服务

## 快速开始

### 1. 安装 Go（如果未安装）

#### 在 Debian 13 上安装 Go

如果已有 `go1.25.7.linux-amd64.tar.gz` 文件在 `/root` 目录：

```bash
# 切换到 /root 目录
cd /root

# 删除旧版本并解压新版本
rm -rf /usr/local/go
tar -C /usr/local -xzf /root/go1.25.7.linux-amd64.tar.gz

# 添加环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> /root/.bashrc
echo 'export GOPATH=$HOME/go' >> /root/.bashrc
echo 'export GOROOT=/usr/local/go' >> /root/.bashrc

# 使环境变量生效
source /root/.bashrc

# 验证安装
go version
```

### 2. 编译程序

```bash
# 进入项目目录
cd go_dns_manager

# 基本编译
go build -o dns_manager

# 或静态编译（推荐，可在其他机器直接运行）
CGO_ENABLED=0 go build -ldflags="-s -w" -o dns_manager
```

**编译好的程序可以直接在其他 Debian 系统上运行，无需安装 Go 环境！**

### 3. 运行程序

```bash
# 交互式模式（首次运行会进入配置向导）
./dns_manager

# 或直接后台运行
./dns_manager --daemon
```

## 使用方法

### 运行模式

程序支持三种运行模式：

1. **交互式模式**（默认）：显示菜单，支持所有功能
   ```bash
   ./dns_manager
   ```

2. **后台运行模式**：直接开始监控，适合系统服务
   ```bash
   ./dns_manager --daemon
   ```

3. **执行一次模式**：执行一次更新后退出，适合 cron
   ```bash
   ./dns_manager --once
   ```

### 首次配置

首次运行会进入配置向导，需要提供以下信息：

1. **Cloudflare API Token**
   - 访问 https://dash.cloudflare.com/profile/api-tokens
   - 点击 "Create Token"
   - 使用 "Edit zone DNS" 模板，或自定义权限：
     - Zone - DNS - Edit
     - Zone - Zone - Read
   - 选择要管理的域名
   - 复制生成的 Token

2. **Zone ID**
   - 在 Cloudflare 控制台选择你的域名
   - 在右侧边栏找到 "Zone ID"
   - 复制 Zone ID

3. **DNS 记录名称**
   - 例如：`subdomain.example.com` 或 `@`（表示根域名）
   - **多机器场景**：所有机器使用相同的记录名称，程序会自动为每台机器创建独立的A记录

4. **记录类型**
   - 通常为 `A`（IPv4）或 `AAAA`（IPv6）
   - 默认为 `A`

### 主菜单功能

1. **开始监控** - 每5秒自动检测并更新（前台运行）
2. **检查当前公网IP** - 立即获取当前公网 IP 地址
3. **立即更新DNS记录** - 手动触发 DNS 记录更新
4. **查看DNS记录** - 列出当前域名的所有 DNS 记录
5. **配置设置** - 重新配置 API Token 等信息
6. **启动后台守护进程** - 自动后台运行（检测到已有服务会先清理）
7. **守护进程管理** - 管理正在运行的守护进程
8. **退出** - 退出程序

### 守护进程管理

#### 命令行管理

```bash
# 查看状态
./dns_manager --status

# 查看详细信息
./dns_manager --info

# 列出所有进程
./dns_manager --list

# 停止守护进程
./dns_manager --stop

# 强制终止守护进程
./dns_manager --kill

# 清理无效PID文件
./dns_manager --cleanup

# 进入管理菜单
./dns_manager --manage
```

#### 交互式管理菜单

在主菜单中选择 "7. 守护进程管理"，提供以下功能：
- 查看守护进程状态
- 查看详细信息
- 列出所有进程
- 停止守护进程
- 强制终止守护进程
- 清理无效PID文件

## 后台持久化运行

### 方法一：自动守护进程（简单，推荐测试环境）

```bash
# 直接启动后台守护进程
./dns_manager --daemon
```

程序会自动：
- 检查配置是否存在，如果不存在则进入配置向导
- 检测到已有服务时，先停止并清理所有现有守护进程和配置
- 要求重新配置（删除旧配置）
- 配置完成后自动转换为守护进程在后台运行
- 每5秒自动检测IP变化并更新DNS记录

**注意**：此方法不支持开机自启，SSH断开后继续运行。

### 方法二：systemd 服务（推荐生产环境）

#### 创建服务文件

```bash
sudo nano /etc/systemd/system/dns-manager.service
```

添加以下内容（根据实际情况修改路径）：

```ini
[Unit]
Description=Cloudflare DNS Manager - Auto Update DNS Records (每5秒检测)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/go_dns_manager
ExecStart=/root/go_dns_manager/dns_manager --daemon
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=dns-manager

[Install]
WantedBy=multi-user.target
```

#### 启用并启动服务

```bash
sudo systemctl daemon-reload
sudo systemctl enable dns-manager
sudo systemctl start dns-manager
sudo systemctl status dns-manager
```

#### 服务管理

```bash
# 启动/停止/重启
sudo systemctl start/stop/restart dns-manager

# 重新加载配置（无需重启）
sudo systemctl reload dns-manager

# 查看日志
sudo journalctl -u dns-manager -f
```

## 多机器场景说明

### 工作原理

当多台机器使用同一个域名时：
- 每台机器会创建或更新指向自己IP的A记录
- 同一个域名可以有多个A记录，每个指向不同的机器IP
- DNS查询会返回所有IP地址，客户端会轮询使用

### 使用示例

**机器1**：
```bash
./dns_manager --daemon
# 配置: example.com -> 1.2.3.4
```

**机器2**：
```bash
./dns_manager --daemon
# 配置: example.com -> 5.6.7.8
```

**结果**：
- `example.com` 有两个A记录：
  - 指向 `1.2.3.4` (机器1)
  - 指向 `5.6.7.8` (机器2)
- DNS查询会返回两个IP，实现负载均衡

### 注意事项

1. **配置冲突**：检测到已有服务时会删除配置并要求重新配置
2. **记录管理**：每台机器维护自己的A记录，互不干扰
3. **IP变化**：机器IP变化时会更新对应的A记录
4. **限制**：建议同一域名最多2-3台机器，过多可能导致DNS记录管理混乱

## 文件位置

- **配置文件**: `~/.go_dns_manager/config.json`
- **日志文件**: `~/.go_dns_manager/logs/dns_manager_YYYY-MM-DD.log`
- **PID文件**: `~/.go_dns_manager/dns_manager.pid`

## 编译选项

### 基本编译

```bash
go build -o dns_manager
```

### 静态编译（推荐，跨机器部署）

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o dns_manager
```

### 优化编译（减小文件大小）

```bash
go build -ldflags="-s -w" -o dns_manager
```

## 跨机器部署

编译好的程序可以直接在其他 Debian 系统上运行：

```bash
# 1. 在编译机器上静态编译
CGO_ENABLED=0 go build -ldflags="-s -w" -o dns_manager

# 2. 传输到目标机器
scp dns_manager user@target-machine:/path/to/destination/

# 3. 在目标机器上运行（无需安装Go）
chmod +x dns_manager
./dns_manager
```

## 故障排除

### 无法获取公网 IP
- 检查网络连接
- 确认防火墙允许访问外部 API
- 查看日志文件找出具体错误

### DNS 更新失败
- 检查 API Token 是否正确
- 确认 Zone ID 和记录名称是否正确
- 检查 API Token 权限是否足够
- 查看日志文件：`tail -f ~/.go_dns_manager/logs/dns_manager_$(date +%Y-%m-%d).log`

### 守护进程无法启动
- 检查是否有其他守护进程在运行：`./dns_manager --list`
- 清理无效的PID文件：`./dns_manager --cleanup`
- 查看日志文件找出错误原因

### 编译错误
- 确认 Go 版本 >= 1.21
- 运行 `go mod tidy` 整理依赖
- 检查代码语法：`go vet .`

## 完整命令列表

| 命令 | 功能 | 说明 |
|------|------|------|
| `./dns_manager` | 交互式模式 | 显示菜单 |
| `--daemon` | 后台运行 | 自动守护进程 |
| `--once` | 执行一次 | 适合 cron |
| `--status` | 查看状态 | 守护进程状态 |
| `--info` | 查看详细信息 | 完整信息 |
| `--list` | 列出所有进程 | 所有相关进程 |
| `--stop` | 停止守护进程 | 优雅停止 |
| `--kill` | 强制终止 | 立即终止 |
| `--cleanup` | 清理PID文件 | 删除无效文件 |
| `--manage` | 管理菜单 | 交互式管理 |

## 技术细节

### 检测频率
- **检测间隔**: 每5秒检测一次公网IP
- **IP确认机制**: 检测到变化后等待3秒再次确认，避免误判
- **更新策略**: 只有确认IP真的变化后才更新DNS记录

### 多机器支持
- 每个机器查找或创建指向自己IP的A记录
- 如果存在指向旧IP的记录，会更新它
- 如果不存在，会创建新记录
- 支持同一域名多个A记录

### 日志系统
- 自动日志文件（daemon模式）
- 日志位置：`~/.go_dns_manager/logs/dns_manager_YYYY-MM-DD.log`
- 按日期自动轮转
- 同时输出到控制台和文件

### 错误处理
- IP获取失败时自动重试3次
- DNS更新失败时自动重试3次
- 智能延迟和错误隔离

## 安全建议

1. **配置文件权限**：配置文件权限已设置为 600（仅所有者可读）
2. **API Token安全**：不要将API Token提交到版本控制系统
3. **运行用户**：如果可能，使用普通用户而非root运行
4. **定期检查**：定期检查日志，确保程序正常运行

## 已知问题和限制

1. **多机器场景**：建议最多2-3台机器共享同一域名
2. **IP检测**：依赖外部IP检测服务，可能不稳定
3. **守护进程**：自动守护进程功能在某些系统上可能不稳定，建议使用systemd
4. **配置管理**：检测到已有服务时会删除配置，需重新配置
5. **错误处理**：某些边缘情况可能没有充分处理

## 许可证

本项目仅供学习和测试使用。

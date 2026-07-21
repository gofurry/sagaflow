# 部署

SagaFlow 个人版只需要一个二进制和一个可写数据目录。模型服务可以在同机或网络中独立运行。

## Linux + systemd

构建并放置二进制：

```bash
make build
sudo install -m 0755 bin/sagaflow /usr/local/bin/sagaflow
sudo useradd --system --home /var/lib/sagaflow --create-home --shell /usr/sbin/nologin sagaflow 2>/dev/null || true
sudo -u sagaflow env SAGAFLOW_DATA_DIR=/var/lib/sagaflow \
  /usr/local/bin/sagaflow account init \
  --username admin --display-name Creator --password 'change-this-password'
```

生成一个最小运行配置，然后安装服务：

```bash
sudo -u sagaflow env SAGAFLOW_DATA_DIR=/var/lib/sagaflow \
  /usr/local/bin/sagaflow config init --output /var/lib/sagaflow/config.yaml
sudo sed -i 's/host: 127.0.0.1/host: 0.0.0.0/' /var/lib/sagaflow/config.yaml
sudo /usr/local/bin/sagaflow service install \
  --user sagaflow --config /var/lib/sagaflow/config.yaml
```

检查状态：

```bash
systemctl status sagaflow
journalctl -u sagaflow -f
curl http://127.0.0.1:8080/health
```

卸载服务不会删除数据：

```bash
sudo /usr/local/bin/sagaflow service uninstall
```

如果通过 Nginx/Caddy 暴露公网，TLS 和反向代理由该软件负责，SagaFlow 仍只需要一个 HTTP 监听端口。

## Docker Compose

```bash
docker compose build
docker compose run --rm sagaflow account init --password 'change-this-password'
docker compose up -d
docker compose logs -f
```

数据位于 `sagaflow_data` 卷。升级时先执行 `sagaflow backup create`，再重建镜像；SQLite 迁移在新版本启动时自动执行。

## Windows 与 macOS

下载或构建对应平台二进制，将其放入任意目录。首次运行：

```powershell
.\sagaflow.exe account init --password "change-this-password"
.\sagaflow.exe serve
```

macOS/Linux 终端去掉 `.exe`。命令行窗口关闭后服务停止；如需长期运行，Linux 使用 systemd，NAS 可以使用 Docker。

## 运行配置

所有字段均可省略，默认配置可直接运行。配置只允许：

- `app.data_dir`、日志级别；
- `server.host`、`server.port`；
- 会话有效期与 Cookie 安全标记；
- 任务并发、轮询间隔与租约。

模型 URL、模型参数、API Key、ComfyUI 工作流、S3 Endpoint 和 S3 密钥都在工作台内部保存。旧版含 `postgres`、`redis`、`storage`、`providers` 等字段的配置会被明确拒绝，不会悄悄忽略。

## 备份与恢复

在线创建一致性备份：

```bash
sagaflow backup create --output /safe/path/sagaflow.zip
```

恢复必须停服：

```bash
sudo systemctl stop sagaflow
sagaflow backup restore --archive /safe/path/sagaflow.zip --yes
sudo systemctl start sagaflow
```

归档包含全部本地素材和凭证主密钥，不应上传到公开位置。恢复保留原数据目录用于回滚；确认成功后再自行删除该目录。

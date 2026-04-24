# kuaizu-server

快组校园小程序服务 —— 校园组队与人才匹配平台。

## 技术栈

- **Go 1.24** + **Echo v4**（HTTP 框架）
- **MySQL** + **sqlx**（数据库）
- **OpenAPI 3.0** + **oapi-codegen**（API 规范与代码生成）
- **JWT**（认证）/ **微信小程序登录** / **微信支付 v3**
- **go-mail**（SMTP 邮件服务）

## 项目结构

```
cmd/
  ├── server/      小程序服务入口
  └── admin/       管理员服务入口
api/               OpenAPI 规范及生成代码
internal/
  ├── admin/       管理员服务
  |     ├── handler/ HTTP 请求处理
  |     └── ...      与上层相似的结构
  ├── handler/     HTTP 请求处理
  ├── service/     服务层
  ├── repository/  数据库访问层
  ├── models/      领域模型
  ├── middleware/  JWT 认证中间件
  ├── auth/        JWT 签发与验证
  ├── wechat/      微信登录与支付
  ├── email/       邮件服务与模板
  └── db/          数据库连接管理
sql/               建表语句、种子数据、ER 图
```

## 快速开始

### 环境要求

- Go >= 1.24
- MySQL

### 1. 克隆仓库

```bash
git clone https://github.com/kuaizu-team/kuaizu-service.git
cd kuaizu-server
```

### 2. 配置环境变量

```bash
cp .env.example .env
```

编辑 `.env`，填写以下必要配置：

| 变量 | 说明 |
|------|------|
| `DATABASE_URL` | MySQL 连接串，格式：`user:pass@tcp(host:3306)/dbname?parseTime=true` |
| `WECHAT_APPID` / `WECHAT_SECRET` | 微信小程序凭证 |
| `WECHAT_MCH_*` | 微信支付商户配置 |
| `SMTP_*` | SMTP 邮件服务配置 |

### 3. 初始化数据库

```bash
mysql -u root -p < sql/create_mysql.sql
```

### 4. 安装依赖并运行

```bash
make tidy      # 整理依赖
make generate  # 生成代码
make run       # 启动开发服务器
```

### 5. 使用 Docker 运行

项目已经实现了容器化，可以使用 Docker Compose 快速部署服务。由于使用远程数据库，请确保已正确配置环境变量：

```bash
# 1. 复制 Docker 专用环境变量文件
cp .env.docker .env.docker.local

# 2. 编辑 .env.docker.local，填写远程数据库信息和其他必要配置
# DATABASE_URL=username:password@tcp(remote_host:3306)/dbname?parseTime=true

# 3. 启动服务 (可能需要 sudo)
docker-compose up --build -d
```

> [!TIP]
> 如果遇到权限错误 (permission denied)，请在命令前加上 `sudo`。如果 `docker-compose` 命令不存在，请尝试使用 `docker compose`。

启动后：
- 小程序 API 服务：`http://localhost:8080/api/v2`
- 管理员后台 API：`http://localhost:8081/admin`

## 常用命令

| 命令 | 说明 |
|------|------|
| `make run` | 启动开发服务器 |
| `make build` | 编译到 `bin/` 目录 |
| `make generate` | 根据 OpenAPI 规范重新生成服务端代码|
| `make tidy` | 整理 Go 模块依赖 |

## 开发流程

1. 在 `api/service/openapi.yaml` 中定义或修改 API 接口
2. 运行 `make generate` 生成服务端代码（`api/api.gen.go`）
3. 在 `internal/handler/` 中实现对应的请求处理
4. 在 `internal/service` 实现业务逻辑
5. 在 `internal/repository/` 中实现数据库操作
6. 在 `internal/models` 定义数据模型

## API 文档

API 基础路径：`/api/v2`

详细接口定义见 `api/service/openapi.yaml`。

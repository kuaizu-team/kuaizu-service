# 生产 Go 安全补丁：执行记录与发布步骤

## 已核实的生产拓扑

2026-10-22 根据服务器终端输出确认：

- Compose 项目为 `kuaizu-server`，目录 `/root/kuaizu-server`。
- `kuaizu-api` 和 `kuaizu-console` 使用同一个镜像，当前源码提交为 `b6e4785974377180e216c420c80ef442a4e49472`。
- 运行镜像名为 `kuaizu-server:codex-feishu-user-sync-backend`；镜像名不是源码版本的充分证据，以启动提交与实际产物检查为准。
- 宝塔 Nginx 与后端在同一服务器。主站转发到 `127.0.0.1:8080`，管理站 API 转发到 `127.0.0.1:8081/admin/`，管理站页面转发到 `127.0.0.1:3000`。
- 另一个站点仍通过服务器公网地址转发至 8080，因此本补丁保留现有端口绑定。收紧端口前先修正并验证这条代理。

## 本补丁的范围

分支 `codex/go-security-production-20261022` 以实际生产提交 b6e4785 为基础。不要用包含尚未上线功能的 main 直接代替该分支发布。

- 最低 Go 版本与镜像 / Actions 构建版本统一为 1.26.10。
- x/net 升至 v0.60.0，必要的关联 x/* 模块随之升级。
- 固定 Redocly CLI 为 2.67.1，确保本地和镜像生成 API 使用同一版本。
- 排除环境文件、私钥和本地二进制进入 Docker 构建上下文。
- 共用 verify workflow 完成生成、模块校验、完整单元测试、vet、Linux 源码扫描、Docker 构建及镜像内两个最终二进制扫描。
- 发布与部署均等待验证成功；部署直接下载同一次验证生成的镜像，不再次构建。
- 核对传输包哈希、镜像 ID、提交和运行中两个文件的哈希。
- 部署前备份当前配置、环境文件及旧镜像 tag；停止自动 prune 旧镜像。

本补丁不包含业务代码、数据库迁移、JWT 密钥轮换、宝塔站点配置或端口变更。

## 本地验证

在 Go 1.26.10 下已通过：

- 固定版本 OpenAPI 生成器重新生成 API。
- `go mod verify`、`go test ./...`、`go vet ./...`。
- Linux/amd64 源码 govulncheck：0 项可达符号级报告。
- 本地交叉编译的 Linux 用户 / 管理服务二进制 govulncheck：均为 0 项符号级报告。
- actionlint v1.7.7 校验工作流。
- 镜像审计脚本模拟验证：合格文件通过，旧 Go 版本和扫描失败均阻止发布，并清理临时容器。

扫描仍报告未见调用的包级 / 模块级条目，不将其表述为整个依赖图没有任何已知漏洞。本地没有 Docker daemon，真实镜像构建与最终镜像审计必须由 GitHub Actions 完成。

## 第一步：只验证，不部署

将本修复分支推送到 GitHub，会触发 `CI & Release` 的检查作业，不会触发 tag 发布或服务器部署。检查对应分支与最新提交。

检查必须全部成功，特别是：

1. Generate API and verify modules。
2. Test and vet。
3. Scan Linux source。
4. Build image。
5. Check image audit failure handling 与 Audit final binaries。
6. Upload verified release，生成 `verified-backend` artifact。

此时不要推送 `v*` tag。若只验证阶段失败，服务器现有容器保持运行，先检查失败步骤。

## 第二步：确认后部署同一修复分支

`Deploy to Server` 手工运行增加布尔输入 `deploy`，默认 false：

- false：只执行 verify，部署作业跳过。
- true：verify 成功后执行生产部署。
- 推送 `v*` tag 保留自动部署行为，也会先经过完整 verify。

选择本修复分支及确认的提交，安排维护窗口后执行部署。不要把旧版 workflow 的手工运行误认为具有新门禁；需要确认所选 ref 的 workflow 包含 `verify` 和 `deploy` 输入。如果 GitHub 页面尚未显示新输入，先核实 workflow 所在 ref，不能为绕过界面问题改为推送 main 的 tag。

当前部署仍使用已有 Compose down/up 流程，会短暂停机。无数据库迁移。GitHub Secrets 沿用现有值，补丁不要求输出或粘贴其内容。

## 第三步：生产核验

部署成功会自动完成：

- 用户服务健康检查 200；管理员错误测试账号登录 401。
- 运行镜像 ID 与 CI 审计记录匹配。
- 运行中的两个文件 SHA-256 与 CI 审计产物匹配。
- 当前证据保存到 `/root/kuaizu-server/.release-runtime/release-metadata.json`。

再用受控账号确认微信登录、管理员登录、正常上传及主要业务功能。观察 5xx、TLS / x509 错误、容器重启和后台任务异常。

## 恢复

每次部署在 `/root/kuaizu-server/backups/<run_id>-<run_attempt>/` 保存旧 Compose、旧环境文件及镜像记录。旧镜像打上 `rollback-<run_id>-<run_attempt>-<container_name>` tag，不自动删除。

恢复前遵守现有 production-deploy.md 的 hash-only 管理员凭据与 schema 兼容要求。恢复配置与镜像的组合应保持一致；不要回滚数据库或引入历史可逆密码字段。若回到旧 Go 镜像，本次证书风险会重新出现，需要尽快前向修复。

## 后续独立处理

先将通过公网地址访问本机 8080 的站点改为经过验证的 loopback upstream，再考虑将 Compose 发布地址收紧到 loopback。另行核实安全组 / 防火墙对 MySQL 与 Nacos 端口的实际访问限制，以及宝塔主机是否运行 PHP 服务。这些检查不能从 Docker 端口列表直接推断已经公网暴露或已经遭入侵。

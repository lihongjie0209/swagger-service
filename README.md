# swagger-service

平台 OpenAPI 文档聚合服务。它提供一个 Swagger UI，在同一页面切换 identity、tenant、authorization、audit、config、notification、file、scheduler 等服务的接口文档。

## 发现方式

开发/测试环境可以在 `aggregation.static` 中配置文档地址。Kubernetes 环境使用官方 `client-go` SharedInformer，通过 API Server 的 list/watch 自动发现 Service；不会轮询 Pod，也不会在每次页面请求时访问 Kubernetes API。

被发现的 Service 需要标签和注解：

```yaml
metadata:
  labels:
    platform.swagger/enabled: "true"
  annotations:
    platform.swagger/enabled: "true"
    platform.swagger/port: http
    platform.swagger/path: /swagger/doc.json
    platform.swagger/title: identity-service
```

可选注解包括 `platform.swagger/name` 和 `platform.swagger/scheme`。端口可写命名端口或数字。默认唯一名称为 `<namespace>--<service>`，因此允许跨命名空间发现同名服务。

生产清单只授予 swagger-service 对 `services` 的 `get/list/watch` 权限，并使用 `platform.swagger/enabled=true` label selector。若只聚合单个命名空间，将 `aggregation.kubernetes.namespace` 设置为目标 namespace，并可将 ClusterRole/ClusterRoleBinding 收紧为 Role/RoleBinding。

## 缓存与故障隔离

- 首次请求按配置超时拉取上游 OpenAPI JSON，并限制最大响应大小。
- 验证响应包含 OpenAPI 3 `openapi` 或 Swagger 2 `swagger` 版本字段。
- TTL 内直接使用内存缓存；上游暂时失败时返回最后一次有效文档。
- 一个上游不可用只影响该文档，不影响服务目录和其他服务。

聚合服务无业务数据库、Redis、事件总线和业务 gRPC 依赖。`/live` 与 `/ready` 仍可供 Kubernetes 探针使用。

## 安全

生产环境统一 UI 要求 Identity JWT。`/swagger/index.html` 和本地静态 UI 资源不包含业务文档，可以公开加载；服务目录和聚合文档端点受 JWT 保护，页面会提示输入 Access Token，并在后续请求中注入 Bearer Header。

上游 `/swagger/doc.json` 只暴露在 ClusterIP 内网，并由 NetworkPolicy 限制为 swagger-service。聚合器在缓存未命中时会把当前用户的 Bearer Token 转发给上游文档端点；Identity 签发的访问令牌必须包含各目标服务 audience。JWT、PSK 或 TLS 私钥绝不能写入 Service 注解。

## 运行与测试

```bash
go run ./cmd/api -env development
open http://127.0.0.1:8080/swagger/index.html

make test-race
make test-integration
```

单元测试覆盖 Service 注解解析、缓存、大小限制和 stale fallback。`integration` build tag 的端到端测试启动真实 HTTP 服务与进程内上游，同时保留脚手架基础设施适配器的 Testcontainers 回归；不要求其他平台服务运行。

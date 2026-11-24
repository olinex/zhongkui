# 如何接入新的数据源插件

> 状态：已校对
> 责任人：@olinex
> 依据：`internal/datasource/reload.go`

实现数据源接口并注册一个新的数据源插件。

**适用角色**：研发、开源贡献者。普通使用者（安全运营/运维）只需按[教程第 7 章：第三方数据源查询](../tutorials/07-datasource-query.md)登记已有类型的数据源，无需阅读本文。

命名空间隔离：每个命名空间独立持有自己的 SQLitePool（vtab.Pools），vtable 以裸名 （{name}）注册到对应命名空间的池，rego 规则直接引用裸名；连接池 Key 仍含命名空间 维度，不同命名空间即使链接同一外部实例也不复用连接。

数据源插件体系把「外部数据源类型」与「宿主」解耦：新增一种类型只需实现 vtab.DatasourcePlugin 接口并在该类型所在文件的 init() 中自注册，宿主无需改动任何 按类型分发的 switch 或数据源清单。

## 一、认识插件契约

插件接口定义在 `internal/pkg/sqlitevtab/vtab/datasource_plugin.go`：

```
type DatasourcePlugin interface {
	Key() string
	NewConfig() any
	ConfigType() reflect.Type
	TestConn(ctx context.Context, cfg any) error
	IntrospectSchema(ctx context.Context, cfg any, file io.Reader) ([]DatasourceColumn, error)
	Register(ctx context.Context, pool *sqlitevtab.SQLitePool, store *s3.ObjectCache, namespace, name string, cfg any) (sqlitevtab.Datasource, error)
	ValidateConfig(cfg any) error
}
```

各方法职责：

- Key：类型键（如 "redis"），作为进程级注册表索引，也是 core.Datasource.Type 的取值来源；
- NewConfig / ConfigType：返回该类型配置指针与反射类型，供 default 标签填充、JSON 反序列化，以及管理端 /datasource/types 经 views.GetStructFields 反射表单 schema；
- TestConn：仅建连探活（不做注册/自省），供管理端「测试连接」；
- IntrospectSchema：建连后解析目标表结构并映射为 []DatasourceColumn{Name, Type}， 供管理端「自省表结构」；`sqlite_file` 场景经 file 参数接收上传的 .db 流；
- Register：在指定命名空间池上建连接池并构造 sqlitevtab.Datasource（真正落注册）；
- ValidateConfig：仅做配置 JSON 与类型的解码级一致性校验（不建连、不注册）。

数据源本身还须满足 internal/pkg/sqlitevtab 的 Datasource 超集契约：Open 依据下推约束 返回行游标（Rows），尽力在数据源头过滤，但结果必须是真实匹配行的超集——sqlite 侧会以 Omit=false 复查每个返回行，漏行无法被复查找回。

## 二、参考现有实现

vtab/ 下每种类型拆成 `{type}.go`（连接池层 + DatasourcePlugin 实现）与 `{type}_table.go` （数据源实现）：postgres / mysql / clickhouse / mongo / redis（string/hash/list/set/zset）/ etcd / sqlite_file。复制最接近的一种即可，例如 redis.go 末尾的 redisPlugin 与其 init()。

## 三、分步清单：新增 XX 数据源

1. 在 vtab/ 下新增 `{type}.go`，定义配置结构体，字段带 `json` 与 `binding`/`default` 标签 （与既有 RedisConfig / MySQLTableConfig 一致；密码字段加 `form:"kind=password"`）。
2. 实现该类型的数据源（实现 sqlitevtab.Datasource.Open，返回 Rows）；需要下推筛选时把 可下推列索引写回 schema.Filterable（见 `postgres_table.go` 的 newPostgresDatasource）。
3. 在 vtab/register.go 的类型键常量区新增 `DatasourceType{Type}` 常量（单一事实来源， 模型层不重复定义）。
4. 在 `{type}.go` 中实现 DatasourcePlugin（Key / NewConfig / ConfigType / TestConn / IntrospectSchema / Register / ValidateConfig），并在文件底部 init() 调用 `vtab.RegisterDatasourceType({plugin}{})`。重复 Key 会 panic（启动期快速失败）。
5. 表型数据源（需要 schema）在 Register 中要求配置携带完整 schema（由前端经 /datasource/introspect 自省或手填），后端注册阶段不再自动自省。
6. 枚举无需额外代码：vtab/enum.go 的 datasourceType 经 enum.RegisterSource 从插件注册表 实时派生，新类型会自动出现在 /datasource/types 与统一枚举接口。
7. 验证：go test ./internal/pkg/sqlitevtab/... ；启动后 POST /api/v1/datasource/types 应能看到新类型及其配置字段 schema。

## 四、注册、连接池与预检

注册入口是 vtab.Pools（pools.go）：

```
pools := vtab.NewPools(store)                     // store 为可选的 S3 对象缓存（sqlite_file 用）
			pools.Register(ctx, ns, name, typ, cfgData)       // 以裸名注册到 ns 命名空间的池
			pools.Unregister(ctx, ns, name)                   // 幂等注销（未注册也返回 nil）

	  - 每个命名空间独立持有 `*sqlitevtab.SQLitePool`，vtable 以裸名 {name} 注册，rego 规则
    直接引用裸名，无需 `{namespace}__` 前缀；
		  - 连接池 Key 含命名空间维度，不同命名空间即使连同一外部实例也不复用连接（隔离凭据）；
		  - 数据源 name 必须是合法 SQL 标识符（vtab.ValidateLocalName）；
		  - 预检（precheck.go）不落库、不注册：vtab.TestConn 建连探活、vtab.IntrospectSchema
		    解析列定义、vtab.ValidateConfigJSON 仅解码校验。三者与 Register 共用同一
		    decodeDatasourceConfig，故新增类型无需改动这三个函数。
```

启动重载与热同步：

- 启动期由 datasource.Reload(ctx, db, pools) 扫描 enabled=true 的数据源逐个注册；
- 写路径（DatasourceCreate / Update / Delete / Enable / Disable）在 DB 提交后调用 datasource.BroadcastChange 向 etcd 广播 (Namespace, Name)；
- 其他进程的 datasource.Watcher 收到事件后经 datasource.SyncOne 精准同步本地池 （记录不存在或停用则 Unregister），负载解析失败时回退全量 Reload。

## 五、管理端 datasource 视图的字段要求

模型 core.Datasource 的字段约束：

- NamespaceName：命名空间，与 Name 组成唯一索引 `uq_datasources_namespace_name`；
- Name：命名空间内唯一，注册为该命名空间池内的裸名 vtable；
- Type：类型键，取值来自 `vtab.DatasourceType*` 常量（模型层不重复定义）；
- Enabled：禁用的数据源不注册 vtable；
- Config：连接配置 JSON，经 serializer.Crypto 加密落库（BYTEA），明文经 ConfigData()；
- Description：描述。

视图层（internal/manager/views/datasource）：

- POST /api/v1/datasource/create 的 DatasourceCreateIn 必填 Name(1~128)/Type(<=16)/Config， Enabled 默认 true，Description <=256；`sqlite_file` 走 multipart/form-data 携带 .db 文件；
- /:id/update 仅允许改 Enabled/Config/Description（Name/Type 不可变）；
- /types 返回各已注册类型及其配置字段 schema（由 ConfigType 反射生成）；
- DatasourceListOut.type 经 init() 的 views.AddEnumBinding 绑定枚举类 datasourceType。

新增类型后，配置字段以结构体标签为准，前端表单会自动出现对应字段，无需改前端。

## 六、约束

- 表型数据源（`mysql` / `postgres` / `clickhouse` / `sqlite_file`）与 `mongo` 的配置必须 携带完整 `schema`，后端注册阶段不再自动自省；列定义由前端 `/datasource/introspect` 自省生成或手动填写。启动 `Reload` 与热同步 `SyncOne` 均以存库 config 重调 `Pools.Register`，故库中缺 `schema` 的存量旧行重载会失败，须以带完整 schema 的配置重建。
- 预检经 `POST /datasource/test`、`POST /datasource/introspect` 暴露，只建连探活 / 自省列 定义，不落库、不注册 vtable；成功与失败原因均放响应体返回（失败不作 HTTP 5xx）。

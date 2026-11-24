# 第 2 章：Rego 规则讲解

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/tagengine/validate.go`、`internal/tagengine/rule.go`、`internal/tagengine/builtin.go`

本章不创建规则，只把平台标签规则的写法、输入与校验讲清楚。读完后再进入第 3 章动手创建。

## 你将完成什么

- 理解平台对 Rego 规则的固定契约：包名、规则名与查询路径。
- 知道规则能看到哪些输入字段，以及加密字段如何处理。
- 能读懂一条最小规则，并知道保存前会经过哪些校验。

## 前置条件

- 已按[第 1 章](01-ingest-traffic.md)接入流量，或至少有可试运行的模型样例。
- 已登录管理端，并能打开「标签事件 → 标签规则」。

## 步骤

### 1. 记住三条硬性契约

平台规则必须同时满足：

1. 包名为 `zki`，在规则中引用为 `data.zki`。
2. 必须产出一个名为 `tags` 的规则。
3. 引擎查询路径固定为 `data.zki.tags`，结果中的字符串集合就是命中的标签。

`tags` 可以用 `contains`、整体赋值或部分集赋值实现，但同一组规则里不能同时出现互相冲突的整体赋值。

验证：在「标签规则」编辑器点「校验」，包名或 `tags` 缺失会直接给出带行列号的错误。

### 2. 认识输入文档

规则看到的 `input` 来自目标模型的输入文档。平台优先使用模型自定义的投影与脱敏逻辑，否则按 json tag 序列化为对象。

当前常用的目标是 HTTP 流量模型，字段包括 `clientIp`、`serverPort`、`method`、`url`、`httpVersion`、`reqHdrUserAgent`、`fpTcpClientSig`、`fpTcpServerSig` 等。字段的完整清单以生成参考页为准，不要在规则里凭印象拼写字段名。

加密字段不会出现在规则可见的输入文档中。规则需要明文时只能调用 `crypto.reveal("<json字段名>")`：Crypto 字段在运行期通常直接返回明文，只有拿到密文时才解密；S3Crypto 字段按对象键下载并解密，超过运行时大小上限会报错。一次求值内同一字段只解密一次。

验证：在编辑器里输入 `input.` 后查看补全列表，或使用「校验」触发的字段 lint 提示。

### 3. 认识可用内置函数

平台当前注册了自定义内置函数 `sqlite.query(sql, params)`，用于查询登记为 sqlite vtable 的第三方数据源，返回行对象数组。它只在求值上下文绑定了该命名空间的数据源池时可用；详细用法见[第 7 章](07-datasource-query.md)。

验证：在「标签规则」编辑器的函数提示中能看到 `sqlite.query`；不存在的函数会在校验阶段报错。

### 4. 读懂一条最小规则

```
package zki

import rego.v1

tags contains "http-login" if {
	input.method == "POST"
	contains(input.url, "/login")
}
```

逐行含义：

- `package zki`：满足平台契约。
- `import rego.v1`：使用 Rego v1 语法。
- `tags contains "http-login"`：命中时向结果集合加入标签 `http-login`。
- `if { ... }`：规则体全部条件成立才算命中。

再变化两个例子，理解同一个 `tags` 可以有多条规则：

```
tags contains "admin-path" if {
	contains(input.url, "/admin")
}

tags contains "long-ua" if {
	count(input.reqHdrUserAgent) > 128
}
```

验证：把其中任意一条粘贴到编辑器，点「校验」应通过；点「试运行」并选择样例后能看到命中的标签。

### 5. 知道保存前会经历哪些校验

平台对规则的校验分四层：

| 层级 | 检查内容 | 失败表现 |
| --- | --- | --- |
| 语法与契约 | Rego v1 语法、包名 `zki`、存在 `tags` 规则 | 带行列号与提示的错误 |
| 组级编译 | 把变更后的整组规则一起编译，拦截跨规则冲突 | 保存被拒绝 |
| 输入字段 lint | 规则引用 `input.{field}` 是否存在于模型输入文档 | 警告，不阻塞保存 |
| 运行期样例 | 把整组规则作用到该模型的验证样例上评估 | 写入被拦截或记告警后放行 |

运行期样例存放在模型的样例表中，按命名空间隔离；HTTP 模型预置了 `http-example` 真实样例，没有样例时平台会补一条零值文档 `default`。

验证：故意把 `input.method` 写成 `input.methd`，点「校验」应出现字段 lint 警告；把 `package zki` 改成 `package zki2` 应直接报错。

### 6. 避免常见错误

- 包名不是 `zki`，或没有 `import rego.v1`。
- 没有 `tags` 规则，或规则名拼写错误。
- 一组规则里同时整体赋值 `tags := [...]` 与使用 `tags contains ...`。
- 引用不存在的输入字段，或对 `null` 字段直接做字符串操作。
- 在规则里写入凭据、密钥或隐私数据；标签名应稳定、可复用、语义单一。

## 成功标志

你能独立解释一条规则为什么命中、会打出哪些标签，并能在编辑器里用「校验」与「试运行」快速定位语法、契约与字段问题。

## 下一步

- 继续[第 3 章：实现你的第一个标签](03-first-tag.md)，把规则保存并启用。
- 需要外部数据时跳到[第 7 章：第三方数据源查询](07-datasource-query.md)。

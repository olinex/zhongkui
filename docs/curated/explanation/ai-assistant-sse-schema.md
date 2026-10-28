# AI 助手自定义 SSE 事件协议

> 状态：已校对
> 责任人：@olinex
> 依据：`internal/ai/assistant`、`internal/ai/turns`、`internal/manager/views/ai/agent_stream.go`、`web/src/apis/ai.ts`
> 关联：`internal/ai/turns`、`internal/manager/views/ai`、`web/src/stores/agentRun.ts`、`web/src/apis/@types/ai.ts`
> 目标：以「领域事件 + 内容块」描述 AI 助手的流式输出，前端按块类型渲染。
> 非目标：本协议不改变会话、输入区、提问轨、块间跳读、消息操作等前端自有逻辑。

## 1. 设计原则

1. **前端掌控渲染**：服务端只下发事实（消息、块、交互状态、用量、引用），按钮与布局由前端按产品设计决定。
2. **一个来源**：实时流与历史回放使用同一套块模型 `AgentMessageBlockOut`；前端据块类型分派渲染组件。
3. **可续播**：每个实时事件带 `seq`，`/events?after=` 可从环形缓冲区重放；历史回放使用独立快照，不与实时游标混用。
4. **可演进**：信封带 `v`，只做加字段式演进；客户端忽略未知 `type` 与未知字段。
5. **行为完整**：审批、问卷、下一步、引用、思考、压缩、回退重答、消息操作均在协议内表达。

## 2. 传输层

SSE 形式：

```text
id: 42
data: {"v":1,"seq":42,"type":"block.delta",...}

```

- `data:` 每行一个完整 JSON 事件；`id:` 为该事件的 `seq`。
- 心跳沿用 SSE 注释行 `: ping`，不定义事件。
- 路由为 `/ai/agent/:sessionID/{chat|render|action|events|abort|interaction/reopen}`。

## 3. 事件信封

```ts
export interface AssistantEvent<T = unknown> {
  /** 协议版本；破坏性变更时递增，客户端据其判断协议版本 */
  v: 1;
  /** 实时游标：同一 runID 内单调递增，从 1 开始；历史快照内为回放序号（从 1 开始） */
  seq: number;
  /** 事件类型 */
  type: AssistantEventType;
  /** 会话 ID（URL 已有时仍下发，便于日志与多路复用） */
  sessionID: number;
  /** 实时运行 ID；历史快照不携带 */
  runID?: string;
  /** 事件所属助手消息 ID（尚未落库时缺省或为 0） */
  messageID?: number;
  /** 服务端时间（Go time.Time，JSON 序列化为 RFC3339 字符串） */
  ts: string;
  payload: T;
}
```

```ts
export type AssistantEventType =
  | "snapshot.begin"
  | "snapshot.message"
  | "snapshot.end"
  | "resume.begin"
  | "resume.reset"
  | "turn.started"
  | "turn.preempted"
  | "message.started"
  | "message.persisted"
  | "block.created"
  | "block.delta"
  | "block.updated"
  | "tool.call.updated"
  | "message.citations"
  | "message.compress"
  | "interaction.waiting"
  | "interaction.resumed"
  | "turn.completed"
  | "turn.error";
```

## 4. 内容块模型

块类型与负载直接复用现有前端类型 `AgentMessageBlockOut`（见 `web/src/apis/@types/ai.ts`），不要另造一套：

```ts
export type AgentBlockKind =
  | "reasoning"
  | "content"
  | "tool_calls"
  | "approval"
  | "questionnaire"
  | "next_steps";

export type BlockPayload =
  | { text: string } // reasoning / content
  | { calls: AgentToolCallRecord[] } // tool_calls
  | { calls: AgentApprovalCallRecord[]; callIDs?: string[] } // approval
  | {
      questions: QuestionnaireQuestion[];
      answers?: QuestionnaireAnswer[];
      awaiting: boolean;
      interruptIDs?: string[];
    } // questionnaire
  | { steps: AgentNextStep[] }; // next_steps
```

约定：

- 块在助手消息内按 `seq` 升序排列，`seq` 即真实产生顺序；事件载荷中的 `blockSeq` 与该 `seq` 是同一值，改名只为避免与信封的传输游标 `seq` 混淆。
- `tool_calls` 块内的调用按 `round` 聚组；同一轮并发调用共用一个块。
- `approval` / `questionnaire` 的 `awaiting` 决定是否可交互；`callIDs` / `interruptIDs` 用于「重新审批 / 重新回答」回退定位。
- 历史快照中的块为终态；实时块的 `reasoning` / `content` 允许增量。

## 5. 事件目录

| type | 方向 | 触发时机 | payload |
| --- | --- | --- | --- |
| `snapshot.begin` | S→C | `/render` 开始 | `{ mode: "history" }` |
| `snapshot.message` | S→C | 每条历史消息 | `{ message: AgentMessageItem }`（含 `blocks` / `citations` / `compress` / `pendingCalls`） |
| `snapshot.end` | S→C | 历史回放结束 | `{}` |
| `resume.begin` | S→C | `/events?after=` 重连 | `{ runID: string; fromSeq: number }` |
| `resume.reset` | S→C | 游标已超出缓冲区 | `{ reason: "gap" \| "expired" }` |
| `turn.started` | S→C | 新一轮 / 续跑开始 | `TurnStartedPayload` |
| `turn.preempted` | S→C | 新提问抢占旧回合 | `{ byRunID: string }` |
| `message.started` | S→C | 助手消息开始 | `{ role: "assistant"; messageID?: number }` |
| `message.persisted` | S→C | 助手消息落库后 | `{ messageID: number }` |
| `block.created` | S→C | 新建块 | `{ blockSeq: number; kind: AgentBlockKind; payload: BlockPayload }` |
| `block.delta` | S→C | 思考 / 正文增量 | `{ blockSeq: number; kind: "reasoning" \| "content"; text: string }` |
| `block.updated` | S→C | 块收敛或非工具块的原位更新 | `{ blockSeq: number; kind: AgentBlockKind; payload: BlockPayload }` |
| `tool.call.updated` | S→C | 单条工具调用状态变化 | `{ blockSeq: number; call: AgentToolCallRecord }` |
| `message.citations` | S→C | 检索结果就绪 | `{ citations: RetrievalResult[] }` |
| `message.compress` | S→C | 压缩开始 / 增量 / 完成 | `{ compress: CompressState }` |
| `interaction.waiting` | S→C | 进入待审批 / 待作答 | `{ substate: "approval" \| "answer"; interruptIDs: string[]; blockSeq?: number }` |
| `interaction.resumed` | S→C | 用户提交后继续 | `{ interruptIDs: string[] }` |
| `turn.completed` | S→C | 回合终态 | `{ phase: "done" \| "failed" \| "cancelled"; messageID?: number; usage?: Usage }` |
| `turn.error` | S→C | 请求 / 生成失败 | `{ message: string; code?: string }` |

约定：

- `turn.error` 的 `code="retrying"` 表示模型调用将自动重试，属**非终态**提示；其后仍可能收到块增量与 `turn.completed`。
- 用户主动取消只下发 `turn.completed { phase: "cancelled" }`，不再重复下发 `turn.error`。
- 审批 / 问卷挂起的回合不发送 `turn.completed`：`interaction.waiting` 即该段实时流的终点。

```ts
export interface TurnStartedPayload {
  runID: string;
  /** 是否为审批 / 问卷提交后的续跑（续跑不新建助手气泡） */
  resumed: boolean;
  /** 续跑 / 已完成回合的助手消息 ID */
  assistantMessageID?: number;
  /** 新提问时下发，用于替换前端本地回显 */
  userMessage?: {
    messageID: number;
    content: string;
    createdAt: string;
    userIndex: number;
  };
}

export interface Usage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  elapsedMs: number;
}
```

## 6. 实时流时序

新提问（`/chat`）：

```text
turn.started              # 带 userMessage，替换本地回显
message.started           # 新建助手气泡
block.created             # reasoning，text 为首个增量
block.delta xN            # 思考增量
block.updated             # reasoning 终态全文
block.created             # tool_calls，calls=[首条 start]
tool.call.updated         # 每条调用 start / done / failed 时按 callID 更新
block.created             # content，text 为首个增量
block.delta xN            # 正文增量
block.updated             # content 终态全文
message.citations         # 可选
block.created             # next_steps
turn.completed            # phase=done + usage + messageID
```

审批挂起：

```text
block.created             # approval，awaiting=true，calls 带 auditLevel/execOwner/approvalRequired
interaction.waiting       # substate=approval，interruptIDs，blockSeq
# 流在此结束；会话进入等待态，输入区由前端禁用
```

审批续跑（`/action`）：

```text
turn.started              # resumed=true，assistantMessageID
interaction.resumed       # interruptIDs
block.updated             # approval，带 decision/result/error，awaiting=false
message.started           # 不新建气泡，仅续接同一助手消息
block.created / delta ... # 后续思考、工具、正文
turn.completed
```

问卷同理：`interaction.waiting { substate: "answer" }` → `block.updated { awaiting:false, answers }` → 续跑。

压缩：

```text
message.compress          # phase=running，draft 为空
message.compress xN       # draft 增量
message.compress          # phase=done，tokensBefore/foldedCount/tokensAfter
```

## 7. 历史回放（`/render`）

历史不重放实时事件，直接给终态快照，避免前端重新播放动画：

```text
snapshot.begin
snapshot.message xN
snapshot.end
```

`snapshot.message` 的 `message` 保留现有会话消息出参字段，并补充：

- `blocks`：按 `seq` 升序的终态块（`created` 不带）。
- `citations` / `compress` / `pendingCalls`：与现有消息出参一致。
- `userMessageIndex` / `messageID`：供回退、分支、压缩定位。

前端收到 `snapshot.begin` 时清空消息列表与运行状态；`snapshot.end` 后结束加载态。

存在在途回合时，服务端可在 `snapshot.end` 之后**追加该回合已缓冲的实时事件**（同一连接、带 `id:` 游标，不具快照语义）：客户端先按快照重建历史，再按事件续接当前回合；随后用 `/events?after=<最后实时游标>` 继续续播。快照事件自身不携带 `id:`，不推进客户端的续播游标。

## 8. 断线续播（`/events?after=`）

- 实时事件进入每轮有界环形缓冲区（沿用当前 512 条容量）；缓冲索引从 1 起，与事件 `seq` 一致。
- `after` 落在缓冲区内：`resume.begin` → 重放 `seq > after` 的事件 → 继续实时。
- `after` 已过期或跨越运行：`resume.reset`，前端重新调用 `/render` 获取快照后恢复。
- 没有在途运行时返回 `204 No Content`，前端以 `/render` 的结果为准。
- `resume.begin` / `resume.reset` 是连接级控制事件：其 `seq` 取客户端最后看到的 `FromSeq`（不推进运行游标），SSE `id:` 行与之相同，因此客户端的续播游标保持不变。

## 9. 客户端动作契约

`/action` 只保留两类服务端动作：

```json
{
  "action": "approval.decide",
  "interruptIDs": ["int-1"],
  "decisions": [{ "callID": "call-1", "approved": true }]
}
```

```json
{
  "action": "questionnaire.submit",
  "interruptIDs": ["int-2"],
  "answers": [
    { "questionID": "q-1", "answered": true, "answer": "选项 A", "custom": false }
  ]
}
```

以下动作不下发服务端，由前端本地处理（保持现状）：

| 动作 | 处理方式 |
| --- | --- |
| 复制 | 剪贴板 |
| 回退 / 删除消息 | 现有 truncate 接口 |
| 分支 | 现有 branch 接口 |
| 压缩到此 | 现有 compact SSE 接口 |
| 填入下一步 | 写入输入框 |
| 发送下一步 | 走 `/chat` 新提问 |

`/action` 只接受 `approval.decide` 与 `questionnaire.submit`；下一步发送统一走 `/chat`。

`POST /session/:id/compact` 输出 `message.compress` 事件序列（`running` + `draft` 增量 + `done` / `failed`），不新建助手气泡；`reason` 恒为 `manual`。

## 10. 前端映射

| 数据 | 现有组件 |
| --- | --- |
| `reasoning` 块 + `block.delta` | `assistant-reasoning` 展开面板、流式粘底 |
| `content` 块 + `block.delta` | `AssistantMarkdown` |
| `tool_calls` 块 | `assistant-stage-group`（并行左边线、状态、耗时） |
| `approval` 块 + `interaction.waiting` | 审批面板（审核级别、自动批准、服务端执行说明、重新审批） |
| `questionnaire` 块 + `interaction.waiting` | `AskUserQuestionnaire`（步骤条、上一题 / 下一题、取消、重新回答） |
| `next_steps` 块 | 推荐下一步面板（发送 / 填入 / 推荐徽标） |
| `message.citations` | 引用列表（链接、序号、两行截断、展开更多） |
| `message.compress` | 压缩卡片（进度、摘要增量、前后占用） |
| `turn.completed` 的 `failed` / `cancelled` | 中断提示 |
| `userMessage` / `snapshot.message` | 用户 / 助手气泡、消息操作行、时间、用量 |

前端复用 `web/src/stores/agentRun.ts` 的 `upsertBlock` / `appendBlockText` / `adoptAnchorBlocks`，按 `seq` 归并块。

## 11. 关键约定

| 议题 | 约定 |
| --- | --- |
| `seq` 作用域 | 实时事件在单轮内从 1 递增；历史快照使用独立回放序号；运行结束后靠 `/render` 重建，不做跨轮持久化游标 |
| `tool_calls` 更新粒度 | 块创建时带首条调用，其后每条调用的 start / done / failed 按 `callID` 发送 `tool.call.updated` |
| 历史快照元信息 | `snapshot.message` 全量包含 `blocks` / `citations` / `compress` / `usage` / `pendingCalls`，前端无需额外回读 `/messages` |
| 动作集合 | `/action` 只接受 `approval.decide` 与 `questionnaire.submit`；下一步发送与普通输入统一走 `/chat` |
| 实时缓冲 | `turns.turnStream` 的条目是领域事件，索引从 1 起并与事件 `seq` 一致；每轮保留 512 条环形缓冲 |

## 12. 实现要点

- **续跑块定位**：挂起轮次的内存投影状态（最大块序号、工具调用定位、挂起交互块）在挂起时保留，续跑时承接，因此审批 / 问卷块可原位 `block.updated`、工具结果可回填挂起前创建的 `tool_calls` 块。
- **冷续跑**：进程重启或内存状态丢失后无法承接块序号，前端以 `/render` 快照重建；`snapshot.message.blocks` 提供终态块。
- **历史快照**：`/render` 把同一用户轮次内的助手 / 工具消息合并为一条助手消息，块按真实产生顺序编号；已作答与待处理交互各自落为 `approval` / `questionnaire` / `next_steps` 块，`pendingCalls` 携带根中断 ID；在途回合的缓冲事件追加在 `snapshot.end` 之后。
- **前端解析**：`web/src/apis/ai.ts` 的 SSE 解析器逐条分发 `AssistantEvent`；实时事件写 `agentRun.noteSeq`，快照 / 控制事件不写；`agentRun` 的 `upsertBlock` / `appendBlockText` / `adoptAnchorBlocks` / `upsertToolCall` 按 `seq` 归并，重放幂等。
- **渲染接线**：抽屉消息体直接消费 `blocks`（历史快照与实时事件同源）。
- **本地收敛**：审批决策 / 问卷作答的本地覆盖态在事件重建后继续生效；冷续跑时前端把决策直接写回审批块，不依赖后端回推 `block.updated`。
- **同源元信息**：`message.citations` 以知识收集器快照下发，与持久化 / `snapshot.message.citations` 同源；历史用量取最后一个有计量的助手行（挂起 + 续跑不重复累加），终态耗时随最后一条助手行落库，刷新后 token / 耗时与 `turn.completed` 一致。

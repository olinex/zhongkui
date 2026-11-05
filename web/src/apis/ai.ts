import type { AxiosPromise } from "axios";
import { current_base_url, getJwtHeader, request } from "./base";

import { useAppStore } from "@/stores/app";
import { localManager } from "@/stores/local";
import { HttpHeader } from "@/types";

import { X_JWT_HEADER_NAME } from "./@types/base";

import type { SearchIn } from "@/components/@types/table";
import type { FlexIDs } from "./@types/admin";
import type { AssistantEventUnion } from "./@types/assistant";
import type {
  AgentChatIn,
  AgentInteractionReopenIn,
  AgentMemoryStatusIn,
  AgentMessageTruncateIn,
  AgentPromptOptimizeIn,
  AgentSessionBranchIn,
  AgentSessionCompactIn,
  AgentSessionCreateIn,
  AgentSessionRenameIn,
  AgentSessionValveIn,
  AgentSkillCreateIn,
  AgentSkillStatusIn,
  AgentSkillUpdateIn,
  AssistantActionRequest,
  DryRunStageEvent,
  EvalStage,
  KnowledgeDocumentCreateIn,
  KnowledgeDocumentUpdateIn,
  RetrievalDryRunIn,
  RetrievalDryRunOut,
} from "./@types/ai";

// ---- 知识库文档 ----
export function knowledgeDocumentListGet(data: SearchIn): AxiosPromise {
  return request.post("/ai/doc/list", data, {
    headers: getJwtHeader(),
  });
}

export function knowledgeDocumentListOptions(): AxiosPromise {
  return request.options("/ai/doc/list", {
    headers: getJwtHeader(),
  });
}

export function knowledgeDocumentCreate(data: KnowledgeDocumentCreateIn): AxiosPromise {
  return request.post("/ai/doc/create", data, {
    headers: getJwtHeader(),
  });
}

// knowledgeDocumentUpload 导入文档原件：以 multipart/form-data 提交，字段 title 可选（缺省取
// 文件名去扩展名）、file 必填。后端契约要求 file 必须为最后一个表单部分，故 FormData 构造时
// 须最后 append file（与 apis/datasource.ts 的 buildCreateForm 范式一致）；出参 data 为
// KnowledgeDocumentUploadOut（新建文档 id 与 pending 状态）。
export function knowledgeDocumentUpload(formData: FormData): AxiosPromise {
  return request.post("/ai/doc/upload", formData, {
    headers: getJwtHeader(),
  });
}

export function knowledgeDocumentDetailGet(id: number): AxiosPromise {
  return request.post(
    `/ai/doc/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function knowledgeDocumentUpdate(id: number, data: KnowledgeDocumentUpdateIn): AxiosPromise {
  return request.post(`/ai/doc/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function knowledgeDocumentDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/ai/doc/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function knowledgeDocumentReindex(id: number): AxiosPromise {
  return request.post(
    `/ai/doc/${id}/reindex`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// 知识库文档分块明细：读取该文档分块并返回索引摘要与逐块明细（读路径不要求 AI 能力已启用）
export function knowledgeDocumentChunksGet(id: number): AxiosPromise {
  return request.post(
    `/ai/doc/${id}/chunks`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// 系统命名空间知识库文档列表：系统内置命名空间的只读接口，数据范围恒为系统命名空间
// （不取自客户端命名空间），供知识库页「系统知识库（只读）」视图读取列表。
export function knowledgeSystemDocumentListGet(data: SearchIn): AxiosPromise {
  return request.post("/ai/doc/system/list", data, {
    headers: getJwtHeader(),
  });
}

// 系统命名空间知识库文档列表参数：与列表接口同源的只读接口，供「系统知识库（只读）」
// 视图预取筛选项 / 字段元信息，数据范围恒为系统命名空间。
export function knowledgeSystemDocumentListOptions(): AxiosPromise {
  return request.options("/ai/doc/system/list", {
    headers: getJwtHeader(),
  });
}

// 系统命名空间知识库文档详情：系统内置命名空间的只读接口，数据范围恒为系统命名空间，
// 供引用溯源详情页读取来源文档正文。
export function knowledgeSystemDocumentDetailGet(id: number): AxiosPromise {
  return request.post(
    `/ai/doc/system/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// 系统命名空间知识库文档分块明细：系统内置命名空间的只读接口，数据范围恒为系统命名空间，
// 供引用溯源详情页读取来源文档分块明细。
export function knowledgeSystemDocumentChunksGet(id: number): AxiosPromise {
  return request.post(
    `/ai/doc/system/${id}/chunks`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// knowledgeDocumentDownload 下载导入文档原件：响应为二进制流（不走 getResponse 的 JSON 成功
// 回调，由调用方直接处理 Blob；原件文件名在 Content-Disposition 响应头中）。仅导入文档可下载，
// 手动录入文档返回 404。
export function knowledgeDocumentDownload(id: number): AxiosPromise {
  return request.post(
    `/ai/doc/${id}/download`,
    {},
    {
      headers: {
        ...getJwtHeader(),
        Accept: "application/octet-stream",
      },
      responseType: "blob",
    },
  );
}

// 检索试运行参数默认值（来自后端 [llm].rerank 配置，供前端对话框预填展示）
export function knowledgeRetrievalOptions(): AxiosPromise {
  return request.post(
    "/ai/retrieval/options",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- AI 会话 ----
export function agentSessionListGet(): AxiosPromise {
  return request.post(
    "/ai/session/list",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// agentSessionCreate 创建会话：后端会据首个问题同步调用 LLM 生成会话主题，
// 故显式覆盖 axios 实例的全局短超时（1s），避免远端模型响应稍慢时客户端提前判失败——
// 请求被中止后 getResponse 无成功/失败回调可触发，前端既不建流也无提示（会话已在后端创建）。
export function agentSessionCreate(data: AgentSessionCreateIn): AxiosPromise {
  return request.post("/ai/session/create", data, {
    headers: getJwtHeader(),
    timeout: 60000,
  });
}

// agentPromptOptimize 优化用户提问：把当前诉求改写为更清晰、更易被正确理解的提问。
// 该接口在后端单轮调用 LLM，故显式覆盖 axios 实例的全局短超时（60s），避免模型响应稍慢被误判失败。
export function agentPromptOptimize(data: AgentPromptOptimizeIn): AxiosPromise {
  return request.post("/ai/prompt/optimize", data, {
    headers: getJwtHeader(),
    timeout: 60000,
  });
}

// agentSessionBranch 以某轮对话为结尾分支出新会话：把该轮及之前的全部消息复制到新会话，
// 出参 data 为新建的会话项，前端据其切换到新会话。
export function agentSessionBranch(id: number, data: AgentSessionBranchIn): AxiosPromise {
  return request.post(`/ai/session/${id}/branch`, data, {
    headers: getJwtHeader(),
  });
}

export function agentSessionDetailGet(id: number): AxiosPromise {
  return request.post(
    `/ai/session/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function agentSessionRename(id: number, data: AgentSessionRenameIn): AxiosPromise {
  return request.post(`/ai/session/${id}/rename`, data, {
    headers: getJwtHeader(),
  });
}

// agentSessionValveSet 更新单个会话的审核阀门：返回更新后的会话项（与 rename 同风格）
export function agentSessionValveSet(id: number, data: AgentSessionValveIn): AxiosPromise {
  return request.post(`/ai/session/${id}/valve`, data, {
    headers: getJwtHeader(),
  });
}

export function agentSessionDelete(id: number): AxiosPromise {
  return request.post(
    `/ai/session/${id}/delete`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function agentMessageListGet(id: number): AxiosPromise {
  return request.post(
    `/ai/session/${id}/messages`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// agentMessageTruncate 从某一轮用户消息起截断会话消息：删除该轮提问（按其用户在会话内的序号定位）
// 及其之后的全部消息。供「回退 / 删除到某一轮对话」使用。
export function agentMessageTruncate(id: number, data: AgentMessageTruncateIn): AxiosPromise {
  return request.post(`/ai/session/${id}/messages/truncate`, data, {
    headers: getJwtHeader(),
  });
}

// ---- 长期记忆 ----
export function agentMemoryListGet(data: SearchIn): AxiosPromise {
  return request.post("/ai/memory/list", data, {
    headers: getJwtHeader(),
  });
}

export function agentMemoryListOptions(): AxiosPromise {
  return request.options("/ai/memory/list", {
    headers: getJwtHeader(),
  });
}

export function agentMemoryDetailGet(id: number): AxiosPromise {
  return request.post(
    `/ai/memory/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function agentMemoryDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/ai/memory/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function agentMemoryStatusSet(id: number, data: AgentMemoryStatusIn): AxiosPromise {
  return request.post(`/ai/memory/${id}/status`, data, {
    headers: getJwtHeader(),
  });
}

// ---- AI 技能 ----
export function agentSkillListGet(data: SearchIn): AxiosPromise {
  return request.post("/ai/skill/list", data, {
    headers: getJwtHeader(),
  });
}

export function agentSkillListOptions(): AxiosPromise {
  return request.options("/ai/skill/list", {
    headers: getJwtHeader(),
  });
}

// agentSkillOptionsGet 拉取聊天框技能选择器选项：合并跨命名空间内置技能与当前命名空间已启用技能
export function agentSkillOptionsGet(): AxiosPromise {
  return request.post("/ai/skill/options", {}, {
    headers: getJwtHeader(),
  });
}

export function agentSkillDetailGet(id: number): AxiosPromise {
  return request.post(
    `/ai/skill/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// agentSkillCreate 创建技能：周期能力、输入字段与固定参数随请求一次落库。
export function agentSkillCreate(data: AgentSkillCreateIn): AxiosPromise {
  return request.post("/ai/skill/create", data, {
    headers: getJwtHeader(),
  });
}

// agentSkillUpdate 更新技能：全部定义字段整体覆盖。
export function agentSkillUpdate(id: number, data: AgentSkillUpdateIn): AxiosPromise {
  return request.post(`/ai/skill/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function agentSkillDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/ai/skill/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function agentSkillStatusSet(id: number, data: AgentSkillStatusIn): AxiosPromise {
  return request.post(`/ai/skill/${id}/status`, data, {
    headers: getJwtHeader(),
  });
}

// ---- AI 状态 ----
// agentStatusGet 查询 AI 助手状态；显式传入 sessionID 时请求体携带该会话 ID，
// 后端据此在出参中回带该会话的在途运行态（AgentStatusOut.liveRun，无在途运行时为 null）。
export function agentStatusGet(sessionID?: number): AxiosPromise {
  return request.post("/ai/status", sessionID ? { sessionID } : {}, {
    headers: getJwtHeader(),
  });
}

// ---- 流式响应（SSE，原生 fetch 实现）----
// 检索试运行流式处理器：stage 为阶段状态事件（start/done/failed），
// delta 为该阶段的模型增量原始输出，done 为最终完整结果（含调用元信息）。
export interface RetrievalDryRunStreamHandlers {
  onStage?: (stage: DryRunStageEvent) => void;
  onDelta?: (stage: EvalStage, delta: string) => void;
  onDone?: (out: RetrievalDryRunOut) => void;
  onError?: (msg: string) => void;
}

// 解析后的原始 SSE 事件（事件类型 + 可选序号 + 已反序列化的 JSON 负载）。
interface SSERawEvent {
  type: string;
  // id 事件序号（SSE `id:` 行），无该行时为 undefined；供续播去重与游标持久化
  id?: number;
  data: unknown;
}

// parseEventBlock 解析单个 SSE 事件块（`id:` / `event:` / `data:` 行）；缺少 type 或 data 时返回 null。
// ping 事件无 data，仅作保活，因此自然被丢弃。
function parseEventBlock(block: string): SSERawEvent | null {
  let type = "";
  let data = "";
  let id: number | undefined;
  for (const line of block.split("\n")) {
    if (line.startsWith("id:")) {
      const parsed = Number(line.slice("id:".length).trim());
      if (Number.isFinite(parsed)) {
        id = parsed;
      }
    } else if (line.startsWith("event:")) {
      type = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      data = line.slice("data:".length).trim();
    }
  }
  if (!type || !data) {
    return null;
  }
  return { type, id, data: JSON.parse(data) as unknown };
}

// responseErrorText 从开流前返回的非 SSE 错误响应中提取展示文案：优先统一错误封套的
// summary（后端已本地化），其次 details（仅字符串形态），都取不到再回落到状态码描述。
// 用于把「接口调用已超时 / 已处理 / 无权恢复」等业务提示透出给用户，而非只显示状态码。
async function responseErrorText(rsp: Response): Promise<string> {
  try {
    const body = (await rsp.json()) as { summary?: unknown; details?: unknown };
    if (typeof body.summary === "string" && body.summary) {
      return body.summary;
    }
    if (typeof body.details === "string" && body.details) {
      return body.details;
    }
  } catch {
    // 响应体非 JSON（如网关错误页），忽略并按状态码兜底
  }
  return `Request failed with status ${rsp.status}`;
}

// streamSSE 启动一个 SSE 流式请求（POST 携带 JSON 体；GET 不携带请求体，用于 ?after= 续播）并按事件
// 分发，返回中止函数。axios 不支持流式响应，故使用原生 fetch + ReadableStream；事件以空行（\n\n）
// 分隔，逐块取出解析。请求头与 axios 拦截器保持一致（语言、命名空间、JWT）。
function streamSSE(
  method: "POST" | "GET",
  path: string,
  body: unknown,
  onEvent: (event: SSERawEvent) => void,
  onError: (msg: string) => void,
  onComplete?: () => void,
): () => void {
  const controller = new AbortController();

  const run = async (): Promise<void> => {
    const store = useAppStore();
    try {
      const rsp = await fetch(`${current_base_url()}${path}`, {
        method,
        signal: controller.signal,
        headers: {
          [HttpHeader.CONTENT_TYPE]: "application/json",
          [HttpHeader.ACCEPT]: "text/event-stream",
          [HttpHeader.ACCEPT_LANGUAGE]: store.language,
          [HttpHeader.CONTENT_LANGUAGE]: store.language,
          // 命名空间可能包含非 ASCII 字符，请求头仅允许 Latin-1，故先 URL 编码
          // （与 base.ts 拦截器行为一致，后端 NamespaceMiddleware 会相应解码）。
          [HttpHeader.NAMESPACE]: encodeURIComponent(store.namespace),
          [X_JWT_HEADER_NAME]: localManager.getJwt() || "",
        },
        body: method === "POST" ? JSON.stringify(body) : undefined,
      });
      if (!rsp.ok) {
        onError(await responseErrorText(rsp));
        return;
      }
      if (!rsp.body) {
        onError("Request failed: empty response body");
        return;
      }
      const reader = rsp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        buffer += decoder.decode(value, { stream: true });
        let sep = buffer.indexOf("\n\n");
        while (sep !== -1) {
          const block = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          dispatchEventBlock(block, onEvent);
          sep = buffer.indexOf("\n\n");
        }
      }
      // 流结束后的残余缓冲（无尾随空行）也需处理。
      if (buffer.trim()) {
        dispatchEventBlock(buffer, onEvent);
      }
      // 流正常读到 EOF：通知调用方（主动中止不会走到这里）。
      onComplete?.();
    } catch (err) {
      if (controller.signal.aborted) {
        // 调用方主动中止，静默结束。
        return;
      }
      onError(err instanceof Error ? err.message : String(err));
    }
  };

  void run();
  return () => controller.abort();
}

// dispatchEventBlock 解析并分发单个事件块，解析失败（非 JSON 负载）时静默跳过。
function dispatchEventBlock(block: string, onEvent: (event: SSERawEvent) => void): void {
  let event: SSERawEvent | null = null;
  try {
    event = parseEventBlock(block);
  } catch {
    return;
  }
  if (event) {
    onEvent(event);
  }
}

// agentInteractionReopen 「重新审批 / 重新回答」：POST /ai/agent/:sessionID/interaction/reopen。把
// 会话回退到该次审批 / 问卷交互的待处理状态（删除该交互之后的回答、重置交互卡片），不重跑整轮回答；
// 调用方随后经 /render 重放历史即可再次看到批准 / 拒绝按钮或待作答问卷。
export function agentInteractionReopen(
  sessionID: number,
  data: AgentInteractionReopenIn,
): AxiosPromise {
  return request.post(`/ai/agent/${sessionID}/interaction/reopen`, data, {
    headers: getJwtHeader(),
  });
}

// ---- 领域事件流（SSE）----
// AssistantStreamHandlers 流式处理器：onEvent 为逐条领域事件；onSeq 为可选的 SSE `id:`
// 续播游标（快照 / 连接级控制事件不携带）；onError 为开流失败；onComplete 为正常 EOF。
export interface AssistantStreamHandlers {
  onEvent?: (event: AssistantEventUnion) => void;
  onSeq?: (seq: number) => void;
  onError?: (msg: string) => void;
  onComplete?: () => void;
}

// streamAssistantSSE 启动一条事件流（POST 携带 JSON 体；GET 用于 /render 与 /events），
// 按行解析 `id:` / `data:`，逐条分发领域事件；204（无在途运行）按正常 EOF 处理。
function streamAssistantSSE(
  method: "POST" | "GET",
  path: string,
  body: unknown,
  handlers: AssistantStreamHandlers,
): () => void {
  const controller = new AbortController();

  const run = async (): Promise<void> => {
    const store = useAppStore();
    try {
      const rsp = await fetch(`${current_base_url()}${path}`, {
        method,
        signal: controller.signal,
        headers: {
          [HttpHeader.CONTENT_TYPE]: "application/json",
          [HttpHeader.ACCEPT]: "text/event-stream",
          [HttpHeader.ACCEPT_LANGUAGE]: store.language,
          [HttpHeader.CONTENT_LANGUAGE]: store.language,
          [HttpHeader.NAMESPACE]: encodeURIComponent(store.namespace),
          [X_JWT_HEADER_NAME]: localManager.getJwt() || "",
        },
        body: method === "POST" ? JSON.stringify(body) : undefined,
      });
      if (rsp.status === 204) {
        handlers.onComplete?.();
        return;
      }
      if (!rsp.ok) {
        handlers.onError?.(await responseErrorText(rsp));
        return;
      }
      if (!rsp.body) {
        handlers.onError?.("Request failed: empty response body");
        return;
      }
      const reader = rsp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      let pendingSeq: number | undefined;
      const consumeLine = (line: string): void => {
        const trimmed = line.trim();
        if (
          !trimmed ||
          trimmed.startsWith(":") ||
          trimmed.startsWith("event:") ||
          trimmed.startsWith("retry:")
        ) {
          return;
        }
        if (trimmed.startsWith("id:")) {
          const parsed = Number(trimmed.slice("id:".length).trim());
          if (Number.isFinite(parsed)) {
            pendingSeq = parsed;
          }
          return;
        }
        if (!trimmed.startsWith("data:")) {
          return;
        }
        const payload = trimmed.slice("data:".length).trim();
        if (!payload || payload === "[DONE]") {
          pendingSeq = undefined;
          return;
        }
        let event: AssistantEventUnion;
        try {
          event = JSON.parse(payload) as AssistantEventUnion;
        } catch {
          pendingSeq = undefined;
          return;
        }
        if (pendingSeq !== undefined) {
          handlers.onSeq?.(pendingSeq);
          pendingSeq = undefined;
        }
        handlers.onEvent?.(event);
      };
      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        buffer += decoder.decode(value, { stream: true });
        let sep = buffer.indexOf("\n");
        while (sep !== -1) {
          consumeLine(buffer.slice(0, sep));
          buffer = buffer.slice(sep + 1);
          sep = buffer.indexOf("\n");
        }
      }
      if (buffer.trim()) {
        consumeLine(buffer);
      }
      handlers.onComplete?.();
    } catch (err) {
      if (controller.signal.aborted) {
        return;
      }
      handlers.onError?.(err instanceof Error ? err.message : String(err));
    }
  };

  void run();
  return () => controller.abort();
}

// assistantChatStream 启动一轮对话：POST /ai/agent/:sessionID/chat（事件流）。
export function assistantChatStream(
  sessionID: number,
  data: AgentChatIn,
  handlers: AssistantStreamHandlers,
): () => void {
  return streamAssistantSSE("POST", `/ai/agent/${sessionID}/chat`, data, handlers);
}

// assistantRenderStream 读取历史快照（snapshot.begin/message/end + 在途实时事件）。
export function assistantRenderStream(
  sessionID: number,
  handlers: AssistantStreamHandlers,
): () => void {
  return streamAssistantSSE("GET", `/ai/agent/${sessionID}/render`, undefined, handlers);
}

// assistantActionStream 提交审批 / 问卷动作并续跑（事件流）。
export function assistantActionStream(
  sessionID: number,
  data: AssistantActionRequest,
  handlers: AssistantStreamHandlers,
): () => void {
  return streamAssistantSSE("POST", `/ai/agent/${sessionID}/action`, data, handlers);
}

// assistantEventsStream 运行中重连：GET /ai/agent/:sessionID/events?after=<seq>（事件流）。
export function assistantEventsStream(
  sessionID: number,
  after: number,
  handlers: AssistantStreamHandlers,
): () => void {
  return streamAssistantSSE(
    "GET",
    `/ai/agent/${sessionID}/events?after=${encodeURIComponent(String(Math.max(0, Math.floor(after))))}`,
    undefined,
    handlers,
  );
}

// assistantCompactStream 手动压缩上下文：POST /ai/session/:id/compact（message.compress 事件）。
export function assistantCompactStream(
  id: number,
  data: AgentSessionCompactIn,
  handlers: AssistantStreamHandlers,
): () => void {
  return streamAssistantSSE("POST", `/ai/session/${id}/compact`, data, handlers);
}

export function agentAbort(sessionID: number): AxiosPromise {
  return request.post(
    `/ai/agent/${sessionID}/abort`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// knowledgeRetrievalDryRunStream 启动 POST /ai/retrieval/dry-run 的 SSE 流式检索试运行，
// 返回中止函数；运行期间按阶段实时回调模型原始输出与阶段状态。
export function knowledgeRetrievalDryRunStream(
  data: RetrievalDryRunIn,
  handlers: RetrievalDryRunStreamHandlers,
): () => void {
  return streamSSE(
    "POST",
    "/ai/retrieval/dry-run",
    data,
    (event) => {
      switch (event.type) {
        case "delta": {
          const payload = event.data as { stage: EvalStage; delta: string };
          handlers.onDelta?.(payload.stage, payload.delta);
          break;
        }
        case "stage":
          handlers.onStage?.(event.data as DryRunStageEvent);
          break;
        case "done":
          handlers.onDone?.(event.data as RetrievalDryRunOut);
          break;
        case "error":
          handlers.onError?.((event.data as { error: string }).error);
          break;
        default:
          break;
      }
    },
    (msg) => handlers.onError?.(msg),
  );
}

import type { AxiosPromise } from "axios";
import { current_base_url, getJwtHeader, request } from "./base";

import { useAppStore } from "@/stores/app";
import { localManager } from "@/stores/local";
import { HttpHeader } from "@/types";

import { X_JWT_HEADER_NAME } from "./@types/base";

import type { SearchIn } from "@/components/@types/table";
import type { FlexIDs } from "./@types/admin";
import type {
  AgentOutput,
  AgentSessionCreateIn,
  AgentSessionRenameIn,
  AgentStageEvent,
  AgentStreamIn,
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

export function agentSessionCreate(data: AgentSessionCreateIn): AxiosPromise {
  return request.post("/ai/session/create", data, {
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

// ---- AI 状态 ----
export function agentStatusGet(): AxiosPromise {
  return request.post(
    "/ai/status",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 流式响应（SSE，原生 fetch 实现）----
// Agent 生成流式处理器：delta 为答案增量文本，stage 为工具调用阶段状态事件
// （toolCall / toolResult 的 start/done/failed，含工具名），done 为最终完整结果。
export interface AgentStreamHandlers {
  onDelta?: (delta: string) => void;
  onStage?: (stage: AgentStageEvent) => void;
  onDone?: (out: AgentOutput) => void;
  onError?: (msg: string) => void;
}

// 检索试运行流式处理器：stage 为阶段状态事件（start/done/failed），
// delta 为该阶段的模型增量原始输出，done 为最终完整结果（含调用元信息）。
export interface RetrievalDryRunStreamHandlers {
  onStage?: (stage: DryRunStageEvent) => void;
  onDelta?: (stage: EvalStage, delta: string) => void;
  onDone?: (out: RetrievalDryRunOut) => void;
  onError?: (msg: string) => void;
}

// 解析后的原始 SSE 事件（事件类型 + 已反序列化的 JSON 负载）。
interface SSERawEvent {
  type: string;
  data: unknown;
}

// parseEventBlock 解析单个 SSE 事件块（`event:` / `data:` 行）；缺少任一字段时返回 null。
// ping 事件无 data，仅作保活，因此自然被丢弃。
function parseEventBlock(block: string): SSERawEvent | null {
  let type = "";
  let data = "";
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) {
      type = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      data = line.slice("data:".length).trim();
    }
  }
  if (!type || !data) {
    return null;
  }
  return { type, data: JSON.parse(data) as unknown };
}

// streamSSEPost 启动一个 POST 的 SSE 流式请求并按事件分发，返回中止函数。
// axios 不支持流式响应，故使用原生 fetch + ReadableStream；事件以空行（\n\n）分隔，
// 逐块取出解析。请求头与 axios 拦截器保持一致（语言、命名空间、JWT）。
function streamSSEPost(
  path: string,
  body: unknown,
  onEvent: (event: SSERawEvent) => void,
  onError: (msg: string) => void,
): () => void {
  const controller = new AbortController();

  const run = async (): Promise<void> => {
    const store = useAppStore();
    try {
      const rsp = await fetch(`${current_base_url()}${path}`, {
        method: "POST",
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
        body: JSON.stringify(body),
      });
      if (!rsp.ok) {
        onError(`Request failed with status ${rsp.status}`);
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

// agentStreamGenerate 启动 POST /ai/agent/${sessionID}/stream 的 SSE 流式生成，
// 返回中止函数；调用方（组件卸载等）可随时调用中止。
export function agentStreamGenerate(
  sessionID: number,
  data: AgentStreamIn,
  handlers: AgentStreamHandlers,
): () => void {
  return streamSSEPost(
    `/ai/agent/${sessionID}/stream`,
    data,
    (event) => {
      switch (event.type) {
        case "delta":
          handlers.onDelta?.((event.data as { delta: string }).delta);
          break;
        // 工具调用阶段事件：后端在工具调用轮仅上报阶段状态（不带增量输出），
        // 形状与检索试运行的 stage 事件一致，故沿用同一分发路径
        case "stage":
          handlers.onStage?.(event.data as AgentStageEvent);
          break;
        case "done":
          handlers.onDone?.(event.data as AgentOutput);
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

// knowledgeRetrievalDryRunStream 启动 POST /ai/retrieval/dry-run 的 SSE 流式检索试运行，
// 返回中止函数；运行期间按阶段实时回调模型原始输出与阶段状态。
export function knowledgeRetrievalDryRunStream(
  data: RetrievalDryRunIn,
  handlers: RetrievalDryRunStreamHandlers,
): () => void {
  return streamSSEPost(
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

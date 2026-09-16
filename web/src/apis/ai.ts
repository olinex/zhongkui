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
  AgentStreamIn,
  KnowledgeDocumentCreateIn,
  KnowledgeDocumentUpdateIn,
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

// ---- 流式生成（SSE，原生 fetch 实现）----
export interface AgentStreamHandlers {
  onDelta?: (delta: string) => void;
  onDone?: (out: AgentOutput) => void;
  onError?: (msg: string) => void;
}

// handleStreamEvent 解析单个 SSE 事件块（`event:` / `data:` 行），按类型分发到处理器。
// ping 事件无 data，仅作保活，直接忽略。
function handleStreamEvent(block: string, handlers: AgentStreamHandlers): void {
  let eventType = "";
  let data = "";
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) {
      eventType = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      data = line.slice("data:".length).trim();
    }
  }
  if (!eventType || !data) {
    return;
  }
  switch (eventType) {
    case "delta": {
      const payload = JSON.parse(data) as { delta: string };
      handlers.onDelta?.(payload.delta);
      break;
    }
    case "done": {
      handlers.onDone?.(JSON.parse(data) as AgentOutput);
      break;
    }
    case "error": {
      const payload = JSON.parse(data) as { error: string };
      handlers.onError?.(payload.error);
      break;
    }
    default:
      break;
  }
}

// agentStreamGenerate 启动 POST /ai/agent/${sessionID}/stream 的 SSE 流式生成，
// 返回中止函数；调用方（组件卸载等）可随时调用中止。
// axios 不支持流式响应，故此处使用原生 fetch + ReadableStream。
export function agentStreamGenerate(
  sessionID: number,
  data: AgentStreamIn,
  handlers: AgentStreamHandlers,
): () => void {
  const controller = new AbortController();

  const run = async (): Promise<void> => {
    const store = useAppStore();
    try {
      const rsp = await fetch(`${current_base_url()}/ai/agent/${sessionID}/stream`, {
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
        body: JSON.stringify(data),
      });
      if (!rsp.ok) {
        handlers.onError?.(`Generation failed with status ${rsp.status}`);
        return;
      }
      if (!rsp.body) {
        handlers.onError?.("Generation failed: empty response body");
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
        // 事件以空行（\n\n）分隔，逐块取出解析。
        let sep = buffer.indexOf("\n\n");
        while (sep !== -1) {
          const block = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          handleStreamEvent(block, handlers);
          sep = buffer.indexOf("\n\n");
        }
      }
      // 流结束后的残余缓冲（无尾随空行）也需处理。
      if (buffer.trim()) {
        handleStreamEvent(buffer, handlers);
      }
    } catch (err) {
      if (controller.signal.aborted) {
        // 调用方主动中止，静默结束。
        return;
      }
      handlers.onError?.(err instanceof Error ? err.message : String(err));
    }
  };

  void run();
  return () => controller.abort();
}

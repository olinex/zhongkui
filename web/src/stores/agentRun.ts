// stores/agentRun.ts
// AI 运行态 store（单例）：承担 SSE 订阅所有权与「运行生命周期」状态，与抽屉组件的挂载 / 可见性解耦。
// 组件关闭抽屉或卸载时仅 detach（退订），不取消服务端运行——退订本身即触发服务端退订宽限计时，
// 宽限内重连即停表续跑；宽限内始终无订阅者才由服务端自动取消。刷新后由组件读 live-run 状态并经
// /agent/:sessionID/events 续播。lastSeq 经 localManager 按会话持久化，作为续播的 after 参数。

import { defineStore } from "pinia";

import { localManager } from "@/stores/local";
import type {
  AgentBlockKind,
  AgentMessageBlockOut,
  AgentToolCallRecord,
  AgentToolCallsBlockPayload,
  ToolRequestPayload,
} from "@/apis/@types/ai";

// RunPendingCall 单条待审批调用的运行态记录（与视图层 PendingCallState 同形）
export interface RunPendingCall {
  callID: string;
  toolName: string;
  arguments: string;
  auditLevel?: string;
  approvalRequired?: boolean;
  // 执行归属：client 由前端执行、backend 由后端执行（前端仅审批）
  execOwner: "client" | "backend";
  decision?: "approved" | "rejected";
  result?: string;
  error?: string;
  submitted?: boolean;
}

// 会话交互主状态取值（与后端枚举类 agentInteractionState 对齐）：running / waiting 表示会话内仍有
// 在途活动，是「是否运行中 / 是否等待审批」的唯一权威口径。前端不声明取值空间，仅在此用于判定在途活动。
const STATE_RUNNING = "running";
const STATE_WAITING = "waiting";
const STATE_LOCKED = "locked";

// RunState 单个会话的运行态：
//   runID                   当前运行标识（新运行发起前为空）
//   lastSeq                 已消费的最大 SSE 事件序号（续播 after 用，单调不回退）
//   state / substate        会话交互主状态与子状态：唯一权威来自后端（SSE state 事件 / 快照出参），
//                           前端据此驱动 UI，不再以 streaming / awaitingApproval 拼状态
//   locked                  会话是否已锁定（有效轮次达上限）
//   userTurns / maxUserTurns 会话有效轮次与轮次上限（快照口径，供锁定提示展示）
//   buffer                  已累积的助手正文（供同页重连时无缺口续接）
//   blocks                  本消息的有序内容块：实时流 block 事件按 seq upsert 进来、思考 / 正文增量
//                           归并进「当前开放块」（openSeq），是消息体的唯一展示来源（同后端出参 blocks）
//   openSeq                 当前开放块的序号：由最近一次 reasoning / content 的创建事件确立，增量据它
//                           精确追加（0 表示未知，退回「最后一个同 kind 块」兜底）
//   pendingCalls            本轮待审批调用
//   unsubscribe             当前订阅的退订句柄（由 store 持有，组件重挂载不影响在途流）
export interface RunState {
  runID: string;
  lastSeq: number;
  state: string;
  substate: string;
  locked: boolean;
  userTurns: number;
  maxUserTurns: number;
  buffer: string;
  blocks: AgentMessageBlockOut[];
  openSeq: number;
  pendingCalls: RunPendingCall[];
  unsubscribe: (() => void) | null;
}

// emptyRun 构造空运行态；lastSeq 初值取 localManager 持久化值，供刷新后续播。
// state 初值为空串（未知），由组件按「暂停」保守兜底（放行输入），刷新后由快照恢复。
const emptyRun = (sessionID: number): RunState => ({
  runID: "",
  lastSeq: localManager.getAgentRunSeq(sessionID),
  state: "",
  substate: "",
  locked: false,
  userTurns: 0,
  maxUserTurns: 0,
  buffer: "",
  blocks: [],
  openSeq: 0,
  pendingCalls: [],
  unsubscribe: null,
});

// isActive 会话当前是否有在途活动（运行中 / 等待中）
const isActive = (state: RunState): boolean =>
  state.state === STATE_RUNNING || state.state === STATE_WAITING;

// insertBlock 按 seq 升序把块插入运行块列表（同 seq 已存在时调用方应先走覆盖分支）
const insertBlock = (state: RunState, block: AgentMessageBlockOut): void => {
  const pos = state.blocks.findIndex((b) => b.seq > block.seq);
  if (pos >= 0) {
    state.blocks.splice(pos, 0, block);
  } else {
    state.blocks.push(block);
  }
};

// releaseSubscription 释放某会话当前的订阅句柄：仅当仍有在途活动（运行中 / 等待中）时真正中止连接，
// 使服务端据订阅者退订启动退订宽限计时（无人观看的运行才被自动取消）。活动已收敛（暂停 / 已锁定，或
// settle 已清空状态）时其连接正在自然收尾，此时 abort 不再有任何服务端含义，却会在浏览器网络记录里
// 留下一条 net::ERR_ABORTED 被误读为请求失败，故只丢弃句柄、让该流自行读到 EOF。
const releaseSubscription = (state: RunState): void => {
  if (state.unsubscribe === null) {
    return;
  }
  if (isActive(state)) {
    state.unsubscribe();
  }
  state.unsubscribe = null;
};

export const useAgentRunStore = defineStore("agentRun", {
  state: () => ({
    // runs 按会话 ID 维护运行态
    runs: {} as Record<number, RunState>,
  }),
  actions: {
    // stateOf 取（必要时创建）某会话的运行态
    stateOf(sessionID: number): RunState {
      return (this.runs[sessionID] ??= emptyRun(sessionID));
    },
    // lastSeqOf 取某会话已消费的事件序号（续播的 after 参数）
    lastSeqOf(sessionID: number): number {
      return this.stateOf(sessionID).lastSeq;
    },
    // begin 开始一次全新运行：重置运行标识、游标、缓冲与待审批项，并清空持久化游标
    begin(sessionID: number): RunState {
      const state = this.stateOf(sessionID);
      // 上一轮运行的订阅句柄先释放：已收敛的旧连接只丢弃句柄、不做 abort（见 releaseSubscription），
      // 仍在途的旧连接才中止（同会话至多一条在途订阅）
      releaseSubscription(state);
      state.runID = "";
      state.lastSeq = 0;
      // 新运行发起即置为运行中，闭合「begin 到首个 state 事件到达」之间的闸门空档（与后端初态一致）；
      // 子状态留空，待后端 state 事件下发后覆盖（前端对空子状态有兜底文案）
      state.state = STATE_RUNNING;
      state.substate = "";
      state.locked = false;
      state.buffer = "";
      // 换新数组（而非就地清空）：本消息体的块列表即上一运行遗留的数组由已结束的消息继续持有，
      // 换新数组可让新运行独享一份，避免把上一条已完成的助手消息的块一并清空
      state.blocks = [];
      state.openSeq = 0;
      state.pendingCalls = [];
      localManager.setAgentRunSeq(sessionID, 0);
      return state;
    },
    // setState 应用后端下发的交互状态（SSE state 事件）：主状态 / 子状态为唯一权威；locked 恒等于
    // 主状态为 locked（与后端推导口径一致）
    setState(sessionID: number, state: string, substate: string): void {
      const s = this.stateOf(sessionID);
      s.state = state;
      s.substate = substate;
      s.locked = state === STATE_LOCKED;
    },
    // setSnapshot 应用快照出参的交互状态五项（会话项 / 在途快照共用），刷新 / 切会话后据其恢复
    setSnapshot(
      sessionID: number,
      patch: {
        state: string;
        substate: string;
        locked: boolean;
        userTurns: number;
        maxUserTurns: number;
      },
    ): void {
      const s = this.stateOf(sessionID);
      s.state = patch.state;
      s.substate = patch.substate;
      s.locked = patch.locked;
      s.userTurns = patch.userTurns;
      s.maxUserTurns = patch.maxUserTurns;
    },
    // hasActivityOf 会话当前是否有在途活动（运行中 / 等待中）：供组件续播判定等复用
    hasActivityOf(sessionID: number): boolean {
      return isActive(this.stateOf(sessionID));
    },
    // restore 用 live-run 恢复运行态（刷新 / 重开抽屉后续播）：仅重置运行标识与待审批项，
    // 保留已持久化的续播游标；交互状态五项由 setSnapshot 单独写入
    restore(
      sessionID: number,
      patch: {
        runID: string;
        pendingCalls: RunPendingCall[];
      },
    ): RunState {
      const state = this.stateOf(sessionID);
      state.runID = patch.runID;
      state.pendingCalls = patch.pendingCalls;
      return state;
    },
    // noteSeq 记录已消费的事件序号并持久化（单调不回退）
    noteSeq(sessionID: number, seq: number): void {
      const state = this.stateOf(sessionID);
      if (seq > state.lastSeq) {
        state.lastSeq = seq;
        localManager.setAgentRunSeq(sessionID, seq);
      }
    },
    // resetSeq 为一条新的传输流派生游标：每轮 /chat、/action、/render 的实时事件游标都从 1 重新
    // 计数，直接消费该流前须把续播游标归零，避免旧轮次的大序号让 /events?after= 越界。
    resetSeq(sessionID: number): void {
      const state = this.stateOf(sessionID);
      state.lastSeq = 0;
      localManager.setAgentRunSeq(sessionID, 0);
    },
    // appendBuffer 累积助手正文增量
    appendBuffer(sessionID: number, delta: string): void {
      this.stateOf(sessionID).buffer += delta;
    },
    // adoptAnchorBlocks 以给定块序列（回退重开待输入回合的锚点出参，或刷新回放时该消息已落库的块）
    // 作为本会话运行块列表：换新数组（与上一运行的遗留块解耦，避免旧块混入），并作为随后回放 / 提交 /
    // 取消续跑的块起点。调用方随即据 stateOf(...).blocks 取定新数组并绑到承载消息，使续跑的 block 事件
    // 与 delta / reasoning 增量即时落到该消息上渲染（否则会落到上一运行的遗留数组上，表现为「生成完却
    // 无正文，刷新才见」）。openSeq 一并归零：新基线里没有「当前开放块」，待回放的创建事件重新确立。
    adoptAnchorBlocks(sessionID: number, blocks: AgentMessageBlockOut[]): void {
      const state = this.stateOf(sessionID);
      state.blocks = [...blocks];
      state.openSeq = 0;
    },
    // upsertBlock 应用一条流式 block 事件：按 seq 归并，列表恒按 seq 升序。reasoning / content 块分两类
    // 事件：创建事件（created）的载荷仅带首个增量、其后同段增量经 appendBlockText 追加，故把该序号处建为 /
    // 置回空文本占位并记为「当前开放块」，交由增量精确累积；收敛 / 更新事件（非 created，含回放时对已有块
    // 的覆盖）的载荷为终态全文，整块覆盖。其余块（tool_calls / approval / questionnaire）创建即终态，按
    // 载荷覆盖即可。
    upsertBlock(sessionID: number, ev: AgentMessageBlockOut): void {
      const state = this.stateOf(sessionID);
      const idx = state.blocks.findIndex((b) => b.seq === ev.seq);
      if (ev.kind !== "reasoning" && ev.kind !== "content") {
        if (idx >= 0) {
          state.blocks[idx] = ev;
        } else {
          insertBlock(state, ev);
        }
        return;
      }
      if (!ev.created && idx >= 0) {
        state.blocks[idx] = ev;
        return;
      }
      // 创建事件携带首个增量（block.created 的 text 即首段），并入占位块；其后同段增量经
      // appendBlockText 精确追加，避免首段文本丢失。
      const first = (ev.payload as { text?: string } | undefined)?.text ?? "";
      const placeholder: AgentMessageBlockOut = {
        seq: ev.seq,
        kind: ev.kind,
        payload: { text: first },
      };
      if (idx >= 0) {
        state.blocks[idx] = placeholder;
      } else {
        insertBlock(state, placeholder);
      }
      if (ev.created) {
        state.openSeq = ev.seq;
      }
    },
    // upsertToolCall 应用一条 tool.call.updated：按块序号定位 tool_calls 块，按 callID 原位归并单条
    // 调用记录（start / done / failed 与耗时、结果），使同一次调用在消息体内始终只占一行。
    upsertToolCall(sessionID: number, blockSeq: number, call: AgentToolCallRecord): void {
      const state = this.stateOf(sessionID);
      let block = state.blocks.find((b) => b.seq === blockSeq && b.kind === "tool_calls");
      if (!block) {
        block = { seq: blockSeq, kind: "tool_calls", payload: { calls: [] } };
        insertBlock(state, block);
      }
      const payload = block.payload as AgentToolCallsBlockPayload | undefined;
      const calls = [...(payload?.calls ?? [])];
      const idx = calls.findIndex((c) => c.callID === call.callID);
      if (idx >= 0) {
        calls[idx] = { ...calls[idx], ...call };
      } else {
        calls.push(call);
      }
      block.payload = { ...payload, calls };
    },
    // appendBlockText 把一段思考 / 正文增量追加进「当前开放块」（openSeq，由该 kind 的创建事件确立）的文本
    // 载荷——从序号 0 回放时基线里可能已有后续同 kind 块，按「最后一个同 kind 块」追加会落错序号；openSeq
    // 不可用（该 kind 的创建事件未收到）时退回最后一个同 kind 块兜底。无同 kind 块时忽略。
    appendBlockText(sessionID: number, kind: AgentBlockKind, delta: string): void {
      if (delta === "") {
        return;
      }
      const state = this.stateOf(sessionID);
      let target = state.blocks.find((b) => b.seq === state.openSeq && b.kind === kind);
      if (!target) {
        for (let i = state.blocks.length - 1; i >= 0; i--) {
          if (state.blocks[i].kind === kind) {
            target = state.blocks[i];
            break;
          }
        }
      }
      if (!target) {
        return;
      }
      const payload = target.payload as { text?: string };
      payload.text = (payload.text ?? "") + delta;
    },
    // addPendingCall 记录一条待审批调用（按 callID 幂等：回放重复下发不重复入列）。交互状态不在此推断，
    // 由后端 state 事件（waiting/approval）下发达成「等待审批」口径统一
    addPendingCall(sessionID: number, req: ToolRequestPayload): void {
      const state = this.stateOf(sessionID);
      if (state.pendingCalls.some((call) => call.callID === req.callID)) {
        return;
      }
      state.pendingCalls.push({
        callID: req.callID,
        toolName: req.toolName,
        arguments: req.arguments,
        auditLevel: req.auditLevel,
        approvalRequired: req.approvalRequired,
        execOwner: req.execOwner,
        decision: req.approvalRequired === false ? "approved" : undefined,
      });
    },
    // settle 运行结束（done / error）：清空交互状态与待审批项，交由组件按「暂停」保守兜底（放行输入）
    settle(sessionID: number): void {
      const state = this.stateOf(sessionID);
      state.state = "";
      state.substate = "";
      state.pendingCalls = [];
    },
    // attach 登记当前订阅的退订句柄：替换旧句柄（旧订阅先释放），由 store 承担订阅所有权
    attach(sessionID: number, unsubscribe: () => void): void {
      const state = this.stateOf(sessionID);
      releaseSubscription(state);
      state.unsubscribe = unsubscribe;
    },
    // release 丢弃某会话当前的订阅句柄（仅当与给定句柄一致），**不中止**连接：流已自然结束
    // （EOF / 错误）时用它在收尾阶段清理句柄，避免后续 attach / detach 对已完成的请求再调用
    // abort，在浏览器网络记录里留下 net::ERR_ABORTED。
    release(sessionID: number, unsubscribe: () => void): void {
      const state = this.runs[sessionID];
      if (!state || state.unsubscribe === null) {
        return;
      }
      if (state.unsubscribe === unsubscribe) {
        state.unsubscribe = null;
      }
    },
    // detach 退订某会话的在途流（仅断连接，不取消服务端运行）。幂等：运行已收敛时只丢弃句柄、
    // 不再 abort 那条正在自然收尾的连接（见 releaseSubscription）
    detach(sessionID: number): void {
      const state = this.runs[sessionID];
      if (!state) {
        return;
      }
      releaseSubscription(state);
    },
  },
});

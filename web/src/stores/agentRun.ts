// stores/agentRun.ts
// AI 运行态 store（单例）：承担 SSE 订阅所有权与「运行生命周期」状态，与抽屉组件的挂载 / 可见性解耦。
// 组件关闭抽屉或卸载时仅 detach（退订），不取消服务端运行——退订本身即触发服务端退订宽限计时，
// 宽限内重连即停表续跑；宽限内始终无订阅者才由服务端自动取消。刷新后由组件读 live-run 状态并经
// agentStreamResume 续播。lastSeq 经 localManager 按会话持久化，作为续播的 after 参数。

import { defineStore } from "pinia";

import { localManager } from "@/stores/local";
import type { ToolRequestPayload } from "@/apis/@types/ai";

// RunPendingCall 单条待审批调用的运行态记录（与视图层 PendingCallState 同形）
export interface RunPendingCall {
  callID: string;
  toolName: string;
  arguments: string;
  auditLevel?: string;
  approvalRequired?: boolean;
  // 执行归属：client 由前端执行（默认，字段缺失时按此处理）、backend 由后端执行（前端仅审批）
  execOwner?: "client" | "backend";
  decision?: "approved" | "rejected";
  result?: string;
  error?: string;
  submitted?: boolean;
}

// RunState 单个会话的运行态：
//   runID             当前运行标识（新运行发起前为空）
//   lastSeq           已消费的最大 SSE 事件序号（续播 after 用，单调不回退）
//   streaming         是否正在流式接收
//   awaitingApproval  是否处于「等待人工审批」态
//   buffer            已累积的助手正文（供同页重连时无缺口续接）
//   pendingCalls      本轮待审批调用
//   unsubscribe       当前订阅的退订句柄（由 store 持有，组件重挂载不影响在途流）
export interface RunState {
  runID: string;
  lastSeq: number;
  streaming: boolean;
  awaitingApproval: boolean;
  buffer: string;
  pendingCalls: RunPendingCall[];
  unsubscribe: (() => void) | null;
}

// emptyRun 构造空运行态；lastSeq 初值取 localManager 持久化值，供刷新后续播。
const emptyRun = (sessionID: number): RunState => ({
  runID: "",
  lastSeq: localManager.getAgentRunSeq(sessionID),
  streaming: false,
  awaitingApproval: false,
  buffer: "",
  pendingCalls: [],
  unsubscribe: null,
});

// releaseSubscription 释放某会话当前的订阅句柄：仅当运行仍在途（流式中 / 等待审批）时真正中止连接，
// 使服务端据订阅者退订启动退订宽限计时（无人观看的运行才被自动取消）。运行已收敛（done / error 后
// settle 已把 streaming 置 false）时其连接正在自然收尾，此时 abort 不再有任何服务端含义，却会在浏览器
// 网络记录里留下一条 net::ERR_ABORTED 被误读为请求失败，故只丢弃句柄、让该流自行读到 EOF。
const releaseSubscription = (state: RunState): void => {
  if (state.unsubscribe === null) {
    return;
  }
  if (state.streaming || state.awaitingApproval) {
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
      state.streaming = true;
      state.awaitingApproval = false;
      state.buffer = "";
      state.pendingCalls = [];
      localManager.setAgentRunSeq(sessionID, 0);
      return state;
    },
    // restore 用 live-run 状态恢复运行态（刷新 / 重开抽屉后续播），保留已持久化的续播游标
    restore(
      sessionID: number,
      patch: {
        runID: string;
        streaming: boolean;
        awaitingApproval: boolean;
        pendingCalls: RunPendingCall[];
      },
    ): RunState {
      const state = this.stateOf(sessionID);
      state.runID = patch.runID;
      state.streaming = patch.streaming;
      state.awaitingApproval = patch.awaitingApproval;
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
    // appendBuffer 累积助手正文增量
    appendBuffer(sessionID: number, delta: string): void {
      this.stateOf(sessionID).buffer += delta;
    },
    // addPendingCall 记录一条待审批调用（按 callID 幂等：回放重复下发不重复入列），
    // 并把运行态置为「等待审批」
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
      state.awaitingApproval = true;
      state.streaming = false;
    },
    // markAwaiting 更新等待审批标记（提交本轮结果续跑后置 false）
    markAwaiting(sessionID: number, awaiting: boolean): void {
      this.stateOf(sessionID).awaitingApproval = awaiting;
    },
    // settle 运行结束（done / error）：清除运行中与待审批标记
    settle(sessionID: number): void {
      const state = this.stateOf(sessionID);
      state.streaming = false;
      state.awaitingApproval = false;
      state.pendingCalls = [];
    },
    // attach 登记当前订阅的退订句柄：替换旧句柄（旧订阅先释放），由 store 承担订阅所有权
    attach(sessionID: number, unsubscribe: () => void): void {
      const state = this.stateOf(sessionID);
      releaseSubscription(state);
      state.unsubscribe = unsubscribe;
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

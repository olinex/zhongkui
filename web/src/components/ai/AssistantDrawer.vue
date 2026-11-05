<template>
  <v-navigation-drawer
    v-model="opened"
    location="right"
    temporary
    :width="drawerWidth"
    :class="{ 'assistant-drawer--resizing': drawerResizing }"
  >
    <div class="assistant-drawer d-flex flex-column h-100">
      <!-- 左缘拖拽手柄：向左拖动即增宽（抽屉贴右缘），握把样式与通用手柄组件一致。
           placement="outside"：8px 命中条越过抽屉边框贴到外侧，不再压住柜内内容；
           代价是内容区须放开横向裁剪，见 <style> 中的 :deep(.v-navigation-drawer__content) -->
      <resize-handle
        v-model="drawerWidth"
        side="left"
        placement="outside"
        :min="DRAWER_WIDTH_MIN"
        :max="maxDrawerWidth"
        @resize-start="drawerResizing = true"
        @resize-end="onDrawerResizeEnd"
      />
      <!-- 头部：标题 + 会话切换 + 新建会话 + 关闭 -->
      <div class="d-flex align-center pa-2">
        <v-icon icon="mdi-robot-outline" color="primary" class="me-2" />
        <span class="zk-section-title flex-grow-1 text-truncate" :title="headerTitle">
          {{ headerTitle }}
        </span>
        <!-- 会话列表：点击后在侧边栏内直接展开全部会话（替换消息区与输入区），
             展开态以 primary 高亮标识 -->
        <v-btn
          variant="text"
          icon="mdi-format-list-bulleted"
          :color="sessionListMode ? 'primary' : undefined"
          :disabled="!aiEnabled"
          :title="$t('model.aiSession.name')"
          :aria-label="$t('model.aiSession.name')"
          @click="toggleSessionList"
        />
        <v-btn
          variant="text"
          icon="mdi-plus"
          :disabled="!aiEnabled"
          :title="$t('page.aiAssistant.newSession')"
          :aria-label="$t('page.aiAssistant.newSession')"
          @click="startNewSession"
        />
        <v-btn
          variant="text"
          icon="mdi-close"
          :title="$t('action.close')"
          :aria-label="$t('action.close')"
          @click="opened = false"
        />
      </div>
      <v-divider />

      <!-- 会话列表态：替换消息区与输入区，在侧边栏内直接平铺全部会话；
           整行可点击切换，右侧常驻重命名 / 删除按钮 -->
      <section v-if="sessionListMode" class="flex-grow-1 overflow-y-auto pa-3">
        <div
          v-if="sessions.length === 0"
          class="d-flex flex-column align-center justify-center h-100 text-center"
        >
          <div class="zk-hint">{{ $t("page.aiAssistant.hintNoSession") }}</div>
        </div>
        <div v-else class="zk-panel session-panel">
          <div
            v-for="(s, idx) in sessions"
            :key="s.id"
            class="zk-chat-session-row zk-row-hover d-flex align-center py-2 px-3"
            :class="{
              'zk-row-selected': activeSessionID === s.id,
              'zk-hairline-top': idx > 0,
            }"
            @click="pickSession(s.id)"
          >
            <div class="zk-chat-session-main">
              <div class="text-body-medium text-truncate">
                {{ s.title }}
              </div>
              <div class="zk-hint">{{ localeDatetimeFormat(DateTime.fromISO(s.updatedAt)) }}</div>
            </div>
            <v-btn
              icon="mdi-pencil-outline"
              size="small"
              variant="text"
              :title="$t('page.aiAssistant.renameSession')"
              :aria-label="$t('page.aiAssistant.renameSession')"
              @click.stop="openRenameSessionDialog(s)"
            />
            <v-btn
              icon="mdi-delete-outline"
              size="small"
              variant="text"
              :title="$t('page.aiAssistant.deleteSession')"
              :aria-label="$t('page.aiAssistant.deleteSession')"
              @click.stop="openDeleteSessionDialog(s)"
            />
          </div>
        </div>
      </section>

      <!-- 消息区：提问轨由 assistant-question-rail 自行绝对定位在抽屉左缘（整高、不随正文滚动），
           故这里只有滚动正文；ps-6(24px) 让出 8px 轨 + 8px 间隙，余量 8px 与全抽屉的 8px
           内边距观感一致（左缘拖拽手柄已改为贴到抽屉外侧，不再占这里的横向空间）
           没有提问时轨道不渲染，左内边距仍保留（值不足以按提问数变化） -->
      <div v-if="!sessionListMode" class="assistant-conversation d-flex flex-grow-1 min-h-0">
        <assistant-question-rail
          v-if="userQuestions.length >= 1"
          :questions="userQuestions"
          :active-key="activeQuestionKey"
          :at-bottom="messagesAtBottom"
          :panel-max-width="questionPanelMaxWidth"
          @select="scrollToTurn"
          @bottom="scrollToBottom"
        />
        <div
          ref="messagesRef"
          class="assistant-messages flex-grow-1 overflow-y-auto py-3 pe-3 ps-6"
          @scroll.passive="updateActiveQuestion"
        >
          <!-- 空态提示：仅在没有会话且消息区为空时展示。待创建态下首条消息会在创建等待期
             本地回显（见 echoUserQuery），此时消息区已有内容，不能再被空态提示顶掉 -->
          <div
            v-if="!activeSessionID && messages.length === 0"
            class="d-flex flex-column align-center justify-center h-100 text-center"
          >
            <div class="zk-hint">
              {{
                $t(
                  draftMode || sessions.length === 0
                    ? "page.aiAssistant.hintFirstQuery"
                    : "page.aiAssistant.hintSelectSession",
                )
              }}
            </div>
          </div>
          <template v-else>
            <div
              v-for="m in messages"
              :key="m.key"
              :ref="(el) => setTurnRef(m.key, el)"
              class="assistant-turn zk-chat-turn d-flex flex-column mb-4"
              :class="m.role === 'user' ? 'assistant-turn--user' : 'assistant-turn--assistant'"
            >
              <!-- 仅用户消息用气泡框包裹；助手回答内容多，气泡会把正文压成窄长条，
                 故改为直接铺满消息区宽度（见 .assistant-bubble--assistant） -->
              <div
                class="assistant-bubble"
                :class="
                  m.role === 'user'
                    ? 'zk-chat-bubble bg-primary zk-chat-bubble--user assistant-bubble--user pa-2 px-3'
                    : 'assistant-bubble--assistant'
                "
              >
                <div class="d-flex align-center mb-1">
                  <span class="zk-hint">
                    {{
                      $t(m.role === "user" ? "page.aiAssistant.me" : "page.aiAssistant.assistant")
                    }}
                  </span>
                  <v-spacer />
                  <span v-if="m.streaming" class="zk-hint">
                    {{
                      $t(
                        awaitingFirstResponse(m)
                          ? "page.aiAssistant.awaitingResponse"
                          : "page.aiAssistant.generating",
                      )
                    }}
                  </span>
                </div>
                <!-- 携带技能发送的用户消息：技能名常驻气泡顶部，表明本轮使用的技能。技能锚定在用户
                     消息（轮次）上，即时气泡与刷新后的历史气泡同源展示，同一会话各轮可带不同技能 -->
                <div v-if="m.skillName" class="d-flex align-center ga-1 mb-1">
                  <v-icon icon="mdi-lightning-bolt-outline" size="x-small" />
                  <span class="text-body-small">{{ m.skillName }}</span>
                </div>
                <!-- 消息体内按时序排布的内容块（见 assistantBlocksOf）：工具调用阶段按轮聚组——该轮
                   思考折叠块渲染一次、置于组上方，组内各调用行自上而下成行；一轮内并发多次调用时以
                   并行计数与左侧细线框成一组，体现「一次思考 → N 次并行调用」。人工审批与主动提问两块
                   按其触发轮次内联插入，使整条消息自上而下严格按时序阅读——作答 / 批准之后新增的思考
                   与调用必然落在交互块之后，不再出现「后续过程反在提问之上」的错位 -->
                <div v-if="assistantBlocksOf(m).length > 0" class="mt-1">
                  <template v-for="b in assistantBlocksOf(m)" :key="b.key">
                    <!-- 末个思考折叠块（b.primary）：流式期间默认展开、结束自动折叠，受控 v-model 承载
                         用户手动切换；流式增长期内把其滚动区粘到底部 -->
                    <div
                      v-if="b.kind === 'reasoning' && b.primary"
                      :data-block-key="m.key + ':' + b.key"
                      class="zk-panel assistant-reasoning my-2"
                    >
                      <v-expansion-panels
                        :model-value="m.reasoningExpanded ? REASONING_PANEL : ''"
                        @update:model-value="(v: unknown) => setReasoningExpanded(m, v)"
                      >
                        <v-expansion-panel :value="REASONING_PANEL">
                          <expansion-panel-title
                            :icon="reasoningStreamingOf(m) ? 'mdi-loading' : 'mdi-brain'"
                            :spin="reasoningStreamingOf(m)"
                            size="sm"
                            :title="
                              reasoningStreamingOf(m)
                                ? $t('page.aiAssistant.reasoningStreaming')
                                : $t('page.aiAssistant.reasoningDone')
                            "
                          >
                            <!-- 思考来源说明：当前档位模型未配置思考开关（开关已隐藏），却仍产出思考内容，
                               说明是该模型默认开启思考，以警示图标 + 悬浮提示解释来源 -->
                            <reasoning-default-on-hint
                              v-if="!slotThinkingToggle"
                              :max-width="REASONING_NOTE_MAX_WIDTH"
                            />
                          </expansion-panel-title>
                          <v-expansion-panel-text>
                            <div
                              :ref="(el) => setReasoningBodyRef(m.key, el)"
                              class="assistant-reasoning-body"
                            >
                              <assistant-markdown
                                :content="b.text ?? ''"
                                :streaming="reasoningStreamingOf(m)"
                              />
                            </div>
                          </v-expansion-panel-text>
                        </v-expansion-panel>
                      </v-expansion-panels>
                    </div>
                    <!-- 其余各轮思考折叠块：锚定在对应调用组上方（历史消息无分段时整条思考落到正文上方那块） -->
                    <div
                      v-else-if="b.kind === 'reasoning'"
                      :data-block-key="m.key + ':' + b.key"
                      class="zk-panel assistant-reasoning my-2"
                    >
                      <v-expansion-panels>
                        <v-expansion-panel :value="REASONING_PANEL">
                          <expansion-panel-title
                            icon="mdi-brain"
                            size="sm"
                            :title="$t('page.aiAssistant.reasoningDone')"
                          >
                            <!-- 与末个思考块共用同一来源说明：更早各轮同样由默认开启思考的模型产出 -->
                            <reasoning-default-on-hint
                              v-if="!slotThinkingToggle"
                              :max-width="REASONING_NOTE_MAX_WIDTH"
                            />
                          </expansion-panel-title>
                          <v-expansion-panel-text>
                            <div class="assistant-reasoning-body">
                              <assistant-markdown :content="b.text ?? ''" />
                            </div>
                          </v-expansion-panel-text>
                        </v-expansion-panel>
                      </v-expansion-panels>
                    </div>
                    <!-- 正文块：整条消息末尾的最终正文与其间各轮说明文本各成一块（b.primary 为末尾最终
                         正文，流式期间禁用 mermaid），按 seq 自然排布 -->
                    <div
                      v-else-if="b.kind === 'content'"
                      :data-block-key="m.key + ':' + b.key"
                      class="mt-2"
                    >
                      <assistant-markdown
                        :content="b.text ?? ''"
                        :streaming="b.primary === true && m.streaming === true"
                        :citations="citationsOf(m)"
                      />
                    </div>
                    <div
                      v-else-if="b.kind === 'stages'"
                      :data-block-key="m.key + ':' + b.key"
                      class="assistant-stage-group"
                      :class="(b.parallelCount ?? 0) > 1 ? ['zk-chat-parallel', 'ps-3'] : undefined"
                    >
                      <div v-if="(b.parallelCount ?? 0) > 1" class="zk-hint mb-1">
                        {{ $t("page.aiAssistant.parallelCalls", { count: b.parallelCount ?? 0 }) }}
                      </div>
                      <div v-for="s in b.stages" :key="s.key" class="assistant-stage py-1">
                        <v-icon
                          :icon="stageIconOf(s, m.streaming === true)"
                          :color="stageColorOf(s, m.streaming === true)"
                          size="small"
                          :class="{ 'mdi-spin': m.streaming === true && s.status === 'start' }"
                        />
                        <span class="text-body-medium">{{ stageLabelOf(s.stage) }}</span>
                        <span v-if="s.tool" class="zk-hint font-mono">{{ s.tool }}</span>
                        <v-spacer />
                        <span v-if="s.elapsedMs > 0 || s.error" class="zk-hint">
                          {{ stageMetaTextOf(s) }}
                        </span>
                      </div>
                    </div>
                    <!-- 主动提问：每轮各自渲染成一个独立问答框（多轮提问按轮次先后自上而下排列，最新一轮
                         沉在底部，避免用户找不到新提问）——仍待回答的那轮渲染交互问卷面板（会话被阻塞，
                         由输入区禁用体现），已作答（含取消）的各轮渲染问答记录卡片；「重新回答」入口仅
                         末轮提供（更早各轮的运行现场已被消费）。
                         面板自身保持完全可交互——不传会话级 busy：问卷挂起时 awaitingApproval 为
                         true、busy 亦为 true，若以 busy 闸住面板会把「下一题 / 提交 / 取消」一并锁死。
                         现场不可用（无未决策的 ask_user 调用可回传）时降级为只读展示并给出说明文本，
                         避免提交 / 取消可点却无反应、也不发请求 -->
                    <div
                      v-else-if="b.kind === 'questionnaire' && b.questionnaire"
                      :data-block-key="m.key + ':' + b.key"
                      class="my-2"
                    >
                      <ask-user-questionnaire
                        v-if="b.questionnaire.awaiting"
                        :payload="questionnairePayloadOf(b.questionnaire)"
                        :disabled="!questionnaireInteractive(m)"
                        @submit="submitQuestionnaire"
                        @cancel="cancelQuestionnaire"
                      />
                      <div v-else class="zk-hairline-top pt-2">
                        <div class="d-flex align-center mb-1">
                          <v-icon icon="mdi-comment-question-outline" size="small" class="me-1" />
                          <span class="zk-label-caps">{{
                            $t("page.aiAssistant.questionnaireTitle")
                          }}</span>
                          <v-spacer />
                          <!-- 重新回答：仅末轮（最新一轮）可重答，更早各轮的现场已被后续轮次消费；
                               回退到该轮未作答状态（删除该轮作答之后的回答，不重跑整轮），由重放的挂起
                               问卷重新作答；会话正忙时禁用，仅以提示说明原因。禁用按钮自身不派发指针
                               事件，故套一层 span 承载气泡激活器 -->
                          <v-tooltip
                            v-if="b.reanswerable && (b.questionnaireInterruptIDs?.length ?? 0) > 0"
                            location="top"
                            :text="$t('page.aiAssistant.questionnaireReanswerBusy')"
                            :disabled="!busy"
                          >
                            <template #activator="{ props: reanswerProps }">
                              <span v-bind="reanswerProps" class="d-inline-flex">
                                <v-btn
                                  size="small"
                                  variant="text"
                                  :disabled="busy"
                                  @click="
                                    openRerunConfirm('answer', {
                                      interruptIDs: b.questionnaireInterruptIDs,
                                    })
                                  "
                                >
                                  {{ $t("page.aiAssistant.questionnaireReanswer") }}
                                </v-btn>
                              </span>
                            </template>
                          </v-tooltip>
                        </div>
                        <div
                          v-for="(q, idx) in b.questionnaire.questions"
                          :key="q.id || idx"
                          class="zk-panel--subtle pa-2 mb-2"
                        >
                          <div class="text-body-medium">{{ q.question }}</div>
                          <div class="zk-hint mt-1">
                            {{ questionnaireAnswerTextOf(b.questionnaire, q.id) }}
                          </div>
                        </div>
                      </div>
                    </div>
                    <!-- 人工审批：模型请求调用平台原生接口，同一批多个调用由审批步骤组件整合成
                         一个多步骤面板（一项一步），逐项批准 / 拒绝；ask_user 问卷类调用豁免审批，
                         由后端 approval 块直接排除、不出现在此面板 -->
                    <div
                      v-else-if="b.kind === 'approval'"
                      :data-block-key="m.key + ':' + b.key"
                      class="my-2"
                    >
                      <assistant-approval-steps
                        :calls="b.approvalCalls ?? []"
                        :interactive="m.awaitingApproval === true"
                        :executing-call-ids="executingCallIDsOf(m)"
                        @approve="(callID) => approveCall(m, callID)"
                        @reject="(callID) => rejectCall(m, callID)"
                      >
                        <!-- 重新审批：仅历史（已决策）授权回合提供——回退到该次审批的待审状态
                             （删除该审批之后的回答，不重跑整轮），由重放的挂起卡片重新提供批准 /
                             拒绝；会话正忙时禁用，仅以提示说明原因。禁用按钮自身不派发指针事件，
                             故套一层 span 承载气泡激活器 -->
                        <template
                          v-if="m.awaitingApproval !== true && (b.approvalCallIDs?.length ?? 0) > 0"
                          #action
                        >
                          <v-tooltip
                            location="top"
                            :text="$t('page.aiAssistant.approvalRerunBusy')"
                            :disabled="!busy"
                          >
                            <template #activator="{ props: rerunProps }">
                              <span v-bind="rerunProps" class="d-inline-flex">
                                <v-btn
                                  size="small"
                                  variant="text"
                                  class="ms-2"
                                  :disabled="busy"
                                  @click="
                                    openRerunConfirm('approval', { callIDs: b.approvalCallIDs })
                                  "
                                >
                                  {{ $t("page.aiAssistant.approvalRerun") }}
                                </v-btn>
                              </span>
                            </template>
                          </v-tooltip>
                        </template>
                      </assistant-approval-steps>
                    </div>
                    <!-- 推荐的下一步：仅最新一条助手回答、非流式态展示。点击行直接发送；右侧按钮把文本
                         追加到输入框（已有内容时换行 append）。输入框非空 / 会话忙 / 已锁定时禁用直接发送，
                         以 tooltip 说明原因，避免清空用户草稿 -->
                    <div
                      v-else-if="
                        b.kind === 'nextSteps' &&
                        m.key === latestAssistantKey &&
                        !m.streaming &&
                        m.phase !== 'failed' &&
                        m.phase !== 'cancelled'
                      "
                      :data-block-key="m.key + ':' + b.key"
                      class="zk-panel--subtle pa-2 w-100 mt-3"
                    >
                      <div class="d-flex align-center ga-1 mb-1">
                        <v-icon icon="mdi-lightbulb-on-outline" size="small" color="primary" />
                        <span class="zk-label-caps">{{
                          $t("page.aiAssistant.nextStepsTitle")
                        }}</span>
                        <v-spacer />
                        <span class="zk-hint">{{ $t("page.aiAssistant.nextStepsHint") }}</span>
                      </div>
                      <div class="d-flex flex-column ga-1">
                        <div
                          v-for="(step, idx) in b.nextSteps ?? []"
                          :key="`${idx}-${step.text}`"
                          class="zk-chat-next-step d-flex align-center py-1 px-1 ps-2"
                          :class="{
                            'zk-row-hover': canSendNextStep,
                            'zk-chat-next-step--disabled': !canSendNextStep,
                          }"
                        >
                          <v-tooltip
                            location="top"
                            :text="nextStepSendBlockedReason"
                            :disabled="canSendNextStep"
                          >
                            <template #activator="{ props: sendProps }">
                              <span v-bind="sendProps" class="assistant-next-step-activator">
                                <button
                                  type="button"
                                  class="zk-chat-next-step-main d-flex align-center"
                                  :disabled="!canSendNextStep"
                                  @click="sendNextStep(step.text)"
                                >
                                  <span class="assistant-next-step-text">
                                    <span class="text-body-medium">
                                      {{ step.title || step.text }}
                                    </span>
                                    <span v-if="step.reason" class="zk-hint">
                                      {{ step.reason }}
                                    </span>
                                  </span>
                                </button>
                              </span>
                            </template>
                          </v-tooltip>
                          <v-chip
                            v-if="step.recommended || idx === 0"
                            size="x-small"
                            color="primary"
                            variant="tonal"
                            class="flex-grow-0 flex-shrink-0"
                          >
                            {{ $t("page.aiAssistant.nextStepsRecommended") }}
                          </v-chip>
                          <v-tooltip
                            location="top"
                            :text="
                              nextStepFillBlockedReason || $t('page.aiAssistant.nextStepsFill')
                            "
                          >
                            <template #activator="{ props: fillProps }">
                              <span v-bind="fillProps" class="d-inline-flex">
                                <v-btn
                                  :icon="true"
                                  size="small"
                                  variant="text"
                                  :disabled="!canFillNextStep"
                                  :aria-label="$t('page.aiAssistant.nextStepsFill')"
                                  @click.stop="appendNextStep(step.text)"
                                >
                                  <v-icon icon="mdi-content-paste" size="x-small" />
                                </v-btn>
                              </span>
                            </template>
                          </v-tooltip>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
                <!-- 无块回退：用户消息按纯文本展示；助手消息无块（尚未产出 / 全是空档）时以纯文本兜底 -->
                <div v-else-if="m.content" class="assistant-plain text-body-medium">
                  {{ m.content }}
                </div>
                <!-- 运行未正常结束提示：助手消息 phase 为 failed / cancelled 时展示，按阶段取文案
                   （由前端 i18n 提供，不落库、不混入正文）；以警示图标 + 弱化底与正文区分 -->
                <div v-if="interruptedTextOf(m)" class="zk-panel--subtle pa-2 mt-2">
                  <div class="d-flex align-center">
                    <v-icon icon="mdi-alert-circle-outline" size="small" color="warning" />
                    <span class="zk-hint ms-2">{{ interruptedTextOf(m) }}</span>
                  </div>
                  <!-- 失败原因原样透出：才有机会看出是工具、模型还是续跑现场的问题 -->
                  <div v-if="m.turnError" class="zk-error-text ms-6 mt-1">{{ m.turnError }}</div>
                </div>
                <!-- 引用来源仅在整轮回答完整结束后展示：message.citations 会先于 turn.completed
                     到达（检索命中后即下发），提前渲染会让引用一直挂在流式正文下方；等待审批 /
                     作答、失败、取消同样不算完整结束 -->
                <template v-if="citationsOf(m).length > 0 && !m.streaming && m.phase === 'done'">
                  <v-divider class="my-2" />
                  <div :data-block-key="m.key + ':citations'" class="zk-label-caps mb-1">
                    {{ $t("page.aiAssistant.citations") }}
                  </div>
                  <v-list class="bg-transparent">
                    <v-list-item v-for="(c, idx) in visibleCitations(m)" :key="idx">
                      <v-list-item-title class="text-body-medium">
                        <!-- 序号与正文中的 [n] 标注一一对应，便于对照「哪一段引用的是哪个来源」 -->
                        <span class="zk-chat-citation-index">[{{ idx + 1 }}]</span>
                        <!-- 来源命名空间可溯源（含历史消息缺失该字段的降级）时标题渲染为链接，
                           直接跳转文档详情页；缺失则保持纯文本，避免产生无效跳转 -->
                        <a
                          v-if="c.namespaceName"
                          class="zk-chat-citation-link"
                          :href="knowledgeDocumentHref(c.documentID, c.namespaceName)"
                          target="_blank"
                          rel="noopener noreferrer"
                          :title="$t('page.aiAssistant.citationOpen')"
                        >
                          {{ c.title }}
                        </a>
                        <template v-else>{{ c.title }}</template>
                      </v-list-item-title>
                      <v-list-item-subtitle class="assistant-citation-text">
                        {{ c.content }}
                      </v-list-item-subtitle>
                    </v-list-item>
                  </v-list>
                  <!-- 引用超过 3 条时默认收起其余条目，由用户按需展开 / 收起 -->
                  <v-btn
                    v-if="citationsOf(m).length > CITATION_VISIBLE_LIMIT"
                    size="small"
                    variant="text"
                    class="zk-chat-citations-toggle"
                    @click="toggleCitations(m)"
                  >
                    {{
                      citationsExpandedOf(m)
                        ? $t("action.collapse")
                        : $t("page.aiAssistant.citationsExpand", {
                            count: citationsOf(m).length - CITATION_VISIBLE_LIMIT,
                          })
                    }}
                    <v-icon
                      class="ms-1"
                      size="x-small"
                      :icon="citationsExpandedOf(m) ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                    />
                  </v-btn>
                </template>
              </div>
              <!-- 助手回答快捷操作：回答完（非生成中）后常驻展示——助手内容多且已去掉气泡，
                 操作不必再靠悬停去发现；复制整段回答 / 重试本轮 / 以本轮为结尾分支出新会话 -->
              <div
                v-if="m.role === 'assistant' && !m.streaming"
                class="zk-chat-actions zk-chat-actions--persistent d-flex align-center ga-2 mt-1"
              >
                <!-- 用量说明置于操作图标之后、同一行：末尾先给三个操作，再补「花了多少 token、多久」 -->
                <div class="assistant-message-buttons">
                  <v-btn
                    icon="mdi-content-copy"
                    size="small"
                    variant="text"
                    :title="$t('page.aiAssistant.copyMessage')"
                    :aria-label="$t('page.aiAssistant.copyMessage')"
                    @click="copyMessage(m)"
                  />
                  <v-btn
                    icon="mdi-refresh"
                    size="small"
                    variant="text"
                    :disabled="busy"
                    :title="$t('page.aiAssistant.retryMessage')"
                    :aria-label="$t('page.aiAssistant.retryMessage')"
                    @click="retryMessage(m)"
                  />
                  <v-btn
                    icon="mdi-source-branch"
                    size="small"
                    variant="text"
                    :disabled="busy"
                    :title="$t('page.aiAssistant.branchMessage')"
                    :aria-label="$t('page.aiAssistant.branchMessage')"
                    @click="branchMessage(m)"
                  />
                  <v-btn
                    icon="mdi-archive-arrow-down-outline"
                    size="small"
                    variant="text"
                    :disabled="busy || m.id === undefined || foldedMessageKeys.has(m.key)"
                    :title="$t('page.aiAssistant.compressToHere')"
                    :aria-label="$t('page.aiAssistant.compressToHere')"
                    @click="compactToHere(m)"
                  />
                </div>
                <!-- 用量合计：悬浮查看输入 / 输出 token 明细（无明细时气泡禁用，不弹空泡） -->
                <v-tooltip
                  v-if="usageTextOf(m)"
                  location="top"
                  :text="usageDetailOf(m)"
                  :disabled="!usageDetailOf(m)"
                >
                  <template #activator="{ props: usageProps }">
                    <span v-bind="usageProps" class="zk-hint">{{ usageTextOf(m) }}</span>
                  </template>
                </v-tooltip>
              </div>
              <!-- 上下文压缩展示块：自动压缩挂本轮助手消息、手动压缩挂被点击的助手消息，
                 仅当该消息带压缩态时展示。属运行元信息（手动压缩语义即「压缩到此」），
                 故置于该轮快捷操作行下方、作为整轮最后一条提示——既不与正文 / 思考块抢位置，
                 也让操作行紧贴它所操作的正文 -->
              <div
                v-if="m.compress"
                class="zk-panel--subtle mt-2 w-100 pa-2"
                :class="{ 'zk-chat-compress--failed': m.compress.phase === 'failed' }"
              >
                <!-- 标题行兼展开开关：完成后有摘要时整行可点击（键盘 Enter / 空格同效），右侧箭头指示
                     展开态；进行中不提供展开，失败 / 空摘要同样不提供交互 -->
                <div
                  class="d-flex align-center"
                  :class="{ 'assistant-compress-toggle': compressExpandableOf(m) }"
                  :role="compressExpandableOf(m) ? 'button' : undefined"
                  :tabindex="compressExpandableOf(m) ? 0 : undefined"
                  :aria-expanded="compressExpandableOf(m) ? compressExpandedOf(m) : undefined"
                  @click="toggleCompress(m)"
                  @keydown.enter.prevent="toggleCompress(m)"
                  @keydown.space.prevent="toggleCompress(m)"
                >
                  <v-icon
                    :icon="compressIconOf(m.compress)"
                    :color="compressColorOf(m.compress)"
                    size="small"
                    :class="{ 'mdi-spin': m.compress.phase === 'running' }"
                  />
                  <span class="zk-hint ms-2">{{ compressTextOf(m) }}</span>
                  <v-spacer />
                  <v-icon
                    v-if="compressExpandableOf(m)"
                    :icon="compressExpandedOf(m) ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                    size="x-small"
                    class="zk-hint"
                  />
                </div>
                <!-- 进行中：摘要增量实时累积，三行截断预览（不完整阶段不做完整收纳） -->
                <div
                  v-if="m.compress.phase === 'running' && m.compress.draft"
                  class="zk-code-preview font-mono assistant-compress-draft mt-1"
                >
                  {{ m.compress.draft }}
                </div>
                <!-- 完成后：默认完全收起（不展示正文），点击标题行 / 箭头后展开限高滚动查看全文 -->
                <div
                  v-if="compressExpandedOf(m) && compressBodyOf(m)"
                  class="zk-code-preview font-mono assistant-compress-draft assistant-compress-draft--expanded mt-1"
                >
                  {{ compressBodyOf(m) }}
                </div>
              </div>
              <!-- 用户消息快捷操作：悬停该轮时淡入（左侧创建时间 + 复制 / 回退 / 删除）；回退与删除在生成中禁用，
                 避免截断在途运行正在使用的消息；复制不受影响，等待用户操作时也应可用 -->
              <div v-if="m.role === 'user'" class="zk-chat-actions d-flex align-center ga-2 mt-1">
                <span class="zk-hint">{{ messageTimeOf(m) }}</span>
                <div class="assistant-message-buttons">
                  <v-btn
                    icon="mdi-content-copy"
                    size="small"
                    variant="text"
                    :title="$t('page.aiAssistant.copyMessage')"
                    :aria-label="$t('page.aiAssistant.copyMessage')"
                    @click="copyMessage(m)"
                  />
                  <v-btn
                    icon="mdi-undo-variant"
                    size="small"
                    variant="text"
                    :disabled="busy"
                    :title="$t('page.aiAssistant.rollbackMessage')"
                    :aria-label="$t('page.aiAssistant.rollbackMessage')"
                    @click="openMessageAction(m, true)"
                  />
                  <v-btn
                    icon="mdi-delete-outline"
                    size="small"
                    variant="text"
                    :disabled="busy"
                    :title="$t('page.aiAssistant.deleteMessage')"
                    :aria-label="$t('page.aiAssistant.deleteMessage')"
                    @click="openMessageAction(m, false)"
                  />
                </div>
              </div>
            </div>
          </template>
        </div>
        <!-- 块间快速跳读：长回答内按块（思考 / 工具调用 / 审批 / 问卷 / 正文 / 下一步 / 引用）上下跳。
             浮于消息区右缘（不占正文宽度），右移让开滚动条；无任何块时整体隐藏 -->
        <assistant-block-nav
          v-if="blockNavVisible"
          class="assistant-block-nav-slot"
          :current="blockNavCurrent"
          :total="blockNavTotal"
          :can-prev="blockNavCanPrev"
          :can-next="blockNavCanNext"
          @prev="scrollToBlock(-1)"
          @next="scrollToBlock(1)"
        />
      </div>

      <!-- 输入区：与消息区之间不设分隔线，让输入框直接落在对话内部，读作同一整体 -->
      <div v-if="!sessionListMode" class="pa-3">
        <v-alert v-if="aiStatus && !aiStatus.enabled" type="warning" class="mb-3">
          <div>{{ aiStatus.reason || $t("page.aiAssistant.disabled") }}</div>
          <v-btn variant="text" size="small" class="mt-1" @click="gotoLlmConfig">
            {{ $t("page.featureUnavailable.activate", { page: $t("navbar.admin.llmConfig") }) }}
          </v-btn>
        </v-alert>
        <!-- 单一边框容器：正文在上，档位选择与发送按钮内置其底部（对齐主流对话产品）。
             底色取弱化面板（on-surface 6% + hairline），让输入区在消息流中凸显出来 -->
        <div
          class="zk-chat-composer zk-panel zk-panel--subtle"
          :class="{ 'zk-chat-composer--blocked': composerDisabled }"
        >
          <!-- 会话状态条：输入被闸住时展示原因——运行 / 等待子状态取后端枚举 label（正在思考 /
               正在调用工具 / 等待审批…），已锁定则展示锁定提示与「回滚后可继续提问」引导。
               以图标 + 缩进与输入区其它部分作视觉区分，不与下方控件抢占 -->
          <div
            v-if="inputGated && !questionnairePending"
            class="zk-chat-composer-status zk-hint d-flex align-center ga-1 mt-2 mx-2 py-1 px-2"
            :class="{ 'zk-chat-composer-status--locked': locked }"
          >
            <v-progress-circular v-if="!locked" indeterminate size="14" width="2" color="primary" />
            <v-icon v-else icon="mdi-lock-outline" size="x-small" />
            <span>{{ composerStatusText }}</span>
          </div>
          <!-- 问卷待作答提示行：问卷已内联在消息流中（见 assistantBlocksOf），会话被阻塞，
               输入区整体禁用；此处置一条提示并给出「查看问题」入口，把用户视线引回消息流里的问卷 -->
          <div v-if="questionnairePending" class="zk-hint d-flex align-center ga-1 pt-2 px-2">
            <v-icon icon="mdi-comment-question-outline" size="x-small" />
            <span>{{ $t("page.aiAssistant.questionnairePendingHint") }}</span>
            <v-spacer />
            <v-btn size="x-small" variant="text" @click="scrollToBottom">
              {{ $t("page.aiAssistant.questionnaireJump") }}
            </v-btn>
          </div>
          <!-- 待发提问队列：当前回答进行中提交的追问按序堆叠在输入框上方，本轮结束后自动依次投递。
               每项可置顶（移到队首）/ 修改（出队并回填输入框）/ 删除；后两者在输入框已有内容时
               一并禁用，避免回填覆盖用户已写内容 -->
          <div v-if="activeQueue.length > 0" class="d-flex flex-column ga-1 pt-2 px-2">
            <div
              v-for="item in activeQueue"
              :key="item.key"
              class="zk-chat-queue-item d-flex align-center ga-1 py-1 px-1 ps-2"
            >
              <v-icon icon="mdi-clock-outline" size="x-small" color="on-surface-variant" />
              <v-chip
                v-if="item.skillName"
                size="x-small"
                variant="tonal"
                prepend-icon="mdi-lightning-bolt-outline"
              >
                {{ item.skillName }}
              </v-chip>
              <span class="zk-chat-queue-text">{{ item.text }}</span>
              <v-btn
                icon="mdi-arrow-up-bold-outline"
                size="x-small"
                variant="text"
                :title="$t('action.pinTop')"
                :aria-label="$t('action.pinTop')"
                @click="pinQueued(item)"
              />
              <v-btn
                icon="mdi-pencil-outline"
                size="x-small"
                variant="text"
                :disabled="!canModifyQueued"
                :title="
                  canModifyQueued ? $t('action.edit') : $t('page.aiAssistant.queuedQueryLocked')
                "
                :aria-label="$t('action.edit')"
                @click="editQueued(item)"
              />
              <v-btn
                icon="mdi-delete-outline"
                size="x-small"
                variant="text"
                :title="$t('action.delete')"
                :aria-label="$t('action.delete')"
                @click="removeQueued(item)"
              />
            </div>
          </div>
          <!-- 技能下拉：输入框整段为「/关键字」时浮现在输入框上方，按关键字本地过滤已启用技能；
               上下键移动高亮、回车选中、Esc 关闭（见 onComposerKeydown / onComposerEnter） -->
          <div v-if="skillMenuVisible" class="zk-chat-skill-menu zk-panel zk-panel--subtle">
            <v-list v-if="filteredSkills.length > 0" density="compact" nav>
              <v-list-item
                v-for="(item, idx) in filteredSkills"
                :key="item.id"
                :active="idx === skillActiveIndex"
                @click="selectSkillAt(idx)"
              >
                <template #prepend>
                  <v-icon icon="mdi-lightning-bolt-outline" size="small" />
                </template>
                <v-list-item-title>{{ item.name }}</v-list-item-title>
                <v-list-item-subtitle v-if="item.description">
                  {{ item.description }}
                </v-list-item-subtitle>
              </v-list-item>
            </v-list>
            <div v-else class="zk-hint px-3 py-2">
              {{
                skillLoading
                  ? $t("page.aiAssistant.skillsLoading")
                  : $t("page.aiAssistant.skillsEmpty")
              }}
            </div>
          </div>
          <!-- 已选技能：选中后以 chip 常驻输入框上方，随本次提问发送；可点关闭取消选择 -->
          <div v-if="selectedSkill" class="d-flex align-center ga-1 pt-2 px-2">
            <v-chip
              size="small"
              variant="tonal"
              color="primary"
              closable
              prepend-icon="mdi-lightning-bolt-outline"
              @click:close="selectedSkill = null"
            >
              {{ selectedSkill.name }}
            </v-chip>
          </div>
          <v-textarea
            ref="composerRef"
            v-model="query"
            variant="plain"
            rows="2"
            auto-grow
            hide-details
            :disabled="composerDisabled"
            :placeholder="$t('page.aiAssistant.composerPlaceholder')"
            :title="$t('model.aiSession.field.query')"
            @keydown="onComposerKeydown"
            @keydown.enter.exact="onComposerEnter"
            @compositionstart="composerComposing = true"
            @compositionend="composerComposing = false"
          />
          <div class="d-flex align-center ga-2 py-1 px-2">
            <v-select
              v-model="slot"
              :items="slotOptions"
              variant="plain"
              hide-details
              :disabled="composerDisabled"
              :title="slotFullTitle"
              class="zk-chat-slot zk-control-hover"
            />
            <!-- 审核阀门（会话级）：切换即持久化，草稿态随创建请求写入。
                 关闭态以图标标识当前阀门，完整名称与说明由展开项与悬浮提示承载 -->
            <v-select
              :model-value="reviewValve"
              :items="reviewValveOptions"
              variant="plain"
              hide-details
              :disabled="composerDisabled"
              :title="reviewValveTooltip"
              class="zk-chat-valve zk-control-hover"
              @update:model-value="updateReviewValve"
            >
              <template #selection>
                <v-icon :icon="reviewValveIcon" size="small" />
              </template>
              <!-- 展开态每项左侧展示该阀门图标，label 下补一行说明文本，帮助理解把关强度 -->
              <template #item="{ props: itemProps, item }">
                <v-list-item
                  v-bind="itemProps"
                  :prepend-icon="item.icon"
                  :title="item.title"
                  :subtitle="item.subtitle || undefined"
                />
              </template>
            </v-select>
            <!-- 上下文占用指示：紧邻审核阀门右侧的小环 + 百分比文字（文字置于环外，环得以保持小尺寸）；
                 完整「占用 / 容量」置于悬浮提示。达压缩阈值转警示色，容量未配置（≤0）时整块隐藏；
                 容量取自当前档位，切换档位由此重算、不发请求 -->
            <v-tooltip
              v-if="contextUsageVisible"
              location="top"
              :text="
                $t('page.aiAssistant.contextUsageTitle', {
                  used: formatTokenCount(contextUsedTokens),
                  limit: formatTokenCount(contextLimitTokens),
                })
              "
            >
              <template #activator="{ props: activatorProps }">
                <div v-bind="activatorProps" class="d-flex align-center ga-1">
                  <v-progress-circular
                    :model-value="contextUsagePercent"
                    :size="16"
                    :width="2"
                    :color="contextUsageColor"
                  />
                  <span class="zk-chat-usage-percent" :class="`text-${contextUsageColor}`">
                    {{ $t("page.aiAssistant.contextUsage", { percent: contextUsagePercent }) }}
                  </span>
                </div>
              </template>
            </v-tooltip>
            <!-- 思考开关：仅当前档位绑定模型配置了思考开关字段（三者齐备）时展示；未配置时隐藏，
                 由该模型自身的默认思考行为决定是否产出思考内容（见思考过程区块的来源说明图标）。
                 开启后请求携带思考字段并展示思考过程；偏好跨会话本地记忆 -->
            <v-tooltip v-if="slotThinkingToggle" location="top" :text="thinkingTooltip">
              <template #activator="{ props: thinkingProps }">
                <div
                  v-bind="thinkingProps"
                  class="assistant-composer-thinking d-flex align-center ps-2"
                >
                  <v-switch
                    v-model="thinking"
                    density="compact"
                    color="primary"
                    hide-details
                    :disabled="composerDisabled"
                    :aria-label="$t('page.aiAssistant.thinking')"
                  />
                </div>
              </template>
            </v-tooltip>
            <!-- 右侧动作组：优化提问与发送 / 停止成组贴右，与左侧档位 / 阀门 / 占用指示 / 思考开关分区 -->
            <div class="d-flex align-center ga-2 ms-auto">
              <!-- 撤回优化：刚优化完、用户尚未改动或发送时，优化按钮转为撤回按钮，一键恢复优化前原文。
                   输入被改写或消息发出后自动回到「优化提问」（见 optimizeUndo 与 watch） -->
              <v-btn
                v-if="optimizeUndo"
                icon="mdi-backup-restore"
                size="small"
                variant="text"
                :disabled="!canUndoOptimize"
                :title="$t('page.aiAssistant.undoOptimizePrompt')"
                :aria-label="$t('page.aiAssistant.undoOptimizePrompt')"
                @click="undoOptimizePrompt"
              />
              <!-- 优化提问：把当前输入改写为更清晰、更易被正确理解的提问，结果整体替换输入框内容
                   （不自动发送）；生成中 / 等待审批 / 创建会话 / 优化途中或输入为空时禁用 -->
              <v-btn
                v-else
                icon="mdi-auto-fix"
                size="small"
                variant="text"
                :loading="optimizing"
                :disabled="!canOptimize"
                :title="$t('page.aiAssistant.optimizePrompt')"
                :aria-label="$t('page.aiAssistant.optimizePrompt')"
                @click="optimizePrompt"
              />
              <v-btn
                v-if="runActive"
                icon="mdi-stop"
                size="small"
                variant="tonal"
                color="error"
                :title="$t('page.aiAssistant.stop')"
                :aria-label="$t('page.aiAssistant.stop')"
                @click="cancelGeneration"
              />
              <!-- 发送 / 排队：创建会话（后端需同步生成主题，耗时明显）途中发送按钮转入加载态，
                   既挡住重复提交，也把「回车已受理、正在处理」显式交给用户；当前回答进行中或
                   上下文压缩在途时，同一按钮转入「加入待发队列」，把追问排在队尾，待运行结束自动依次投递 -->
              <v-btn
                v-else
                icon="mdi-arrow-up"
                size="small"
                variant="flat"
                color="primary"
                :loading="creating"
                :disabled="!canSend && !canEnqueueQueued"
                :title="$t(busy ? 'page.aiAssistant.enqueueQuery' : 'page.aiAssistant.generate')"
                :aria-label="
                  $t(busy ? 'page.aiAssistant.enqueueQuery' : 'page.aiAssistant.generate')
                "
                @click="onSubmitClick"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 重命名会话对话框 -->
    <temp-editable-dialog
      ref="renameSessionDialogRef"
      :title="$t('page.aiAssistant.renameSession')"
      :confirm-disabled="!renameForm.meta.value.valid"
      force-editable
      prepend-icon="mdi-pencil-outline"
      @confirm="renameSessionAction"
    >
      <template #body>
        <v-form>
          <v-text-field
            v-model="renameSessionTitle"
            v-bind="renameSessionTitleProps"
            :label="$t('model.aiSession.field.title')"
          />
        </v-form>
      </template>
    </temp-editable-dialog>

    <!-- 删除会话确认对话框 -->
    <confirm-dialog
      ref="deleteConfirmRef"
      :title="$t('page.aiAssistant.deleteSession')"
      prepend-icon="mdi-delete-outline"
      @confirm="deleteSessionAction"
    >
      <template #body>
        <confirm-alert
          :action="$t('action.delete')"
          :text="$t('page.aiAssistant.confirmDeleteSession')"
        />
      </template>
    </confirm-dialog>

    <!-- 消息截断确认对话框：回退 / 删除共用，标题与说明随操作类型切换 -->
    <confirm-dialog
      ref="messageActionDialogRef"
      :title="
        $t(
          pendingMessage?.rollback
            ? 'page.aiAssistant.rollbackMessage'
            : 'page.aiAssistant.deleteMessage',
        )
      "
      :prepend-icon="pendingMessage?.rollback ? 'mdi-undo-variant' : 'mdi-delete-outline'"
      @confirm="confirmMessageAction"
    >
      <template #body>
        <confirm-alert
          :action="
            $t(
              pendingMessage?.rollback
                ? 'page.aiAssistant.rollbackMessage'
                : 'page.aiAssistant.deleteMessage',
            )
          "
          :text="
            $t(
              pendingMessage?.rollback
                ? 'page.aiAssistant.confirmRollbackMessage'
                : 'page.aiAssistant.confirmDeleteMessage',
            )
          "
        />
      </template>
    </confirm-dialog>

    <!-- 回退到助手待输入回合确认对话框（重新回答问卷 / 重新授权工具共用一个弹窗，按种类切换文案）：
         提醒确认后本回合之后的全部内容将被清空并重新进入对应的等待态 -->
    <confirm-dialog
      ref="rerunConfirmRef"
      :title="
        pendingRerunKind === 'approval'
          ? $t('page.aiAssistant.approvalRerun')
          : $t('page.aiAssistant.questionnaireReanswer')
      "
      prepend-icon="mdi-refresh"
      @confirm="confirmRerun"
    >
      <template #body>
        <confirm-alert
          :action="
            pendingRerunKind === 'approval'
              ? $t('page.aiAssistant.approvalRerun')
              : $t('page.aiAssistant.questionnaireReanswer')
          "
          :text="
            pendingRerunKind === 'approval'
              ? $t('page.aiAssistant.confirmRerunApproval')
              : $t('page.aiAssistant.confirmReanswerQuestionnaire')
          "
        />
      </template>
    </confirm-dialog>
  </v-navigation-drawer>
</template>

<script setup lang="ts" name="assistant-drawer">
import axios from "axios";
import { DateTime } from "luxon";
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  agentAbort,
  agentInteractionReopen,
  agentMessageTruncate,
  agentPromptOptimize,
  agentSessionBranch,
  agentSessionCreate,
  agentSessionDelete,
  agentSessionListGet,
  agentSessionRename,
  agentSessionValveSet,
  agentSkillOptionsGet,
  agentStatusGet,
  assistantActionStream,
  assistantChatStream,
  assistantCompactStream,
  assistantEventsStream,
  assistantRenderStream,
  type AssistantStreamHandlers,
} from "@/apis/ai";
import type { AssistantEventUnion, AssistantMessageSnapshot } from "@/apis/@types/assistant";
import type {
  AgentApprovalBlockPayload,
  AgentApprovalCallRecord,
  AgentChatIn,
  AgentContentBlockPayload,
  AgentLiveRunOut,
  AgentMessageBlockOut,
  AgentNextStep,
  AgentNextStepsBlockPayload,
  AgentPromptOptimizeOut,
  AgentQuestionnaireBlockPayload,
  AgentReasoningBlockPayload,
  AgentSessionItem,
  AgentSkillOptionListOut,
  AgentStatusOut,
  AgentToolCallsBlockPayload,
  AssistantActionRequest,
  AssistantApprovalDecision,
  AssistantQuestionnaireAnswer,
  EvalStage,
  EvalStageStatus,
  QuestionnaireAnswer,
  QuestionnairePayload,
  RetrievalResult,
  ToolRequestPayload,
} from "@/apis/@types/ai";
import { apiRequestURL, getJwtHeader, getResponse, getSimpleResponse, request } from "@/apis/base";
import { useAgentRunStore, type RunState } from "@/stores/agentRun";
import { useAppStore } from "@/stores/app";
import { localManager } from "@/stores/local";
import { knowledgeDocumentHref } from "@/utils/ai";
import { copyToClipboard } from "@/utils/clipboard";
import { EnumClass, enumEntriesOf, enumLabelOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { formatTokenCount } from "@/utils/token";

const props = withDefaults(
  defineProps<{
    /** 抽屉当前是否展开（v-model）。 */
    modelValue: boolean;
  }>(),
  { modelValue: false },
);

// 抽屉开关状态变更事件。
const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const router = useRouter();
const route = useRoute();
const appStore = useAppStore();
// agentRun 运行态 store（单例）：承担 SSE 订阅所有权与续播游标，组件关闭抽屉 / 重挂载不影响在途流
const agentRun = useAgentRunStore();

// historyLoading 是否正在回放历史（/render 流）：回放期间需跟随贴底，实时增量则仅在底部附近跟随
let historyLoading = false;
// activeAssistantKey 当前正在流式 / 续跑的助手消息 key；历史重载后为 null。
let activeAssistantKey: number | null = null;
// pendingUserEcho 尚未出现在历史快照中的本地用户气泡（实时回合的 userMessage 到达后清空）。
let pendingUserEcho: ChatMessage | null = null;
// questionnaireOverrides 问卷本地收敛态：提交 / 取消后服务端不回推块更新，前端据此收起面板。
const questionnaireOverrides = new Map<
  number,
  { answers?: QuestionnaireAnswer[]; awaiting: boolean }
>();
// ApprovalOverride 单条调用的本地审批收敛态：服务端不回写已决策状态，而挂起块会随消息重建被
// 重新解析（消息上的 pendingCalls 视图态可能被覆盖），按 callID 记住决策与执行结果，重建时回填，
// 使「已拒绝 / 已批准」徽章与「等待审批」提示即时收敛、不回退。
interface ApprovalOverride {
  decision?: "approved" | "rejected";
  result?: string;
  error?: string;
}
const approvalOverrides = new Map<string, ApprovalOverride>();
// cancelRequested 本次运行是否由用户主动停止：complete 时不再额外提示。
let cancelRequested = false;

// ActiveInterrupt 当前挂起的审批 / 问卷定位（由 interaction.waiting 与快照 pendingCalls 给出）：
// 提交续跑时原样回传根中断 ID。
interface ActiveInterrupt {
  interruptId?: string;
  interruptIds?: string[];
  questionnaire: boolean;
}
// activeInterrupt 实时流下最近一次 interaction.waiting 的挂起定位。
let activeInterrupt: ActiveInterrupt | null = null;

// opened 双向绑定：向上透传抽屉开关状态
const opened = computed<boolean>({
  get: () => props.modelValue,
  set: (value) => emits("update:modelValue", value),
});

// ---- 抽屉宽度 ----
// 默认随视口收放，但保持在可读区间内（窄屏不小于 320px）；上限取视口的 80%，允许用户在宽屏
// 上把抽屉拉到接近全屏。用户可在左缘拖拽手动设定宽度，手动值优先于视口比例值，并在视口收窄
// 时同步收敛
const DRAWER_WIDTH_RATIO = 0.42;
const DRAWER_WIDTH_MIN = 320;
const DRAWER_WIDTH_MAX_RATIO = 0.8;

const viewportWidth = ref<number>(window.innerWidth);

// maxDrawerWidth 当前视口下允许的最大宽度：视口的 80%，且不低于最小宽度
const maxDrawerWidth = computed<number>(() =>
  Math.max(DRAWER_WIDTH_MIN, Math.round(viewportWidth.value * DRAWER_WIDTH_MAX_RATIO)),
);

// autoDrawerWidth 未手动设定时的宽度：按视口比例取宽并收敛到可读区间
const autoDrawerWidth = computed<number>(() =>
  Math.min(
    maxDrawerWidth.value,
    Math.max(DRAWER_WIDTH_MIN, Math.round(viewportWidth.value * DRAWER_WIDTH_RATIO)),
  ),
);

// manualWidth 用户拖拽设定的宽度；为 null 时回退视口比例值
const manualWidth = ref<number | null>(null);

// drawerResizing 是否正在拖拽宽度：拖拽期间关闭抽屉过渡、并跳过逐帧的提问位置重算
const drawerResizing = ref(false);

// drawerWidth 抽屉实际宽度：手动值优先于视口比例值；写入时落到手动值上（手柄组件已按 min/max 收敛）
const drawerWidth = computed<number>({
  get: () =>
    Math.min(
      maxDrawerWidth.value,
      Math.max(DRAWER_WIDTH_MIN, Math.round(manualWidth.value ?? autoDrawerWidth.value)),
    ),
  set: (value) => {
    manualWidth.value = value;
  },
});
const onViewportResize = (): void => {
  viewportWidth.value = window.innerWidth;
};

// ---- 会话表单校验 schema ----
// 重命名会话仅校验标题
const renameSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiSession.field.title"),
});
const renameForm = useForm({ validationSchema: renameSchema });
const [renameSessionTitle, renameSessionTitleProps] = renameForm.defineField("title", vuetifyCfg);

const renameSessionDialogRef = useTemplateRef("renameSessionDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const messagesRef = useTemplateRef("messagesRef");
const composerRef = useTemplateRef<{ focus: () => void }>("composerRef");

// ---- 会话与消息状态 ----
const sessions = ref<AgentSessionItem[]>([]);
// 当前会话 ID：初值取本地上次记录，使刷新后打开抽屉即恢复原会话并自动续播（见 initialize）
const activeSessionID = ref<number | null>(localManager.getAgentActiveSession());
// draftMode 待创建态：点「新建会话」已进入新会话但尚未发出首条消息；会话在首条消息发出时
// 才真正创建，主题由后端据该消息生成，故此时不存在任何会话 ID。
// 该态只影响空态提示文案——未选中会话本身（无论是否点过「新建会话」）即允许直接发送并建会话
const draftMode = ref(false);
// sessionListMode 会话列表态：由头部列表按钮切换，为 true 时用会话列表替换消息区与输入区，
// 在侧边栏内直接平铺全部会话，供逐行切换 / 重命名 / 删除
const sessionListMode = ref(false);
// renamingSessionID / deletingSessionID 当前操作的目标会话 id：列表行内的重命名 / 删除
// 按行定位，不复用 activeSessionID，避免误操作到当前选中会话之外的行
const renamingSessionID = ref<number | null>(null);
const deletingSessionID = ref<number | null>(null);

// AgentStageState 工具调用阶段视图模型：一次工具调用一行，由 tool_calls 块内的一条记录映射而来
// （同一次调用的发起与结果收敛在同一记录上），仅供既有阶段组渲染复用 stageLabelOf / stageIconOf
interface AgentStageState {
  key: string;
  stage: EvalStage;
  tool: string;
  status?: EvalStageStatus;
  elapsedMs: number;
  error?: string;
}

// PendingCallState 单条待审批的工具调用视图模型：toolName / arguments 为后端原样下发的
// 工具名与入参文本；auditLevel / approvalRequired 为后端评估与合成结果；execOwner 指明执行
// 归属（client 前端执行、backend 后端执行，前端仅审批）；decision / result / error 记录用户
// 决策与执行结果；submitted 标记是否已随本轮结果回传后端（回传后仍保留展示，直到续跑流产生输出）
interface PendingCallState {
  callID: string;
  toolName: string;
  // arguments 模型给出的调用入参（原始 JSON 文本）
  arguments: string;
  // interruptId 该调用所属的根中断 ID（动作回传时用于定位现场）
  interruptId?: string;
  // auditLevel 本次调用的审核重要级（must/suggest/auto/none），经枚举注册表渲染展示名
  auditLevel?: string;
  // approvalRequired 是否需要人工审批：由后端按重要级与会话阀门合成；为 false 时自动批准
  approvalRequired?: boolean;
  // execOwner 执行归属：client 由前端执行（默认，字段缺失时按此处理）、backend 由后端执行（前端仅审批）
  execOwner?: "client" | "backend";
  decision?: "approved" | "rejected";
  // result 执行成功时的结果文本；error 执行失败时的错误文本
  result?: string;
  error?: string;
  // executing 该调用的前端执行是否进行中（api_invoke 使用；其余 client 工具无需本地执行）
  executing?: boolean;
  submitted?: boolean;
}

// CompressState 上下文压缩的展示态（自动挂在当前助手消息、手动挂在被点击的助手消息、静默滚动
// 与其余推进一样在刷新后随历史回显挂在被折叠的最后一条消息）：
//   phase       压缩阶段（running 进行中 / done 完成 / failed 失败）
//   draft       compress_delta 累积的摘要增量（流式呈现）
//   summary     压缩完成后的摘要全文（实时取 compress_done，历史回放取压缩检查点；收起态点击展开）
//   reason      推进原因（auto 自动 / manual 手动 / rolling 静默滚动）
//   tokensBefore 推进前占用 token（实时的取自 compress_start，历史的取自压缩检查点）
//   foldedCount  本次折叠的消息条数（实时的取自 compress_done，历史的取自压缩检查点）
//   tokensAfter  压缩后占用 token（压缩完成即刻取 compress_done 的估算，随压缩检查点落库并于刷新 /
//                重进会话后随历史带回；供前后对比，下一次生成的真实 done.promptTokens 到达后校准）
interface CompressState {
  phase: "running" | "done" | "failed";
  draft: string;
  summary?: string;
  reason: "auto" | "manual" | "rolling";
  tokensBefore?: number;
  foldedCount?: number;
  tokensAfter?: number;
}

// ChatMessage 会话消息视图模型：会话历史消息与流式占位消息统一形态。消息体一律由 blocks（后端有序
// 内容块）驱动渲染——实时流经 block 事件按 seq upsert、刷新 / 重进会话由出参 blocks 恢复，同一套
// 「块 → 渲染块」映射同时服务二者；content 仅作纯文本兜底与复制 / 提问导航用，不再参与消息体渲染
interface ChatMessage {
  key: number;
  // id 服务端消息主键：历史消息自列表接口带回；实时气泡则由流内事件回填——助手消息在 done
  // （AgentOutput.messageID）或挂起 tool_request（锚点 ID）到达时绑定，故无需回读历史即可按
  // 消息 ID 发起「压缩到此」等操作。续播中断运行时就地复用该行重建本轮内容，避免与已落库的
  // 中间产物重影
  id?: number;
  role: "user" | "assistant" | "system";
  content: string;
  // createdAt 消息创建时间（ISO 文本）：历史消息取自后端，即时发出、尚未回读服务端的消息取本地时间
  createdAt?: string;
  citations?: RetrievalResult[];
  // skillID / skillName 本用户消息携带的技能（锚定在轮次而非会话）：即时气泡由发送时的选择写入，
  // 历史气泡由快照 / 消息出参回填，二者一致；用于气泡顶部展示本轮技能、回退时把技能一并回填输入区
  skillID?: number;
  skillName?: string;
  streaming?: boolean;
  // reasoningExpanded 思考折叠区块（末个 reasoning 块）当前是否展开：流式期间默认展开、结束自动折叠，
  // 用户可手动切换
  reasoningExpanded?: boolean;
  // blocks 本条助手消息的有序内容块（后端 AgentMessageBlockOut）：实时会话直接引用 store 的当前运行
  // 块列表，历史消息取自列表接口——消息体的唯一展示来源
  blocks?: AgentMessageBlockOut[];
  // pendingCalls 本次挂起、待用户审批并执行的接口调用（为空表示无待审批项）；其 callID 为提交续跑所必需
  pendingCalls?: PendingCallState[];
  // awaitingApproval 是否处于「等待审批」态（收到 tool_request 后置 true，提交回传后置 false）
  awaitingApproval?: boolean;
  // 本轮回答的用量元信息（token 累计与总耗时毫秒）：刚生成完取自 done 事件，历史消息取自列表接口；
  // 两者均为 0（用户消息 / 旧数据）时不展示
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  elapsedMs?: number;
  // compress 本次上下文压缩的展示态（自动挂在当前助手消息、手动挂在被点击的助手消息）
  compress?: CompressState;
  // phase 本条助手消息所属运行的生命周期终态阶段（done/failed/cancelled，取值来自后端枚举类
  // agentRunPhase）：为 failed/cancelled 时渲染中断提示横幅（文案由前端 i18n 提供，不落库、不混入
  // 正文）；历史消息取自列表接口，在途消息由 error 事件回填
  phase?: string;
  // turnError 本轮失败的原始错误文本（turn.error / 流错误）：随中断提示一并展示，避免只有一条泛化
  // 文案而看不出真实原因；仅本会话内存态，刷新后不回填
  turnError?: string;
}

let messageKey = 0;
const nextKey = (): number => ++messageKey;
const messages = ref<ChatMessage[]>([]);

// ---- AI 状态与档位 ----
const aiStatus = ref<AgentStatusOut | null>(null);
// slot 当前档位：初值留空，由状态接口下发的后端默认档位（参数 agent.defaultSlot）填充，
// 前端不声明档位字面量，避免与后端配置漂移
const slot = ref("");
// reviewValve 当前审核阀门：草稿态初值留空，由状态接口下发的默认阀门（defaultReviewValve）
// 填充；会话态随会话列表回显，前端不声明阀门字面量
const reviewValve = ref("");

// thinking 思考开关：用户跨会话偏好，初值取本地持久化（默认关闭），切换即写回本地记忆
const thinking = ref<boolean>(localManager.getAgentThinking());
watch(thinking, (value) => {
  localManager.setAgentThinking(value);
});

// aiEnabled AI 能力是否可用；状态未加载完成时不禁用操作，由后端错误兜底
const aiEnabled = computed<boolean>(() => (aiStatus.value ? aiStatus.value.enabled : true));

// reviewValveIcons 审核阀门取值 → 关闭态图标：以同一「盾牌」族的三种修形区分把关强度；
// 未登记取值回退通用盾牌图标（前端不据此推断业务规则，仅作视觉标识）
const reviewValveIcons: Record<string, string> = {
  each: "mdi-shield-account-outline",
  auto: "mdi-shield-check-outline",
  none: "mdi-shield-off-outline",
};

// reviewValveHintOf 阀门说明文本：按枚举条目 key 定位本端 i18n；缺失时返回空串（说明属补充信息，不占位）
const reviewValveHintOf = (key: string): string => {
  const path = `page.aiAssistant.reviewValveHint.${key}`;
  const text = i18n.global.t(path);
  return text === path ? "" : text;
};

// reviewValveOptions 审核阀门下拉选项：取值与 label 由后端枚举类 reviewValve 驱动；
// 枚举契约不含说明与图标，故 subtitle 由本端 i18n 按 key 补齐、icon 由本端取值映射补齐
const reviewValveOptions = computed(() =>
  (enumEntriesOf(EnumClass.reviewValve) ?? []).map((e) => ({
    title: e.label,
    value: e.value,
    subtitle: reviewValveHintOf(e.key),
    icon: reviewValveIcons[e.value] ?? "mdi-shield-outline",
  })),
);

// reviewValveSelected 当前选中的枚举条目，供关闭态图标与悬浮提示解析
const reviewValveSelected = computed(() =>
  (enumEntriesOf(EnumClass.reviewValve) ?? []).find((e) => e.value === reviewValve.value),
);

// reviewValveIcon 关闭态图标：未选中或未登记取值时回退通用盾牌图标
const reviewValveIcon = computed(() => reviewValveIcons[reviewValve.value] ?? "mdi-shield-outline");

// reviewValveTooltip 关闭态仅展示图标，以悬浮提示回看当前取值名称与说明
const reviewValveTooltip = computed(() => {
  const name = i18n.global.t("page.aiAssistant.reviewValve");
  const selected = reviewValveSelected.value;
  if (!selected) {
    return name;
  }
  const hint = reviewValveHintOf(selected.key);
  return hint ? `${name}：${selected.label} · ${hint}` : `${name}：${selected.label}`;
});

// slotOptions 档位下拉选项：仅展示对话档位，不可用档位禁选
const slotOptions = computed(() =>
  (aiStatus.value?.slots ?? [])
    .filter((s) => s.kind === "chat")
    .map((s) => ({
      title: s.modelName
        ? `${enumLabelOf(EnumClass.llmSlot, s.slot)} · ${s.modelName}`
        : enumLabelOf(EnumClass.llmSlot, s.slot),
      value: s.slot,
      props: { disabled: !s.available },
    })),
);

// slotFullTitle 当前档位的完整描述（档位 · 模型）：关闭态直接沿用下拉项标题展示，
// 这里同时作为悬浮提示，长模型名被截断时可回看完整取值
const slotFullTitle = computed(
  () => slotOptions.value.find((o) => o.value === slot.value)?.title ?? "",
);

// slotThinkingToggle 当前档位绑定模型是否支持思考开关：思考字段与开 / 关取值三者齐备才为 true。
// 为 false 时思考控件整块隐藏（该模型走自身默认思考行为），且发送时不注入 thinking 字段，
// 从而保持模型的默认思考模式、不强行干预
const slotThinkingToggle = computed<boolean>(
  () => (aiStatus.value?.slots ?? []).find((s) => s.slot === slot.value)?.thinkingToggle === true,
);

// thinkingTooltip 思考开关悬浮提示：控件仅在支持思考开关时才渲染，故恒为开关说明
const thinkingTooltip = computed<string>(
  () =>
    `${i18n.global.t("page.aiAssistant.thinking")}：${i18n.global.t("page.aiAssistant.thinkingHint")}`,
);

// REASONING_PANEL 思考过程折叠面板的取值（手风琴单值 v-model）：展开时取该值，收起时取空串
const REASONING_PANEL = "reasoning";

// REASONING_NOTE_MAX_WIDTH 思考来源说明 tooltip 的最大宽度（px）：说明含「原因 + 如何控制」两段
// 文案，需折成多行才易读。Vuetify 以行内样式把该值落到 .v-overlay__content（同 NavBadge 的
// :max-width 手法），故只能用 :max-width 属性而非 CSS 类覆盖；取 320 约为助手抽屉宽度的
// 三分之二，可折成 2~4 行且不溢出抽屉
const REASONING_NOTE_MAX_WIDTH = 320;

// setReasoningExpanded 同步思考过程折叠区块的展开态：用户手动切换时按面板取值回写
const setReasoningExpanded = (m: ChatMessage, value: unknown): void => {
  m.reasoningExpanded = value === REASONING_PANEL;
};

// ---- 上下文占用指示 ----
// contextUsedTokens 当前会话占用的 token：直接取会话列表项给出的展示占用（后端 ContextUsage：
// 末次真实测量与更晚的压缩后估算中更晚者），故压缩完成即刻即降至压缩后的估算值，刷新 / 重进会话
// 后仍保持该口径；会话随后任一生成的真实测量落定后，后端即改由真实口径给出。草稿态无会话为 0
const contextUsedTokens = computed<number>(() => {
  const id = activeSessionID.value;
  if (id === null) {
    return 0;
  }
  return sessions.value.find((s) => s.id === id)?.contextTokens ?? 0;
});

// headerTitle 头部标题：进入某个会话后展示该会话主题；会话尚无主题（主题由首条消息发出后
// 由后端生成）时回落为「新会话」；未选中任何会话且非待创建态时保持原「AI 助手」标题
const headerTitle = computed<string>(() => {
  if (activeSessionID.value === null) {
    return draftMode.value
      ? i18n.global.t("page.aiAssistant.untitledSession")
      : i18n.global.t("action.aiAssistant");
  }
  const session = sessions.value.find((s) => s.id === activeSessionID.value);
  return session?.title?.trim() || i18n.global.t("page.aiAssistant.untitledSession");
});

// contextLimitTokens 当前所选档位的上下文容量：取自已加载的档位状态（按 slot 匹配），
// 切换档位时由此 computed 自动重算，不发起任何额外请求；≤0 表示该档位未配置容量
const contextLimitTokens = computed<number>(
  () => (aiStatus.value?.slots ?? []).find((s) => s.slot === slot.value)?.contextTokenLimit ?? 0,
);

// contextUsageVisible 容量未配置（≤0）时整块指示隐藏
const contextUsageVisible = computed<boolean>(() => contextLimitTokens.value > 0);

// contextUsagePercent 占用比例（取整）；容量未配置时为 0（不展示）
const contextUsagePercent = computed<number>(() =>
  contextLimitTokens.value > 0
    ? Math.round((contextUsedTokens.value / contextLimitTokens.value) * 100)
    : 0,
);

// contextOverThreshold 占用是否已达自动压缩阈值：达阈值时指示转警示色
const contextOverThreshold = computed<boolean>(() => {
  const limit = contextLimitTokens.value;
  if (limit <= 0) {
    return false;
  }
  const ratio = aiStatus.value?.compactThresholdRatio ?? Number.POSITIVE_INFINITY;
  return contextUsedTokens.value / limit >= ratio;
});

// contextUsageColor 占用指示主题色：达阈值取警示色，否则取弱化色；环与百分比文字共用，保持同色
const contextUsageColor = computed<string>(() =>
  contextOverThreshold.value ? "warning" : "on-surface-variant",
);

// loadStatus 拉取 AI 状态：驱动档位下拉选项与未启用原因展示
const loadStatus = async (): Promise<void> => {
  await getResponse<AgentStatusOut>(
    agentStatusGet(),
    async (rsp) => {
      aiStatus.value = rsp.data;
      // 未选择档位时取后端下发的默认档位（参数 agent.defaultSlot）
      if (!slot.value) {
        slot.value = rsp.data.defaultSlot;
      }
      // 草稿态未选择审核阀门时取后端下发的默认阀门
      if (!reviewValve.value) {
        reviewValve.value = rsp.data.defaultReviewValve;
      }
      // 当前选中档位不可用时回落到首个可用对话档位
      const available = (rsp.data.slots ?? []).filter((s) => s.kind === "chat" && s.available);
      const current = (rsp.data.slots ?? []).find((s) => s.slot === slot.value);
      if (available.length > 0 && current?.available !== true) {
        slot.value = available[0].slot;
      }
    },
    async () => {
      aiStatus.value = null;
    },
    notifyNoResponse,
  );
};

// citationsOf 归一化引用列表（后端消息可能省略该字段）
const citationsOf = (m: ChatMessage): RetrievalResult[] => m.citations ?? [];

// CITATION_VISIBLE_LIMIT 引用默认展示条数：超出部分收起，由用户按需展开
const CITATION_VISIBLE_LIMIT = 3;

// expandedCitationKeys 已展开全部引用的消息 key 集合（按消息维度独立记忆，切换会话不清空）
const expandedCitationKeys = ref(new Set<number>());

const citationsExpandedOf = (m: ChatMessage): boolean => expandedCitationKeys.value.has(m.key);

// visibleCitations 当前应展示的引用：未展开时仅取前 CITATION_VISIBLE_LIMIT 条
const visibleCitations = (m: ChatMessage): RetrievalResult[] => {
  const list = citationsOf(m);
  return citationsExpandedOf(m) ? list : list.slice(0, CITATION_VISIBLE_LIMIT);
};

// toggleCitations 展开 / 收起当前消息的引用的其余条目
const toggleCitations = (m: ChatMessage): void => {
  if (expandedCitationKeys.value.has(m.key)) {
    expandedCitationKeys.value.delete(m.key);
    return;
  }
  expandedCitationKeys.value.add(m.key);
};

// ---- 工具调用阶段 ----
// stageLabelOf 阶段展示名（经枚举注册表 evalStage 按当前语言渲染 label）
const stageLabelOf = (stage: EvalStage): string => enumLabelOf(EnumClass.evalStage, stage);

// stageIconOf 阶段状态图标：运行中旋转、完成打勾、失败告警；流已结束仍在进行的阶段展示为已中止
const stageIconOf = (stage: AgentStageState, running: boolean): string => {
  if (stage.status === "done") {
    return "mdi-check-circle-outline";
  }
  if (stage.status === "failed") {
    return "mdi-alert-circle-outline";
  }
  return running ? "mdi-loading" : "mdi-stop-circle-outline";
};

// stageColorOf 阶段状态语义色
const stageColorOf = (stage: AgentStageState, running: boolean): string => {
  if (stage.status === "done") {
    return "success";
  }
  if (stage.status === "failed") {
    return "warning";
  }
  return running ? "primary" : "warning";
};

// stageMetaTextOf 阶段元信息：耗时与失败原因
const stageMetaTextOf = (stage: AgentStageState): string => {
  const parts: string[] = [];
  if (stage.elapsedMs > 0) {
    parts.push(`${stage.elapsedMs} ms`);
  }
  if (stage.error) {
    parts.push(`${i18n.global.t("page.aiAssistant.stageFailed")}: ${stage.error}`);
  }
  return parts.join(" · ");
};

// stageRowsOf 把一条 tool_calls 块载荷映射为阶段行（复用既有阶段渲染）：问卷类调用（ask_user）豁免
// 工具行（问卷面板即其呈现），故从可见行中剔除；记录状态 start 归入发起（toolCall）、其余归入结果
// （toolResult），以复用既有阶段枚举文案
const stageRowsOf = (payload: AgentToolCallsBlockPayload): AgentStageState[] =>
  (payload.calls ?? [])
    .filter((c) => c.toolName !== ASK_USER_TOOL_NAME)
    .map((c) => ({
      key: c.callID,
      stage: c.status === "start" ? "toolCall" : "toolResult",
      tool: c.toolName,
      status: c.status,
      elapsedMs: c.elapsedMs,
      error: c.error,
    }));

// awaitingFirstResponse 判断该助手消息是否仍处于「请求已发出、尚无任何产出」的等待首包阶段：
// 流式已开始但尚无任何内容块到达（含模型首包前的长等待与被中断前的空档）。此阶段以「等待响应中…」
// 替代「生成中…」，避免长等待被误读为服务异常而触发刷新或重复提交
const awaitingFirstResponse = (m: ChatMessage): boolean =>
  m.streaming === true && (m.blocks?.length ?? 0) === 0;

// reasoningStreamingOf 思考折叠区块是否仍在流式生成：消息仍在流式且末个块仍为 reasoning 块
// （新块（正文 / 工具调用 / 交互）一开即收敛该思考块，故末块一变即视为思考已停）
const reasoningStreamingOf = (m: ChatMessage): boolean => {
  const blocks = m.blocks ?? [];
  const last = blocks[blocks.length - 1];
  return m.streaming === true && last?.kind === "reasoning";
};

// endReasoningStreamingOf 结束思考流式：收起末个思考折叠区块（保留已累积内容）。思考是否仍在流式由
// reasoningStreamingOf 据块序列派生，故此处只需收敛展开态——done / 挂起 / 失败 / 中断时调用
const endReasoningStreamingOf = (m: ChatMessage): void => {
  m.reasoningExpanded = false;
};

// FOLLOW_BOTTOM_THRESHOLD 跟随贴底的容差：距底部超过该值即视为用户已主动上翻
const FOLLOW_BOTTOM_THRESHOLD = 120;

// scrollToBottom 贴底。force=false 时仅在用户已处于底部附近才跟随——流式增量每来一段都会调用，
// 若无条件贴底，用户上翻阅读或按块跳读时会被立刻拽回底部（表现为「点了向上箭头却不动」）
const scrollToBottom = (force = true): void => {
  void nextTick(() => {
    const el = messagesRef.value;
    if (!el) {
      return;
    }
    if (!force && el.scrollHeight - el.scrollTop - el.clientHeight > FOLLOW_BOTTOM_THRESHOLD) {
      return;
    }
    // 瞬间贴底：滚动容器开着 scroll-behavior: smooth，而抽屉开合期内容区被整体移出视口，
    // 浏览器会丢弃此时的平滑滚动（表现为打开抽屉后停在顶部 / 中途）；临时改回 auto 再赋值，
    // 跳转类交互（提问轨 / 块跳读）仍由 CSS 的 smooth 提供动效
    const prev = el.style.scrollBehavior;
    el.style.scrollBehavior = "auto";
    el.scrollTop = el.scrollHeight;
    el.style.scrollBehavior = prev;
  });
};

// reasoningBodyRefs 各条助手消息的「思考正文」滚动区（键为消息 key）：思考正文自带限高滚动，
// 消息区的 scrollToBottom 到不了它内部。与 turnRefs 同法——元素卸载时 Vue 以 null 回调，同步剔除
const reasoningBodyRefs = new Map<number, HTMLElement>();
const setReasoningBodyRef = (key: number, el: unknown): void => {
  if (el instanceof HTMLElement) {
    reasoningBodyRefs.set(key, el);
  } else {
    reasoningBodyRefs.delete(key);
  }
};

// scrollReasoningToBottom 把某条消息的思考正文粘到底部：内容流式增长超过限高后新内容追加在
// 可视区下方，滚动位置不动就会一直停在旧位置、最新思考看不见；每来一段增量即重新贴底。
// 同 scrollToBottom 延后到 nextTick（须待增量落到 DOM 再测量）；面板收起时元素不在，静默跳过
const scrollReasoningToBottom = (key: number): void => {
  void nextTick(() => {
    const el = reasoningBodyRefs.get(key);
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  });
};

// ---- 提问导航（消息区左侧快速回顾）----
// TURN_SCROLL_GAP 跳转后提问轮顶边与消息区上沿的间距。JS 无法读取 CSS 令牌，
// 故在此声明常量（同 ScrollToBottomFab 的 FAB_SIZE / MARGIN 先例）；取 --zk-space-4(16px)，
// 扣除滚动区自带的 12px 上内边距后仍留 4px 呼吸量
const TURN_SCROLL_GAP = 16;
// ACTIVE_TOP_EPSILON 判定「提问已进入视口」的容差：顶边越过上沿该像素数即视为当前提问。
// 必须 ≥ TURN_SCROLL_GAP：跳转后目标轮顶边停在「上沿 + TURN_SCROLL_GAP」处（见 scrollToTurn），
// 若容差小于该间距，刚跳到的这一轮永远不满足条件，高亮会错落到前一轮
// （点第 1 个不高亮、点第 2 个却亮第 1 个）。多出的 8px 为亚像素与平滑滚动终值的余量
const ACTIVE_TOP_EPSILON = TURN_SCROLL_GAP + 8;

// turnRefs 各轮消息的 DOM 元素（键为消息 key）：供提问轨定位滚动与判定当前提问。
// 轮卸载（删除 / 回退）时 Vue 以 null 回调，此处同步剔除，避免命中已卸载节点
const turnRefs = new Map<number, HTMLElement>();
const setTurnRef = (key: number, el: unknown): void => {
  if (el instanceof HTMLElement) {
    turnRefs.set(key, el);
  } else {
    turnRefs.delete(key);
  }
};

// userQuestions 提问轨条目：仅取用户消息，正文归一化空白后作为标题（同时用作悬浮提示与无障碍名称）
const userQuestions = computed<{ key: number; text: string }[]>(() =>
  messages.value
    .filter((m) => m.role === "user")
    .map((m) => ({ key: m.key, text: m.content.replace(/\s+/g, " ").trim() })),
);

// activeQuestionKey 当前视口内的提问 key：轨上的圆点与面板行据此高亮，无匹配时为 undefined
const activeQuestionKey = ref<number | undefined>(undefined);

// messagesAtBottom 消息区是否已滚到底部：提问轨据此决定是否展示「直达底部」按钮。
// 初值取 true——内容不足以滚动（本就无跳转目标）是最常见的首帧状态，避免按钮闪一下再消失
const messagesAtBottom = ref(true);

// updateActiveQuestion 按滚动位置重算当前提问：取最后一个顶边未越过视口上沿的提问。
// 位置用 rect 差值现算（不读 offsetTop，也不依赖 offsetParent），流式期间轮高逐帧变化，
// 故不缓存；提问数有限，扫描开销可忽略
const updateActiveQuestion = (): void => {
  // 块跳读的当前位置与提问高亮共用一次滚动回调：两者都按「最后一个越过视口上沿的锚点」推导
  updateBlockNav();
  const scroller = messagesRef.value;
  if (!scroller) {
    return;
  }
  const top = scroller.getBoundingClientRect().top;
  // 已滚到底部（且确实存在可滚动余量）时，视为当前提问为最后一个：末轮下方的内容不足以
  // 把它的顶边推到激活线，若不作此兜底，点最后一个圆点会永远判定不上、高亮停在前一轮
  const scrollable = scroller.scrollHeight - scroller.clientHeight > 1;
  const atBottom =
    scrollable &&
    scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - ACTIVE_TOP_EPSILON;
  // 无滚动余量时同样视作「已在底部」：内容一屏放得下，本就没有可跳转的下方
  messagesAtBottom.value =
    !scrollable ||
    scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - ACTIVE_TOP_EPSILON;
  if (atBottom) {
    const last = userQuestions.value[userQuestions.value.length - 1];
    if (last) {
      activeQuestionKey.value = last.key;
      return;
    }
  }
  let active: number | undefined;
  for (const q of userQuestions.value) {
    const turn = turnRefs.get(q.key);
    if (turn && turn.getBoundingClientRect().top - top <= ACTIVE_TOP_EPSILON) {
      active = q.key;
    }
  }
  activeQuestionKey.value = active;
};

// scrollToTurn 平滑滚动到指定提问所在轮：目标位置为「该轮相对滚动区顶边的位移 + 当前
// scrollTop」，即该轮顶边落到视口上沿下 TURN_SCROLL_GAP 处；滚动动效由 .assistant-messages
// 的 scroll-behavior: smooth 提供。先 nextTick 以确保目标轮已挂载并注册
const scrollToTurn = async (key: number): Promise<void> => {
  // 先置高亮：平滑滚动期间 scroll 事件会按中间位置反复重算，最终虽会收敛到同一 key，
  // 但立即置位可让被点的圆点在点击瞬间就变蓝，不必等动画结束
  activeQuestionKey.value = key;
  await nextTick();
  const scroller = messagesRef.value;
  const turn = turnRefs.get(key);
  if (!scroller || !turn) {
    return;
  }
  const offset = turn.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
  scroller.scrollTop = Math.max(0, scroller.scrollTop + offset - TURN_SCROLL_GAP);
};

// questionPanelMaxWidth 提问面板最大宽度：按抽屉宽度收敛并预留右侧余量，
// 避免浮层越过抽屉右缘触发 Vuetify 翻边（翻到页面正文上）
const questionPanelMaxWidth = computed<number>(() => Math.max(200, drawerWidth.value - 48));

// ---- 块间快速跳读 ----
// 助手回答由多个块组成（思考 / 工具调用 / 审批 / 问卷 / 正文 / 下一步 / 引用），长回答难以按块定位。
// 这里以滚动区内的块起点（模板标记的 data-block-key）为锚，配合右缘的跳读控件上下逐块移动。
// BLOCK_SCROLL_GAP 跳转后块顶边与消息区上沿的间距（同 TURN_SCROLL_GAP：JS 无法读取 CSS 令牌）
const BLOCK_SCROLL_GAP = 12;
// ACTIVE_BLOCK_EPSILON 判定「块已进入视口」的容差：须 ≥ BLOCK_SCROLL_GAP，否则刚跳到的块不被认作当前块
const ACTIVE_BLOCK_EPSILON = BLOCK_SCROLL_GAP + 8;
// BLOCK_UP_MIN_STEP 向上跳转的最小位移：目标块开头须比视口上沿至少高出该值。末块常只比上沿
// 高出十几像素，若按「块顶是否贴顶」判定，向上只会挪几像素，视觉上等同无反应
const BLOCK_UP_MIN_STEP = 24;

const blockNavCurrent = ref(0);
const blockNavTotal = ref(0);
// blockNavCanPrev 「上一块」是否还有向上目标：当前块之前还有块、首块开头仍在视口上方且位移足够，
// 或还有上一条回答（到达首块后继续向上衔接到上一轮末块）
const blockNavCanPrev = ref(false);
// blockNavCanNext 「下一块」是否还有向下目标：当前块之后还有块，或还有下一条回答（到达末块后继续
// 向下衔接到下一轮首块）
const blockNavCanNext = ref(false);
// blockNavVisible 只要存在块就展示（单个长回答同样可能需要逐块定位，不要求跨轮 / 多块）；
// 会话列表模式隐藏。注意不要把「当前是否可滚动」纳入条件：控件浮动但显隐会改变可用区域，
// 内容恰好临界时会出现抖动
const blockNavVisible = computed(() => !sessionListMode.value && blockNavTotal.value >= 1);

// blockNavMessageKey 块跳读的作用域：当前视口所在的助手回答（一条消息）。块跳读在该回答内按块
// 移动；到达首块 / 末块后继续沿方向衔接到相邻回答（见 scrollToBlock），计数仍只统计当前回答
let blockNavMessageKey: number | null = null;

// blockElementsByMessage 一次 DOM 查询聚合全部块锚点：按 data-block-key 前缀（消息 key）归组，
// 组间保持文档顺序。块随流式输出增删、缓存会在重建后失效，故每次现查
const blockElementsByMessage = (): Map<number, HTMLElement[]> => {
  const grouped = new Map<number, HTMLElement[]>();
  const scroller = messagesRef.value;
  if (!scroller) {
    return grouped;
  }
  scroller.querySelectorAll<HTMLElement>("[data-block-key]").forEach((el) => {
    const key = Number((el.dataset.blockKey ?? "").split(":")[0]);
    if (!Number.isFinite(key)) {
      return;
    }
    const list = grouped.get(key);
    if (list) {
      list.push(el);
    } else {
      grouped.set(key, [el]);
    }
  });
  return grouped;
};

// activeAssistantMessageKey 取当前视口所在的助手回答：最后一条顶边未越过视口上沿的、含块锚点的
// 助手消息；视口仍在其上方（如停在用户提问处）时取第一条（跳转即进入该回答）；滚到底部时取最后一条
const activeAssistantMessageKey = (scroller: HTMLElement, keys: number[]): number | null => {
  if (keys.length === 0) {
    return null;
  }
  const scrollable = scroller.scrollHeight - scroller.clientHeight > 1;
  if (
    scrollable &&
    scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - ACTIVE_BLOCK_EPSILON
  ) {
    return keys[keys.length - 1];
  }
  const top = scroller.getBoundingClientRect().top;
  let active = keys[0];
  for (const key of keys) {
    const el = turnRefs.get(key);
    if (!el) {
      continue;
    }
    if (el.getBoundingClientRect().top - top > ACTIVE_BLOCK_EPSILON) {
      break;
    }
    active = key;
  }
  return active;
};

// blockElements 取当前作用域回答内的块起点元素（模板为每个块根节点标了 data-block-key）
const blockElements = (grouped: Map<number, HTMLElement[]>): HTMLElement[] =>
  blockNavMessageKey === null ? [] : (grouped.get(blockNavMessageKey) ?? []);

// refreshBlockNavFlags 依据作用域内块的位置与相邻回答，决定两端箭头是否可用：末块之后还有下一条
// 回答时仍允许向下（衔接到下一轮首块），首块之前同理，避免在一轮底部 / 顶部把按钮置灰
const refreshBlockNavFlags = (
  keys: number[],
  els: HTMLElement[],
  current: number,
  scroller: HTMLElement,
): void => {
  const index = blockNavMessageKey === null ? -1 : keys.indexOf(blockNavMessageKey);
  const firstTop = els[0].getBoundingClientRect().top - scroller.getBoundingClientRect().top;
  blockNavCanPrev.value = current > 0 || firstTop <= -BLOCK_UP_MIN_STEP || index > 0;
  blockNavCanNext.value = current < els.length - 1 || (index >= 0 && index < keys.length - 1);
};

// updateBlockNav 重算作用域回答、块总数与当前块：当前块取最后一个顶边未越过视口上沿的块
const updateBlockNav = (): void => {
  const scroller = messagesRef.value;
  if (!scroller) {
    blockNavMessageKey = null;
    blockNavTotal.value = 0;
    blockNavCurrent.value = 0;
    blockNavCanPrev.value = false;
    blockNavCanNext.value = false;
    return;
  }
  const grouped = blockElementsByMessage();
  const keys = Array.from(grouped.keys());
  blockNavMessageKey = activeAssistantMessageKey(scroller, keys);
  const els = blockElements(grouped);
  blockNavTotal.value = els.length;
  if (els.length === 0) {
    blockNavCurrent.value = 0;
    blockNavCanPrev.value = false;
    blockNavCanNext.value = false;
    return;
  }
  // 已滚到底部时直接取最后一块：末块可能较短，其顶边到不了视口上沿，仅按顶边判定会停在
  // 倒数第二块（「13/14」而非「14/14」）
  const scrollable = scroller.scrollHeight - scroller.clientHeight > 1;
  if (
    scrollable &&
    scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - ACTIVE_BLOCK_EPSILON
  ) {
    blockNavCurrent.value = els.length - 1;
    refreshBlockNavFlags(keys, els, els.length - 1, scroller);
    return;
  }
  const top = scroller.getBoundingClientRect().top;
  let current = 0;
  for (let i = 0; i < els.length; i++) {
    if (els[i].getBoundingClientRect().top - top > ACTIVE_BLOCK_EPSILON) {
      break;
    }
    current = i;
  }
  blockNavCurrent.value = current;
  refreshBlockNavFlags(keys, els, current, scroller);
};

// scrollToBlock 跳到上一个 / 下一个块的开头（dir 为 -1 / 1）。到达本轮首块 / 末块后继续沿方向
// 衔接到相邻回答：末块再向下即进入下一轮回答的首块，首块再向上即回到上一轮回答的末块
const scrollToBlock = (dir: -1 | 1): void => {
  const scroller = messagesRef.value;
  if (!scroller) {
    return;
  }
  // 先刷新作用域与当前块，再取该回答内的块元素：两者必须同一口径（点击时视口仍在同一回答内，
  // 刷新通常不换作用域，但顺序一致才不会有元素与下标错配）
  updateBlockNav();
  const grouped = blockElementsByMessage();
  const keys = Array.from(grouped.keys());
  const scopeKey = blockNavMessageKey;
  if (scopeKey === null) {
    return;
  }
  let els = grouped.get(scopeKey) ?? [];
  if (els.length === 0) {
    return;
  }
  const top = scroller.getBoundingClientRect().top;
  const current = blockNavCurrent.value;
  const topOf = (i: number): number => els[i].getBoundingClientRect().top - top;
  const scopeIndex = keys.indexOf(scopeKey);
  let target: number;
  let targetKey = scopeKey;
  if (dir > 0) {
    target = current + 1;
    if (target >= els.length) {
      // 本轮末块再向下：衔接到下一轮回答的首块（没有下一轮时按钮为置灰态）
      const nextKey = scopeIndex >= 0 ? keys[scopeIndex + 1] : undefined;
      if (nextKey === undefined) {
        return;
      }
      targetKey = nextKey;
      els = grouped.get(nextKey) ?? [];
      if (els.length === 0) {
        return;
      }
      target = 0;
    }
  } else {
    // 向上取「离视口上沿最近、且已越过激活线」的块起点。判据是位移量而非块顶是否贴顶：
    // 停在该块中段（块顶远在上沿之上）时回到本块开头；块顶离上沿不足 BLOCK_UP_MIN_STEP 时
    // 再往前一块——滚到底部时末块常常只比上沿高出十几像素，按「贴顶」判定就会只移动几像素，
    // 视觉上等同没反应
    target = current;
    while (target > 0 && topOf(target) > -BLOCK_UP_MIN_STEP) {
      target--;
    }
    if (target === 0 && topOf(0) > -BLOCK_UP_MIN_STEP) {
      // 本轮首块再向上：衔接到上一轮回答的末块（没有上一轮时按钮为置灰态）
      const prevKey = scopeIndex > 0 ? keys[scopeIndex - 1] : undefined;
      if (prevKey === undefined) {
        return;
      }
      targetKey = prevKey;
      els = grouped.get(prevKey) ?? [];
      if (els.length === 0) {
        return;
      }
      target = els.length - 1;
    }
  }
  const clamped = Math.min(els.length - 1, Math.max(0, target));
  const offset = els[clamped].getBoundingClientRect().top - top;
  // 逐块跳读用瞬时滚动：容器开着 scroll-behavior: smooth，但平滑滚动在内容区被移出视口（抽屉
  // 开合过渡）或与流式贴底交替时会整段被浏览器丢弃，表现为点了箭头却不动；此处临时改回 auto
  const prev = scroller.style.scrollBehavior;
  scroller.style.scrollBehavior = "auto";
  scroller.scrollTop = Math.max(0, scroller.scrollTop + offset - BLOCK_SCROLL_GAP);
  scroller.style.scrollBehavior = prev;
  blockNavMessageKey = targetKey;
  blockNavTotal.value = els.length;
  blockNavCurrent.value = clamped;
  refreshBlockNavFlags(keys, els, clamped, scroller);
};

// 流式输出会增删块但消息条数不变，用「消息 × 块数」签名驱动重算
const blockNavSignature = computed(() =>
  messages.value.map((m) => `${m.key}:${m.blocks?.length ?? 0}`).join("|"),
);
watch(blockNavSignature, () => void nextTick(updateActiveQuestion));

// 消息区首帧挂载、消息条数变化、抽屉宽度变化都会改变各问位置，需重算当前提问
watch(messagesRef, () => void nextTick(updateActiveQuestion));
watch(
  () => messages.value.length,
  () => void nextTick(updateActiveQuestion),
);
// 拖拽期间每帧宽度变化都会触发一次强制布局（updateActiveQuestion 需要读取各轮 rect），
// 是「拖动不跟手」的主因之一；拖拽中跳过，结束后由 onDrawerResizeEnd 统一重算一次
watch(drawerWidth, () => {
  if (drawerResizing.value) {
    return;
  }
  void nextTick(updateActiveQuestion);
});

// onDrawerResizeEnd 拖拽结束：恢复抽屉过渡并统一重算提问位置
const onDrawerResizeEnd = (): void => {
  drawerResizing.value = false;
  void nextTick(updateActiveQuestion);
};

// ---- 会话加载 ----
const notifyError = (text: string): void => {
  appStore.notify({ text, timeout: 3000, color: "error" });
};

// notifyNoResponse 请求已发出却未取得响应（超时 / 网络异常）时的统一提示：
// getResponse 不传该回调会静默吞掉，用户看不到任何反馈
const notifyNoResponse = async (): Promise<void> => {
  notifyError(i18n.global.t("page.aiAssistant.noResponse"));
};

// applySessionSnapshots 用会话列表出参回填各会话的交互状态快照（interactionState/substate/locked/
// userTurns/maxUserTurns）：刷新 / 切会话后据此恢复输入闸门与锁定提示（无在途运行时以列表为权威）；
// 当前会话若有在途活动（运行中 / 等待中），则以实时 state 事件为准，不用列表覆盖，避免中途被冲淡
const applySessionSnapshots = (list: AgentSessionItem[]): void => {
  for (const s of list) {
    if (agentRun.hasActivityOf(s.id)) {
      continue;
    }
    agentRun.setSnapshot(s.id, {
      state: s.interactionState,
      substate: s.interactionSubstate,
      locked: s.locked,
      userTurns: s.userTurns,
      maxUserTurns: s.maxUserTurns,
    });
  }
};

const loadSessions = async (): Promise<void> => {
  await getResponse<AgentSessionItem[]>(
    agentSessionListGet(),
    async (rsp) => {
      sessions.value = rsp.data ?? [];
      // 用列表出参回填各会话交互状态快照（当前会话在途时跳过，见 applySessionSnapshots）
      applySessionSnapshots(sessions.value);
      // 当前选中会话若已被删除则清空选中态
      if (
        activeSessionID.value !== null &&
        !sessions.value.some((s) => s.id === activeSessionID.value)
      ) {
        activeSessionID.value = null;
        messages.value = [];
      }
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// ---- 消息模型辅助 ----
// approvalOverrideOf 用本地审批收敛态回填调用视图态：决策优先取本地快照（服务端不回写已决策
// 状态），结果 / 错误同理，避免消息重建把已决策状态回退成待审批。
const approvalOverrideOf = (call: PendingCallState): PendingCallState => {
  const override = approvalOverrides.get(call.callID);
  if (!override) {
    return call;
  }
  return {
    ...call,
    decision: call.decision ?? override.decision,
    result: call.result ?? override.result,
    error: call.error ?? override.error,
  };
};

// mergePendingCalls 合并新解析的调用与既有视图态：仅当仍属同一个挂起点（中断 ID 相同）时保留本地
// 决策 / 执行结果 / 提交标记——续跑后模型再次发起同类调用会产生新的挂起点，若继续沿用旧决策，
// 新调用会被误显示为「已批准」而没有批准 / 拒绝按钮，提交也会因旧标记被跳过，界面停在「等待授权」。
const mergePendingCalls = (
  parsed: PendingCallState[],
  existing: PendingCallState[] | undefined,
): PendingCallState[] => {
  const old = new Map((existing ?? []).map((call) => [call.callID, call]));
  return parsed.map((call) => {
    const prev = old.get(call.callID);
    if (!prev || prev.interruptId !== call.interruptId) {
      return call;
    }
    return {
      ...call,
      decision: call.decision ?? prev.decision,
      result: call.result ?? prev.result,
      error: call.error ?? prev.error,
      executing: prev.executing,
      submitted: prev.submitted,
    };
  });
};

// applyQuestionnaireOverrides 应用问卷的本地收敛态（提交 / 取消后服务端不回推块更新）。
const applyQuestionnaireOverrides = (blocks: AgentMessageBlockOut[]): void => {
  for (const block of blocks) {
    if (block.kind !== "questionnaire") {
      continue;
    }
    const override = questionnaireOverrides.get(block.seq - 1);
    if (!override) {
      continue;
    }
    const payload = block.payload as AgentQuestionnaireBlockPayload;
    payload.awaiting = override.awaiting;
    payload.answers = override.answers;
  }
};

// applyApprovalDecisions 把本地决策写入审批块载荷，使面板徽章即时反映批准 / 拒绝。
const applyApprovalDecisions = (
  blocks: AgentMessageBlockOut[],
  calls: PendingCallState[] | undefined,
): void => {
  if (!calls || calls.length === 0) {
    return;
  }
  const byCallID = new Map(calls.map((call) => [call.callID, call]));
  for (const block of blocks) {
    if (block.kind !== "approval") {
      continue;
    }
    const records = (block.payload as AgentApprovalBlockPayload)?.calls ?? [];
    for (const record of records) {
      const call = byCallID.get(record.callID);
      if (!call) {
        continue;
      }
      if (call.decision) {
        record.decision = call.decision === "approved";
      }
      record.result = call.result;
      record.error = call.error;
    }
  }
};

// loadHistory 通过 /agent/:sessionID/render 读取历史快照与在途缓冲：snapshot.message 直接生成
// 消息与终态块；快照之后的实时事件由同一回调应用，保证刷新 / 重进与实时终态一致。
const loadHistory = async (id: number): Promise<void> => {
  agentRun.detach(id);
  activeAssistantKey = null;
  pendingUserEcho = null;
  activeInterrupt = null;
  questionnaireOverrides.clear();
  approvalOverrides.clear();
  messages.value = [];
  // 每次 render 的实时事件游标都从 1 重新计数：先归零续播游标，快照事件不携带 id。
  agentRun.resetSeq(id);
  // render 是一次性回放，EOF 后同样释放句柄：否则等待审批期间（state=waiting）点击批准时，
  // 下一条 /action 的 attach 会 abort 这条早已完成的 /render，控制台出现 net::ERR_ABORTED。
  let abort: (() => void) | null = null;
  const releaseHistoryStream = (): void => {
    historyLoading = false;
    if (abort) {
      agentRun.release(id, abort);
    }
  };
  let retried = false;
  const startRender = (): void => {
    historyLoading = true;
    abort = assistantRenderStream(id, {
      onSeq: (seq) => agentRun.noteSeq(id, seq),
      onEvent: (event) => applyAssistantEvent(id, event),
      onError: (text) => {
        releaseHistoryStream();
        // 首个 /render 可能在浏览器预检 / 连接阶段被取消（控制台 net::ERR_ABORTED，非前端主动 abort）：
        // 静默重试一次，避免把一次可自愈的取消误报给用户；重试仍失败才提示。抽屉已关或已切走则不重试。
        if (!retried && opened.value && activeSessionID.value === id) {
          retried = true;
          window.setTimeout(() => {
            if (opened.value && activeSessionID.value === id) {
              startRender();
            }
          }, 300);
          return;
        }
        notifyError(text);
      },
      onComplete: () => {
        releaseHistoryStream();
        scrollToBottom();
      },
    });
    agentRun.attach(id, abort);
  };
  startRender();
};

// pickSession 从会话列表选中一行：先退回对话视图，再切换会话并滚到最新消息
const pickSession = async (id: number): Promise<void> => {
  sessionListMode.value = false;
  await selectSession(id);
  scrollToBottom();
};

const selectSession = async (id: number): Promise<void> => {
  // 切换会话：先退订上一会话的在途流（仅断连接，不取消服务端），切换后再按新会话的状态重订阅续播
  if (activeSessionID.value !== null && activeSessionID.value !== id) {
    agentRun.detach(activeSessionID.value);
  }
  draftMode.value = false;
  activeSessionID.value = id;
  messages.value = [];
  // 会话记录了缺省档位时同步到档位下拉；未记录时保持当前选择
  const target = sessions.value.find((s) => s.id === id);
  if (target && target.slot) {
    slot.value = target.slot;
  }
  // 会话级审核阀门随会话回显，避免沿用上一会话的选择
  if (target && target.reviewValve) {
    reviewValve.value = target.reviewValve;
  }
  await loadHistory(id);
  // 载入历史后按在途运行快照恢复交互状态（运行中 / 待审批 / 暂停）
  await syncSessionState(id);
  // 载入该会话本地记忆的待发队列；切到空闲会话时自动投递其队首提问（在途运行中由 busy 闸门挡下）
  ensureQueueLoaded(id);
  drainQueuedQuery(id);
};

// ---- 会话增删改 ----
// toggleSessionList 折叠 / 展开会话列表：展开时以列表替换消息区与输入区
const toggleSessionList = (): void => {
  sessionListMode.value = !sessionListMode.value;
};

// startNewSession 进入待创建态：会话在首条消息发出时才创建（主题由后端据该消息生成）；
// 由列表触发时同步退回对话视图，让用户直接落到输入区
const startNewSession = (): void => {
  // 进入待创建态：退订上一会话的在途流（不取消服务端，用户可能稍后切回）
  if (activeSessionID.value !== null) {
    agentRun.detach(activeSessionID.value);
  }
  sessionListMode.value = false;
  draftMode.value = true;
  activeSessionID.value = null;
  messages.value = [];
  activeAssistantKey = null;
  pendingUserEcho = null;
  activeInterrupt = null;
  questionnaireOverrides.clear();
  approvalOverrides.clear();
};

// openRenameSessionDialog 打开某一行的重命名对话框：标题预填该行原文
const openRenameSessionDialog = (session: AgentSessionItem): void => {
  renamingSessionID.value = session.id;
  renameForm.resetForm({ values: { title: session.title } });
  renameSessionDialogRef.value?.open({});
};

// openDeleteSessionDialog 打开某一行的删除确认对话框
const openDeleteSessionDialog = (session: AgentSessionItem): void => {
  deletingSessionID.value = session.id;
  deleteConfirmRef.value?.open({});
};

const renameSessionAction = renameForm.handleSubmit(async (values) => {
  if (renamingSessionID.value === null) {
    return;
  }
  await getSimpleResponse(
    agentSessionRename(renamingSessionID.value, {
      id: renamingSessionID.value,
      title: (values.title as string).trim(),
    }),
    async (rsp) => {
      await loadSessions();
      appStore.notify({ text: rsp.summary, timeout: 3000, color: "success" });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
});

// updateReviewValve 切换审核阀门：草稿态仅更新本地值（随创建请求写入）；会话态调用接口
// 持久化，并刷新会话列表回显
const updateReviewValve = async (value: string | null): Promise<void> => {
  if (!value) {
    return;
  }
  reviewValve.value = value;
  if (activeSessionID.value === null) {
    return;
  }
  await getSimpleResponse(
    agentSessionValveSet(activeSessionID.value, { reviewValve: value }),
    async (rsp) => {
      await loadSessions();
      appStore.notify({ text: rsp.summary, timeout: 3000, color: "success" });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

const deleteSessionAction = async (): Promise<void> => {
  if (deletingSessionID.value === null) {
    return;
  }
  const id = deletingSessionID.value;
  // 删除会话前退订其（若为当前会话的）在途流，避免残留订阅
  agentRun.detach(id);
  agentRun.settle(id);
  await getSimpleResponse(
    agentSessionDelete(id),
    async (rsp) => {
      sessions.value = sessions.value.filter((s) => s.id !== id);
      // 会话已删除：其待发队列一并清除，避免本地留下孤儿记录（内存分桶与本地记忆同步清理）
      delete agentQueues.value[id];
      localManager.clearAgentQueue(id);
      if (activeSessionID.value === id) {
        activeSessionID.value = null;
        messages.value = [];
      }
      appStore.notify({ text: rsp.summary, timeout: 3000, color: "success" });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// ---- 流式问答 ----
const query = ref("");
// creating 是否正在创建会话：未选中会话时首条消息需先建会话再发起流式生成，
// 这段时间无在途运行，单独标记以免同一问题被重复提交；
// 同时驱动发送按钮转入加载态，把创建等待期（后端同步生成主题，耗时明显）显式反馈给用户
const creating = ref(false);
// optimizing 是否正在优化提问：调用提示词优化接口改写当前输入，与运行态一并纳入提交闸门（见 canSend）
const optimizing = ref(false);
// compacting 是否正在手动压缩上下文（「压缩到此」触发的独立一次性 SSE 流）：
// 在途期间不得再向后端发起生成（见 canSend），但提问仍可转入待发队列排队（见 canEnqueueQueued）
const compacting = ref(false);

// ---- 会话交互状态（唯一权威来自后端）----
// 交互主状态取值的后端口径（等待 / 运行 / 已锁定）：前端不声明完整取值空间，
// 仅在此用于判定闸门与派生态；其余取值（暂停）与未知空状态一律按「可输入」保守兜底
const STATE_RUNNING = "running";
const STATE_WAITING = "waiting";
const STATE_LOCKED = "locked";

// activeRunState 当前会话的运行态快照（agentRun store 持有）；未选中会话或无记录时为 undefined
const activeRunState = computed<RunState | undefined>(() => {
  const id = activeSessionID.value;
  return id === null ? undefined : agentRun.runs[id];
});

// interactionState / interactionSubstate 当前会话交互主状态与子状态：唯一权威来自后端
// （SSE state 事件 / 快照出参），经 store 透传；空串表示未知，由派生态保守兜底（绝不卡死）
const interactionState = computed<string>(() => activeRunState.value?.state ?? "");
const interactionSubstate = computed<string>(() => activeRunState.value?.substate ?? "");

// runActive 会话是否有在途活动（运行中 / 等待中）：停止按钮可用、回滚类操作禁用的唯一依据
const runActive = computed<boolean>(
  () => interactionState.value === STATE_RUNNING || interactionState.value === STATE_WAITING,
);

// locked 会话是否已锁定（有效轮次达上限）：输入闸门关闭，但回滚类操作仍放行（回滚后可继续提问）
const locked = computed<boolean>(() => interactionState.value === STATE_LOCKED);

// inputGated 输入闸门是否关闭：运行中 / 等待中 / 已锁定 均关闭；暂停（含未知空状态）放行（保守兜底）
const inputGated = computed<boolean>(() => runActive.value || locked.value);

// userTurns / maxUserTurns 快照口径的会话有效轮次与上限（供锁定提示展示）
const userTurns = computed<number>(() => activeRunState.value?.userTurns ?? 0);
const maxUserTurns = computed<number>(() => activeRunState.value?.maxUserTurns ?? 0);

// questionnairePending 当前会话是否正等待问卷作答（waiting + answer）：输入区整体禁用并给出跳转提示
const questionnairePending = computed<boolean>(
  () => interactionState.value === STATE_WAITING && interactionSubstate.value === "answer",
);

// composerDisabled 输入控件是否禁用：问卷待作答或会话已锁定——此刻既发不出去、也不该入队，直接禁用输入；
// 其余在途状态（运行中 / 等待审批）仍允许输入以转入待发队列（见 canEnqueueQueued）
const composerDisabled = computed<boolean>(() => questionnairePending.value || locked.value);

// stateLabel / substateLabel 交互主状态与子状态展示文案：label 由后端枚举类下发，前端不硬编码文案
const stateLabel = computed<string>(() =>
  interactionState.value
    ? enumLabelOf(EnumClass.agentInteractionState, interactionState.value)
    : "",
);
const substateLabel = computed<string>(() =>
  interactionSubstate.value
    ? enumLabelOf(EnumClass.agentInteractionSubstate, interactionSubstate.value)
    : "",
);

// composerStatusText 输入区状态条文案：已锁定时展示锁定提示（含「回滚后可继续提问」引导）；
// 其余在途状态展示后端下发的交互子状态 label（缺失回退主状态 label），使输入被闸住的原因对用户可见
const composerStatusText = computed<string>(() => {
  if (locked.value) {
    return i18n.global.t("page.aiAssistant.lockedHint", {
      label: stateLabel.value,
      used: userTurns.value,
      max: maxUserTurns.value,
    });
  }
  return substateLabel.value || stateLabel.value;
});

// canSend 是否可发起生成：AI 可用、输入闸门未关（运行中 / 等待中 / 已锁定均不可发），
// 且不在创建会话 / 优化提问 / 手动压缩途中——这些运行态下输入框内容或将被替换、或无权改写。
// 不要求已有会话——未选中会话时发送即先创建会话（主题由后端据该问题生成）
const canSend = computed<boolean>(
  () =>
    aiEnabled.value &&
    !inputGated.value &&
    !creating.value &&
    !optimizing.value &&
    !compacting.value,
);

// composerActionsEnabled 输入区动作（优化 / 撤回）的公共闸门：AI 可用，且输入闸门未关、
// 无会话创建 / 无优化请求
const composerActionsEnabled = computed<boolean>(
  () => aiEnabled.value && !inputGated.value && !creating.value && !optimizing.value,
);

// canOptimize 是否可优化提问：公共闸门可用，且输入框已有非空内容——无诉求可优化
const canOptimize = computed<boolean>(
  () => composerActionsEnabled.value && query.value.trim() !== "",
);

// canUndoOptimize 是否可撤回优化：与优化同源闸门，但不要求输入非空（撤回是把原文写回输入框）
const canUndoOptimize = composerActionsEnabled;

// optimizeUndo 提示词优化的撤回现场：非 null 表示刚完成一次优化、用户尚未改动或发送。
// origin 保存优化前原文，optimized 保存优化后文本——一旦输入被改成与 optimized 不同的内容，
// 即视为用户已改写 / 接纳结果，撤回语义不再成立（见下方 watch）
const optimizeUndo = ref<{ origin: string; optimized: string } | null>(null);

// 输入框内容偏离优化结果（用户手动编辑，或发送后清空、回退 / 复制 / 切换会话等程序化改写）即作废
// 撤回现场，按钮回到「优化提问」
watch(query, (value) => {
  const undo = optimizeUndo.value;
  if (undo && value !== undo.optimized) {
    optimizeUndo.value = null;
  }
});

// ---- 技能选择（斜杠触发）----
// SkillOption 技能下拉选项：内置与自定义技能统一以 agent_skills 行 ID 作为选择标识，
// 选择与关键字匹配只需 id / name / description
interface SkillOption {
  id: number;
  name: string;
  description: string;
}

// skillOptions 合并内置技能与当前命名空间已启用自定义技能的选项；skillOptionsNamespace
// 记录已加载的命名空间（含「已加载但为空」），避免每次按键都重复请求
const skillOptions = ref<SkillOption[]>([]);
const skillOptionsNamespace = ref("");
const selectedSkill = ref<SkillOption | null>(null);
const skillMenuOpen = ref(false);
const skillLoading = ref(false);
const skillActiveIndex = ref(0);

// skillToken 输入框整段恰为「/关键字」时取关键字（「/」后即为空串）；不匹配斜杠形态时为 null
const skillToken = computed<string | null>(() => /^\/([^\s]*)$/.exec(query.value)?.[1] ?? null);

// skillMenuVisible 技能下拉是否可见：已处于斜杠触发态且输入法未在组字
// （组字期间的按键属于候选词选择，不应弹出下拉）
const skillMenuVisible = computed<boolean>(
  () => skillMenuOpen.value && skillToken.value !== null && !composerComposing.value,
);

// filteredSkills 按关键字对技能名 / 描述做小写包含匹配，最多展示 8 项
const filteredSkills = computed<SkillOption[]>(() => {
  const kw = (skillToken.value ?? "").toLowerCase();
  return skillOptions.value
    .filter(
      (s) => !kw || s.name.toLowerCase().includes(kw) || s.description.toLowerCase().includes(kw),
    )
    .slice(0, 8);
});

// ensureSkillsLoaded 懒加载技能选项（内置技能 + 当前命名空间已启用自定义技能）：该命名空间已
// 加载过（含空结果）则直接返回；失败不写入命名空间缓存，下次按键可重试。请求带序号：命名空间
// 切换或并发触发时只采用最后一次请求的结果，避免旧命名空间的响应覆盖新命名空间的列表。
let skillRequestSeq = 0;
const ensureSkillsLoaded = async (): Promise<void> => {
  const requested = appStore.namespace;
  if (skillOptionsNamespace.value === requested) {
    return;
  }
  const seq = ++skillRequestSeq;
  skillLoading.value = true;
  try {
    await getResponse<AgentSkillOptionListOut>(
      agentSkillOptionsGet(),
      async (rsp) => {
        if (seq !== skillRequestSeq || appStore.namespace !== requested) {
          return;
        }
        skillOptions.value = rsp.data.items;
        skillOptionsNamespace.value = requested;
      },
      async (rsp) => {
        if (seq === skillRequestSeq) {
          notifyError(rsp.summary);
        }
      },
      notifyNoResponse,
    );
  } finally {
    if (seq === skillRequestSeq) {
      skillLoading.value = false;
    }
  }
};

// selectSkillAt 选中技能下拉中的第 index 项：记为已选技能、清空输入框并关闭下拉，
// 随后把焦点交还输入框以便续写提问
const selectSkillAt = (index: number): void => {
  const item = filteredSkills.value[index];
  if (!item) {
    return;
  }
  selectedSkill.value = item;
  query.value = "";
  skillMenuOpen.value = false;
  void nextTick(() => composerRef.value?.focus());
};

// onComposerKeydown 输入框按键：技能下拉可见时接管上下键（循环移动高亮）、Esc（关闭下拉）与
// "/"（关闭信号，斜杠本身被吞掉），其余按键一律不干预。
// 斜杠规则与编辑器侧的 slashTrigger.ts 一致：首个 "/" 唤起下拉，再次输入 "/" 仅关闭下拉，
// 输入区保留那个字面量斜杠（不会变成 "//"）。
const onComposerKeydown = (event: KeyboardEvent): void => {
  if (!skillMenuVisible.value) {
    return;
  }
  if (event.key === "/") {
    event.preventDefault();
    skillMenuOpen.value = false;
    return;
  }
  const total = filteredSkills.value.length;
  if (event.key === "ArrowDown") {
    event.preventDefault();
    if (total > 0) {
      skillActiveIndex.value = (skillActiveIndex.value + 1) % total;
    }
    return;
  }
  if (event.key === "ArrowUp") {
    event.preventDefault();
    if (total > 0) {
      skillActiveIndex.value = (skillActiveIndex.value - 1 + total) % total;
    }
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    skillMenuOpen.value = false;
  }
};

// 下拉项变化（过滤结果变短）时把高亮收敛到合法范围，避免回车选中越界项
watch(
  () => filteredSkills.value.length,
  (length) => {
    if (skillActiveIndex.value >= length) {
      skillActiveIndex.value = length > 0 ? length - 1 : 0;
    }
  },
);

// 输入框进入 / 离开「/关键字」形态：进入即打开下拉并预加载技能，离开则收起
watch(query, () => {
  const token = skillToken.value;
  if (token === null) {
    skillMenuOpen.value = false;
    return;
  }
  skillMenuOpen.value = true;
  skillActiveIndex.value = 0;
  void ensureSkillsLoaded();
});

// 命名空间切换：技能按命名空间隔离，清空缓存与已选技能；下拉仍打开时立即重载
watch(
  () => appStore.namespace,
  () => {
    skillOptions.value = [];
    skillOptionsNamespace.value = "";
    selectedSkill.value = null;
    if (skillMenuOpen.value) {
      void ensureSkillsLoaded();
    }
  },
);

// ---- 用户消息快捷操作（复制 / 回退 / 删除）----
// busy 当前会话是否正忙（在途运行中 / 创建会话中 / 手动压缩中）：忙时禁止截断，避免删除在途运行正在使用的
// 消息，导致服务端与本地内容错位；发送入口亦据此把新提问转入待发队列（见 onSubmitClick / onComposerEnter）。
// 已锁定不算忙——锁定只闸住「新提问」，回滚类操作须仍可用（见各按钮的 disabled 绑定）
const busy = computed<boolean>(() => runActive.value || creating.value || compacting.value);

// ---- 待发提问队列（生成中提交的追问）----
// QueuedQuery 队列中的一条待发提问；key 仅用于列表渲染与操作定位，不随正文持久化
interface QueuedQuery {
  key: number;
  text: string;
  // 入队时选中的技能 ID 与展示名（仅内存态，随队列投递的请求透传；正文持久化不包含此项）
  skillID?: number;
  skillName?: string;
}

// agentQueues 各会话的待发队列（按会话 ID 分桶，内存态；正文由 localManager 持久化）
const agentQueues = ref<Record<number, QueuedQuery[]>>({});

// activeQueue 当前会话的待发队列（未选中会话时为空）
const activeQueue = computed<QueuedQuery[]>(() =>
  activeSessionID.value === null ? [] : (agentQueues.value[activeSessionID.value] ?? []),
);

// canModifyQueued 队列项能否「修改」：修改即「出队 + 回填输入框」，输入框已写好内容时
// 回填会覆盖用户输入，故仅在输入框为空时可用。删除与置顶都不触碰输入框，不受此限。
const canModifyQueued = computed<boolean>(() => query.value.trim() === "");

// canEnqueueQueued 运行中能否把当前输入转入待发队列：队列按会话归属，故须已选中会话；
// 须确有「后续会投递队列的在途运行」（在途运行中 / 手动压缩在途）——已锁定时无运行可投递，
// 不得入队（应引导回滚）；创建会话（尚无会话可归属）与优化途中（输入将被整体改写）仍禁止；
// 问卷待作答时会话被阻塞，亦不得入队
const canEnqueueQueued = computed<boolean>(
  () =>
    aiEnabled.value &&
    activeSessionID.value !== null &&
    query.value.trim() !== "" &&
    (runActive.value || compacting.value) &&
    !creating.value &&
    !optimizing.value &&
    !questionnairePending.value,
);

// ensureQueueLoaded 懒加载某会话的队列：首次访问时从本地记忆恢复，按恢复顺序生成列表 key
const ensureQueueLoaded = (sessionID: number): void => {
  if (agentQueues.value[sessionID]) {
    return;
  }
  agentQueues.value[sessionID] = localManager
    .getAgentQueue(sessionID)
    .map((text) => ({ key: nextKey(), text }));
};

// persistQueue 把某会话的队列正文写回本地记忆（空队列自动清除记录）
const persistQueue = (sessionID: number): void => {
  localManager.setAgentQueue(
    sessionID,
    (agentQueues.value[sessionID] ?? []).map((it) => it.text),
  );
};

// enqueueQuery 把当前输入转入待发队列：入队后清空输入框，队列项堆叠在输入框上方，
// 待当前回答结束后自动依次投递（见 drainQueuedQuery）
const enqueueQuery = (): void => {
  const sessionID = activeSessionID.value;
  const text = query.value.trim();
  if (sessionID === null || text === "") {
    return;
  }
  ensureQueueLoaded(sessionID);
  agentQueues.value[sessionID].push({
    key: nextKey(),
    text,
    skillID: selectedSkill.value?.id,
    skillName: selectedSkill.value?.name,
  });
  selectedSkill.value = null;
  query.value = "";
  persistQueue(sessionID);
};

// removeQueued 删除一条待发提问：只操作队列本身、不触碰输入框，故任何情况下都可用
const removeQueued = (target: QueuedQuery): void => {
  const sessionID = activeSessionID.value;
  if (sessionID === null) {
    return;
  }
  ensureQueueLoaded(sessionID);
  agentQueues.value[sessionID] = agentQueues.value[sessionID].filter((it) => it.key !== target.key);
  persistQueue(sessionID);
};

// editQueued 修改一条待发提问：即「先出队，再把正文回填到输入框」，交由用户改完再提交；
// 要求输入框为空，否则回填会覆盖用户已写内容（见 canModifyQueued）
const editQueued = (target: QueuedQuery): void => {
  const sessionID = activeSessionID.value;
  if (sessionID === null || !canModifyQueued.value) {
    return;
  }
  ensureQueueLoaded(sessionID);
  agentQueues.value[sessionID] = agentQueues.value[sessionID].filter((it) => it.key !== target.key);
  query.value = target.text;
  // 该项入队时带技能：一并回填已选技能，重新提交时仍按同一技能生成
  if (target.skillID) {
    selectedSkill.value = {
      id: target.skillID,
      name: target.skillName ?? "",
      description: "",
    };
  }
  persistQueue(sessionID);
};

// pinQueued 置顶一条待发提问：移到队首，成为下一个自动投递的问题
const pinQueued = (target: QueuedQuery): void => {
  const sessionID = activeSessionID.value;
  if (sessionID === null) {
    return;
  }
  ensureQueueLoaded(sessionID);
  const list = agentQueues.value[sessionID];
  const index = list.findIndex((it) => it.key === target.key);
  if (index <= 0) {
    return;
  }
  const [item] = list.splice(index, 1);
  list.unshift(item);
  persistQueue(sessionID);
};

// drainQueuedQuery 当前回答结束后自动投递队首提问：仅当目标会话即当前会话、无在途运行且未锁定时执行
// （切走的会话其队列留待切回时再投递，避免把消息推入非当前会话的消息区；已锁定则投递必被后端拒绝）；
// 先出队持久化再发起生成
const drainQueuedQuery = (sessionID: number): void => {
  if (sessionID !== activeSessionID.value || busy.value || locked.value) {
    return;
  }
  ensureQueueLoaded(sessionID);
  const list = agentQueues.value[sessionID];
  if (!list || list.length === 0) {
    return;
  }
  const [head] = list.splice(0, 1);
  persistQueue(sessionID);
  optimizeUndo.value = null;
  // 队列项的技能（仅内存态）随投递一并透传；无技能时按未选技能发送
  const skill: SkillOption | null = head.skillID
    ? { id: head.skillID, name: head.skillName ?? "", description: "" }
    : null;
  startStream(sessionID, head.text, undefined, skill);
};

// onSubmitClick 发送按钮 / 回车统一入口：空闲时直接发送，运行中（生成 / 待审批 / 建会话 / 压缩）则转入待发队列排队。
// 运行中并非一律可入队（见 canEnqueueQueued）：问卷待作答时会话被阻塞，此刻既发不出、也不该排队，
// 否则输入内容会无声消失且永远等不到投递。此时保持输入原样并把原因告知用户，与发送按钮的禁用态一致。
const onSubmitClick = (): void => {
  if (busy.value) {
    if (!canEnqueueQueued.value) {
      if (questionnairePending.value) {
        notifyError(i18n.global.t("page.aiAssistant.questionnairePendingHint"));
      }
      return;
    }
    enqueueQuery();
    return;
  }
  void sendQuery();
};

// ---- 推荐的下一步 ----
// latestAssistantKey 最后一条助手消息的 key：推荐的下一步只在该条消息下展示，历史回答不再重复打扰
const latestAssistantKey = computed<number | undefined>(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    if (messages.value[i].role === "assistant") {
      return messages.value[i].key;
    }
  }
  return undefined;
});

// nextStepSendBlockedReason 推荐项「直接发送」被闸住的原因：空串表示可发送。输入框已有内容时禁用，
// 避免直接发送清空用户草稿；其余闸门与会话发送入口同源。
const nextStepSendBlockedReason = computed<string>(() => {
  if (!aiEnabled.value) {
    return i18n.global.t("page.aiAssistant.disabled");
  }
  if (query.value.trim() !== "") {
    return i18n.global.t("page.aiAssistant.nextStepsDraftNotEmpty");
  }
  if (locked.value) {
    return i18n.global.t("page.aiAssistant.lockedReject");
  }
  if (inputGated.value || creating.value || optimizing.value || compacting.value) {
    return i18n.global.t("page.aiAssistant.stateGuardReject");
  }
  return "";
});

// canSendNextStep 推荐项是否可直接发送
const canSendNextStep = computed<boolean>(() => nextStepSendBlockedReason.value === "");

// nextStepFillBlockedReason 推荐项「填入输入框」被闸住的原因：空串表示可填入。输入区被禁用
// （问卷待作答 / 已锁定）或会话创建 / 优化途中不写入，避免内容被覆盖或清空。
const nextStepFillBlockedReason = computed<string>(() => {
  if (!aiEnabled.value) {
    return i18n.global.t("page.aiAssistant.disabled");
  }
  if (locked.value) {
    return i18n.global.t("page.aiAssistant.lockedReject");
  }
  if (questionnairePending.value) {
    return i18n.global.t("page.aiAssistant.questionnairePendingHint");
  }
  if (creating.value || optimizing.value) {
    return i18n.global.t("page.aiAssistant.stateGuardReject");
  }
  return "";
});

// canFillNextStep 推荐项是否可填入输入框
const canFillNextStep = computed<boolean>(() => nextStepFillBlockedReason.value === "");

// sendNextStep 点击推荐项直接发送：写入输入框后复用既有发送入口（会话创建 / 状态守卫 / 错误提示一致）
const sendNextStep = (text: string): void => {
  if (!canSendNextStep.value) {
    return;
  }
  query.value = text;
  void sendQuery();
};

// appendNextStep 把推荐项填入输入框：已有内容时以换行 append，不覆盖用户草稿；填入后聚焦输入框
const appendNextStep = (text: string): void => {
  if (!canFillNextStep.value) {
    return;
  }
  const base = query.value.replace(/\s+$/u, "");
  query.value = base === "" ? text : `${base}\n${text}`;
  void nextTick(() => composerRef.value?.focus());
};

// messageTimeOf 该条消息创建时间的展示文本（本地化）；缺失时返回空串（信息属补充，不占位）
const messageTimeOf = (m: ChatMessage): string =>
  m.createdAt ? localeDatetimeFormat(DateTime.fromISO(m.createdAt)) : "";

// usageTextOf 拼装某条助手回答末尾的用量说明（token 总量 · 总耗时）：两项各自有值才计入，
// 均无值时返回空串（用户消息、以及未记录用量的历史消息）由模板据此整行隐藏
const usageTextOf = (m: ChatMessage): string => {
  const parts: string[] = [];
  if (m.totalTokens && m.totalTokens > 0) {
    parts.push(
      i18n.global.t("page.aiAssistant.usageTokens", {
        count: formatTokenCount(m.totalTokens),
      }),
    );
  }
  if (m.elapsedMs && m.elapsedMs > 0) {
    parts.push(
      i18n.global.t("page.aiAssistant.usageElapsed", {
        seconds: (m.elapsedMs / 1000).toFixed(1),
      }),
    );
  }
  return parts.join(" · ");
};

// usageDetailOf 悬浮用量合计时的明细：输入（prompt）与输出（completion）token 分列；口径与
// usageTextOf 一致，取本轮全部模型调用的累加值。两项均无值时返回空串，模板据此禁用气泡
const usageDetailOf = (m: ChatMessage): string => {
  const parts: string[] = [];
  if (m.promptTokens && m.promptTokens > 0) {
    parts.push(
      i18n.global.t("page.aiAssistant.usagePromptTokens", {
        count: formatTokenCount(m.promptTokens),
      }),
    );
  }
  if (m.completionTokens && m.completionTokens > 0) {
    parts.push(
      i18n.global.t("page.aiAssistant.usageCompletionTokens", {
        count: formatTokenCount(m.completionTokens),
      }),
    );
  }
  return parts.join(" · ");
};

// interruptedTextOf 取某条助手消息的中断提示文案：phase 为 failed / cancelled 时按阶段返回提示，
// 其余（done / 空 / 用户消息）返回空串由模板整块隐藏。文案由前端 i18n 提供，不落库、不混入正文
const interruptedTextOf = (m: ChatMessage): string => {
  if (m.role !== "assistant") {
    return "";
  }
  if (m.phase === "cancelled") {
    return i18n.global.t("page.aiAssistant.runCancelled");
  }
  if (m.phase === "failed") {
    return i18n.global.t("page.aiAssistant.runFailed");
  }
  return "";
};

// userMessageIndexOf 目标消息在会话内的用户消息序号（从 1 起，仅计用户消息、按消息顺序计），与后端
// 截断接口的定位口径一致；非用户消息或未找到时返回 0
const userMessageIndexOf = (key: number): number => {
  let index = 0;
  for (const m of messages.value) {
    if (m.role === "user") {
      index += 1;
      if (m.key === key) {
        return index;
      }
    }
  }
  return 0;
};

// copyMessage 复制该条消息正文到剪贴板（成功 / 失败提示由 copyToClipboard 统一给出）
const copyMessage = async (m: ChatMessage): Promise<void> => {
  await copyToClipboard(m.content);
};

// precedingUserMessageOf 取该消息之前最近的一条用户消息（即该助手回答所属轮次的提问）；
// 之前没有用户消息时返回 undefined
const precedingUserMessageOf = (key: number): ChatMessage | undefined => {
  let found: ChatMessage | undefined;
  for (const m of messages.value) {
    if (m.key === key) {
      break;
    }
    if (m.role === "user") {
      found = m;
    }
  }
  return found;
};

// retryMessage 重试该轮回答：先截断服务端的本轮提问及其后全部内容（连同这条回答一并删除），
// 再以原提问重新发起生成，等价于「删除当前回答并重新生成」
const retryMessage = async (m: ChatMessage): Promise<void> => {
  const sessionID = activeSessionID.value;
  const question = precedingUserMessageOf(m.key);
  if (sessionID === null || !question || busy.value) {
    return;
  }
  const userIndex = userMessageIndexOf(question.key);
  if (userIndex < 1) {
    return;
  }
  await getSimpleResponse(
    agentMessageTruncate(sessionID, { userMessageIndex: userIndex }),
    async () => {
      // 本地同构截断：自本轮提问起整体删除，随后按原提问重新推送并生成
      const localIndex = messages.value.findIndex((x) => x.key === question.key);
      if (localIndex >= 0) {
        messages.value.splice(localIndex);
      }
      // 重试沿用原提问的技能：气泡上记录的技能随本次重新生成一并透传
      startStream(
        sessionID,
        question.content,
        undefined,
        question.skillID
          ? { id: question.skillID, name: question.skillName ?? "", description: "" }
          : null,
      );
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// branchMessage 分支：以本轮回答为结尾，把该轮及之前的全部消息复制到新会话并切换过去，
// 便于在原会话之外基于同一段上下文另起一条对话支线
const branchMessage = async (m: ChatMessage): Promise<void> => {
  const sessionID = activeSessionID.value;
  const question = precedingUserMessageOf(m.key);
  if (sessionID === null || !question || busy.value) {
    return;
  }
  const userIndex = userMessageIndexOf(question.key);
  if (userIndex < 1) {
    return;
  }
  const source = sessions.value.find((s) => s.id === sessionID);
  await getResponse<AgentSessionItem>(
    agentSessionBranch(sessionID, {
      userMessageIndex: userIndex,
      // 分支标题在源标题后追加本地化后缀，避免会话列表出现两个同名会话
      title: source
        ? i18n.global.t("page.aiAssistant.branchTitle", { title: source.title })
        : undefined,
    }),
    async (rsp) => {
      sessions.value = [rsp.data, ...sessions.value];
      await selectSession(rsp.data.id);
      appStore.notify({
        text: i18n.global.t("page.aiAssistant.branchCreated"),
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// pendingMessage 待确认的截断操作：记录目标消息 key（用于定位本地待删区间）、其在会话内的用户消息
// 序号（供后端定位）、正文（回退确认后回填输入框）与是否为回退
const pendingMessage = ref<{
  key: number;
  userIndex: number;
  content: string;
  // 目标消息携带的技能：回退确认后与正文一起回填输入区（无技能时为 undefined）
  skillID?: number;
  skillName?: string;
  rollback: boolean;
} | null>(null);

const messageActionDialogRef = useTemplateRef("messageActionDialogRef");

// openMessageAction 打开截断确认弹窗：rollback 为 true 走「回退」（确认后把正文回填输入框），
// 为 false 走「删除」
const openMessageAction = (m: ChatMessage, rollback: boolean): void => {
  const userIndex = userMessageIndexOf(m.key);
  if (userIndex < 1) {
    return;
  }
  pendingMessage.value = {
    key: m.key,
    userIndex,
    content: m.content,
    skillID: m.skillID,
    skillName: m.skillName,
    rollback,
  };
  messageActionDialogRef.value?.open({});
};

// confirmMessageAction 确认截断：先删除服务端该轮及其之后的全部消息，成功后同构截断本地列表
// （保留目标之前的历史），回退模式再把目标正文原样回填输入框
const confirmMessageAction = async (): Promise<void> => {
  const target = pendingMessage.value;
  const sessionID = activeSessionID.value;
  if (!target || sessionID === null) {
    return;
  }
  await getSimpleResponse(
    agentMessageTruncate(sessionID, { userMessageIndex: target.userIndex }),
    async (rsp) => {
      // 截断已一并作废该轮及其之后的运行（后端同构处理）：本地运行态须同步收敛，否则残留的
      // 「生成中」闸门会把新提问挡进待发队列，且后续探测续播会把已删除回合的产出重新拼回消息区
      agentRun.detach(sessionID);
      agentRun.settle(sessionID);
      const localIndex = messages.value.findIndex((m) => m.key === target.key);
      if (localIndex >= 0) {
        messages.value.splice(localIndex);
      }
      // 被截断的区间若含本地提问回显，回显随之作废，避免后续重建把已删除的提问再补回
      if (pendingUserEcho && !messages.value.some((m) => m.key === pendingUserEcho?.key)) {
        pendingUserEcho = null;
      }
      if (target.rollback) {
        query.value = target.content;
        // 技能随正文一并回填，回退后再次发送仍按同一技能生成；该轮未带技能时同步清空当前选择
        selectedSkill.value = target.skillID
          ? { id: target.skillID, name: target.skillName ?? "", description: "" }
          : null;
      }
      scrollToBottom();
      appStore.notify({ text: rsp.summary, timeout: 3000, color: "success" });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// ASK_USER_TOOL_NAME 主动提问客户端工具名（与后端登记一致）：命中即豁免人工审批，改由问卷面板承载。
const ASK_USER_TOOL_NAME = "ask_user";
// API_INVOKE_TOOL_NAME 只读能力调用：后端已在挂起前把能力解析为规范入参（method/path/params），
// 前端批准后按该入参在用户自身凭据下执行，并把响应作为工具结果回传。
const API_INVOKE_TOOL_NAME = "api_invoke";
// API_INVOKE_RESULT_MAX_CHARS 前端回传的工具结果字符上限：结果最终还会经后端
// agent.maxToolResultBytes 截断，此处先挡住超大响应，避免把整段载荷塞进挂起现场。
const API_INVOKE_RESULT_MAX_CHARS = 48_000;

// awaitingQuestionnaireBlockOf 取消息中「仍待回答」的那一轮问卷块（后端保证只可能是末轮）：非空即该轮
// 问卷已内联在消息流中，会话阻塞直至用户提交或取消；提交 / 取消据其载荷原位更新为已作答。
const awaitingQuestionnaireBlockOf = (m: ChatMessage | null): AgentMessageBlockOut | null => {
  const blocks = m?.blocks ?? [];
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i];
    if (b.kind !== "questionnaire") {
      continue;
    }
    const payload = b.payload as AgentQuestionnaireBlockPayload;
    if (payload?.awaiting) {
      return b;
    }
  }
  return null;
};

// AssistantBlock 助手消息体内的**一块**渲染块：由后端有序内容块（AgentMessageBlockOut）直接映射而来，
// 故实时流与刷新 / 重进会话（消息出参 blocks）由同一套映射渲染，二者天然一致。
interface AssistantBlock {
  key: string;
  kind: "reasoning" | "content" | "stages" | "questionnaire" | "approval" | "nextSteps";
  // text 仅 reasoning / content 使用：该块的文本
  text?: string;
  // primary 仅 reasoning / content 使用：是否为同类块中的末个（reasoning 为流式思考块、content 为末尾正文）
  primary?: boolean;
  // stages / parallelCount 仅 stages 使用：本块（一轮）可见调用行与其数量（>1 时展示并行计数）
  stages?: AgentStageState[];
  parallelCount?: number;
  // questionnaire 仅 questionnaire 使用：本轮问卷载荷（题目 + 作答 + 是否待回答）
  questionnaire?: AgentQuestionnaireBlockPayload;
  // questionnaireInterruptIDs 仅 questionnaire 使用：本轮 ask_user 调用的根中断 ID（已作答卡片不再
  // 渲染提交按钮，「重新回答」据其定位交互并回退到未作答状态）
  questionnaireInterruptIDs?: string[];
  // reanswerable 仅 questionnaire 使用：是否为末个问卷块（「重新回答」只作用于末轮：更早各轮的运行现场
  // 已被后续轮次消费、无从重建）
  reanswerable?: boolean;
  // approvalCalls 仅 approval 使用：本块待审批调用清单
  approvalCalls?: AgentApprovalCallRecord[];
  // approvalCallIDs 仅 approval 使用：本块全部调用的 callID（含已决策行，供「重新审批」定位交互）
  approvalCallIDs?: string[];
  // nextSteps 仅 nextSteps 使用：推荐的下一步列表（首条为最推荐）
  nextSteps?: AgentNextStep[];
}

// lastSeqOfKind 求某类块在块列表中的最大 seq（无该类块时返回 -1）：用于标记末个同 kind 块（流式思考 /
// 末尾正文 / 可重答的末轮问卷）
const lastSeqOfKind = (blocks: AgentMessageBlockOut[], kind: string): number => {
  let seq = -1;
  for (const b of blocks) {
    if (b.kind === kind) {
      seq = b.seq;
    }
  }
  return seq;
};

// assistantBlocksOf 把一条助手消息的有序内容块（blocks）映射为渲染块：逐个按 kind 取载荷——文字块
// （reasoning / content）取 payload.text，工具调用块把记录映射为阶段行，交互块取清单 / 问卷载荷；空的
// 文字块与无可见调用行的工具调用块不占位。同类块的末个分别标记为 primary（流式思考 / 末尾正文）与
// reanswerable（末轮问卷）。
const assistantBlocksOf = (m: ChatMessage): AssistantBlock[] => {
  const blocks = m.blocks ?? [];
  const lastReasoning = lastSeqOfKind(blocks, "reasoning");
  const lastContent = lastSeqOfKind(blocks, "content");
  const lastQuestionnaire = lastSeqOfKind(blocks, "questionnaire");
  const out: AssistantBlock[] = [];
  for (const b of blocks) {
    const key = `b-${b.seq}`;
    switch (b.kind) {
      case "reasoning": {
        const text = (b.payload as AgentReasoningBlockPayload)?.text ?? "";
        if (text !== "") {
          out.push({ key, kind: "reasoning", text, primary: b.seq === lastReasoning });
        }
        break;
      }
      case "content": {
        const text = (b.payload as AgentContentBlockPayload)?.text ?? "";
        if (text !== "") {
          out.push({ key, kind: "content", text, primary: b.seq === lastContent });
        }
        break;
      }
      case "tool_calls": {
        const stages = stageRowsOf(b.payload as AgentToolCallsBlockPayload);
        if (stages.length > 0) {
          out.push({ key, kind: "stages", stages, parallelCount: stages.length });
        }
        break;
      }
      case "questionnaire":
        out.push({
          key,
          kind: "questionnaire",
          questionnaire: b.payload as AgentQuestionnaireBlockPayload,
          questionnaireInterruptIDs: (b.payload as AgentQuestionnaireBlockPayload)?.interruptIDs,
          reanswerable: b.seq === lastQuestionnaire,
        });
        break;
      case "approval":
        out.push({
          key,
          kind: "approval",
          approvalCalls: (b.payload as AgentApprovalBlockPayload)?.calls ?? [],
          approvalCallIDs: (b.payload as AgentApprovalBlockPayload)?.callIDs,
        });
        break;
      case "next_steps": {
        const steps = (b.payload as AgentNextStepsBlockPayload)?.steps ?? [];
        if (steps.length > 0) {
          out.push({ key, kind: "nextSteps", nextSteps: steps });
        }
        break;
      }
    }
  }
  return out;
};

// foldedMessageKeys 已折叠进摘要的消息 key 集合：取最新一条（非失败）压缩记录所在消息为折叠边界——
// 消息列表按 seq 升序，压缩记录的锚点即本次折叠的最后一条消息，故该消息及其之前（按序）的消息都已
// 折叠进滚动摘要。对它们再「压缩到此」，折叠集合为空（compactFoldSet 遇到首条 seq > uptoSeq 即止），
// 是无意义的空操作，故其压缩按钮置灰。压缩失败（phase=failed）的记录不构成边界（该次推进未生效、
// 游标未前进），按钮保持可用以便重试。
const foldedMessageKeys = computed<Set<number>>(() => {
  const keys = new Set<number>();
  const list = messages.value;
  let boundary = -1;
  for (let i = list.length - 1; i >= 0; i--) {
    const c = list[i].compress;
    if (c && c.phase !== "failed") {
      boundary = i;
      break;
    }
  }
  for (let i = 0; i <= boundary; i++) {
    keys.add(list[i].key);
  }
  return keys;
});

// compressIconOf 压缩展示态图标：进行中旋转、失败告警、完成归档
const compressIconOf = (c: CompressState): string => {
  if (c.phase === "running") {
    return "mdi-loading";
  }
  if (c.phase === "failed") {
    return "mdi-alert-circle-outline";
  }
  return "mdi-archive-arrow-down-outline";
};

// compressColorOf 压缩展示态语义色：进行中用主色、失败用错误色、完成取中性（继承弱化色）
const compressColorOf = (c: CompressState): string | undefined => {
  if (c.phase === "failed") {
    return "error";
  }
  return c.phase === "running" ? "primary" : undefined;
};

// expandedCompressKeys 已展开摘要的压缩块消息 key 集合：默认收起（三行截断），点击标题行或收起态
// 正文展开查看全文；压缩完成时自动移出集合（见 message.compress 处理），保证完成后回到收起态
const expandedCompressKeys = ref(new Set<number>());

const compressExpandedOf = (m: ChatMessage): boolean => expandedCompressKeys.value.has(m.key);

// compressBodyOf 压缩块可展示的正文：进行中取增量草稿、完成后取摘要全文（实时取 compress_done、
// 历史取检查点）；失败或无内容时返回空串，正文区与展开入口一并隐藏
const compressBodyOf = (m: ChatMessage): string => {
  const c = m.compress;
  if (!c) {
    return "";
  }
  if (c.phase === "running") {
    return c.draft;
  }
  if (c.phase === "done") {
    return c.summary || c.draft;
  }
  return "";
};

// compressExpandableOf 压缩块是否可展开：仅完成后有摘要全文时提供——收起态是完全收起（不展示正文），
// 点击标题行 / 箭头才展开查看全文；进行中只做三行预览，不提供展开
const compressExpandableOf = (m: ChatMessage): boolean =>
  m.compress?.phase === "done" && compressBodyOf(m) !== "";

// toggleCompress 展开 / 收起压缩摘要；仅完成后的摘要可切换
const toggleCompress = (m: ChatMessage): void => {
  if (!compressExpandableOf(m)) {
    return;
  }
  if (expandedCompressKeys.value.has(m.key)) {
    expandedCompressKeys.value.delete(m.key);
    return;
  }
  expandedCompressKeys.value.add(m.key);
};

// compressTokensOf 渲染占用 token 数值：无可用测量（0 或缺失——如会话最近一条助手消息被取消 /
// 失败而从未记下用量）时以「未知」占位，避免把「没有基线」误显示成 0，更避免据此给出「压缩后
// 大于压缩前」这类不可能的对比
const compressTokensOf = (tokens?: number): number | string =>
  tokens !== undefined && tokens > 0
    ? tokens
    : i18n.global.t("page.aiAssistant.compressTokensUnknown");

// compressTextOf 压缩展示态文案：进行中提示压缩中、失败提示失败、完成优先展示前后对比（已有
// 压缩后占用——压缩完成时的估算或后续生成的真实回填——且为正、且当前档位容量已知时），否则仅
// 展示压缩前占用与折叠条数
const compressTextOf = (m: ChatMessage): string => {
  const c = m.compress;
  if (!c) {
    return "";
  }
  if (c.phase === "running") {
    return i18n.global.t("page.aiAssistant.compressStart");
  }
  if (c.phase === "failed") {
    return i18n.global.t("page.aiAssistant.compressFailed");
  }
  if (c.tokensAfter !== undefined && c.tokensAfter > 0 && contextLimitTokens.value > 0) {
    return i18n.global.t("page.aiAssistant.compressCompare", {
      before: compressTokensOf(c.tokensBefore),
      after: c.tokensAfter,
      percent: Math.round((c.tokensAfter / contextLimitTokens.value) * 100),
    });
  }
  return i18n.global.t("page.aiAssistant.compressDone", {
    before: compressTokensOf(c.tokensBefore),
    count: c.foldedCount ?? 0,
  });
};

// finishCompact 手动压缩收尾：落下在途标记并随即投递排队中的提问（压缩期间不允许直接发起生成，
// 提问只能进待发队列，见 canSend / canEnqueueQueued）。压缩流的完成 / 失败 / 关闭三处均经此收尾
const finishCompact = (sessionID: number): void => {
  compacting.value = false;
  drainQueuedQuery(sessionID);
};

// compactToHere 手动把上下文压缩到该助手消息：以该助手消息 ID 为折叠边界调用压缩接口。
// 压缩以 message.compress 事件更新该助手消息的压缩展示态，完成后刷新会话列表使占用环
// 下降到压缩后的估算值。
const compactToHere = (m: ChatMessage): void => {
  const sessionID = activeSessionID.value;
  // busy 已含 compacting：手动压缩亦属在途运行，串行触发由同一闸门兜住
  // m.id 缺失（尚未落库的即时气泡）时无处锚定边界，不作发起
  if (sessionID === null || busy.value || m.id === undefined) {
    return;
  }
  compacting.value = true;
  // 压缩流同样在 EOF / 错误后释放句柄，避免后续 attach / detach 对已结束的连接再 abort。
  let abort: (() => void) | null = null;
  const releaseCompactStream = (): void => {
    if (abort) {
      agentRun.release(sessionID, abort);
    }
  };
  abort = assistantCompactStream(
    sessionID,
    { assistantMessageID: m.id },
    {
      onEvent: (event) => applyAssistantEvent(sessionID, event),
      onError: (message) => {
        releaseCompactStream();
        notifyError(message || i18n.global.t("page.aiAssistant.compressFailed"));
        finishCompact(sessionID);
      },
      onComplete: () => {
        releaseCompactStream();
        appStore.notify({
          text: i18n.global.t("page.aiAssistant.compressSucceeded"),
          timeout: 3000,
          color: "success",
        });
        void loadSessions();
        finishCompact(sessionID);
      },
    },
  );
  agentRun.attach(sessionID, abort);
};

// ---- 领域事件流（assistant.Event）----
// 消息体、交互与本地收敛态复用同一 ChatMessage / blocks 模型；本节只负责把事件写入该模型。

type AssistantEventOf<T extends AssistantEventUnion["type"]> = Extract<
  AssistantEventUnion,
  { type: T }
>;

// activeAssistantMessage 取当前承载实时增量的助手消息。
const activeAssistantMessage = (): ChatMessage | undefined =>
  activeAssistantKey !== null
    ? messages.value.find((msg) => msg.key === activeAssistantKey)
    : undefined;

// assistantMessageFor 定位事件的助手目标：优先当前活动消息，其次按续跑锚点消息 ID，
// 最后（非历史回放时）末条助手消息；历史回放中的新回合不得复用上一轮气泡。
const assistantMessageFor = (messageID?: number): ChatMessage | undefined => {
  const active = activeAssistantMessage();
  if (active) {
    return active;
  }
  if (messageID !== undefined && messageID > 0) {
    const found = messages.value.find((msg) => msg.role === "assistant" && msg.id === messageID);
    if (found) {
      return found;
    }
  }
  if (historyLoading) {
    return undefined;
  }
  for (let i = messages.value.length - 1; i >= 0; i--) {
    if (messages.value[i].role === "assistant") {
      return messages.value[i];
    }
  }
  return undefined;
};

// bindAssistantBlocks 把消息的块列表指向 store 的当前运行块数组：实时 block 事件写入 store 后，
// 消息体随同一数组即时渲染。绑定前先按语义身份清理同块重复（快照块与在途回放块序号不同时会写进
// 两份，表现为同一「AI 提问」卡片出现两次）。
const bindAssistantBlocks = (sessionID: number, msg: ChatMessage): void => {
  msg.blocks = agentRun.normalizeBlocks(sessionID);
};

// syncAssistantContent 由 content 块派生消息纯文本（复制 / 提问导航 / 兜底展示用）。
const syncAssistantContent = (msg: ChatMessage | undefined): void => {
  if (!msg) {
    return;
  }
  const parts = (msg.blocks ?? [])
    .filter((block) => block.kind === "content")
    .map((block) => (block.payload as AgentContentBlockPayload)?.text ?? "")
    .filter((text) => text !== "");
  msg.content = parts.join("\n\n");
};

// snapshotCompressOf 把快照压缩记录映射为展示态（历史记录恒为 done）。
const snapshotCompressOf = (
  compress: AssistantMessageSnapshot["compress"],
): CompressState | undefined => {
  if (!compress) {
    return undefined;
  }
  const extended = compress as AssistantMessageSnapshot["compress"] & {
    phase?: CompressState["phase"];
    draft?: string;
  };
  return {
    phase: extended.phase ?? "done",
    draft: extended.draft ?? "",
    summary: compress.summary,
    reason: compress.reason as CompressState["reason"],
    tokensBefore: compress.tokensBefore,
    foldedCount: compress.foldedCount,
    tokensAfter: compress.tokensAfter,
  };
};

// pendingCallStateOfRequest 把快照 / 挂起清单中的调用映射为视图态：免审批调用预置为已批准，
// ask_user 留待问卷面板作答后定；中断 ID 取自快照（续跑定位）。
const pendingCallStateOfRequest = (call: ToolRequestPayload): PendingCallState => {
  const state: PendingCallState = {
    callID: call.callID,
    toolName: call.toolName,
    arguments: call.arguments,
    interruptId: call.interruptID,
    auditLevel: call.auditLevel,
    approvalRequired: call.approvalRequired,
    execOwner: call.execOwner,
    decision:
      call.approvalRequired === false && call.toolName !== ASK_USER_TOOL_NAME
        ? "approved"
        : undefined,
  };
  return approvalOverrideOf(state);
};

// chatMessageOfSnapshot 把一条历史快照消息映射为视图消息（块即终态，无需再解析卡片）。
const chatMessageOfSnapshot = (snapshot: AssistantMessageSnapshot): ChatMessage => {
  const blocks = snapshot.blocks ?? [];
  const pendingCalls = (snapshot.pendingCalls ?? []).map(pendingCallStateOfRequest);
  const awaitingQuestionnaire = blocks.some(
    (block) =>
      block.kind === "questionnaire" &&
      Boolean((block.payload as AgentQuestionnaireBlockPayload)?.awaiting),
  );
  applyQuestionnaireOverrides(blocks);
  applyApprovalDecisions(blocks, pendingCalls);
  return {
    key: nextKey(),
    id: snapshot.id > 0 ? snapshot.id : undefined,
    role: snapshot.role === "user" ? "user" : snapshot.role === "system" ? "system" : "assistant",
    content: snapshot.content ?? "",
    createdAt: snapshot.createdAt || undefined,
    citations: snapshot.citations,
    // 本轮技能随用户消息快照回放：刷新后气泡顶部仍展示当时选用的技能，回退 / 分支同样沿用
    skillID: snapshot.skillID,
    skillName: snapshot.skillName,
    blocks,
    pendingCalls: pendingCalls.length > 0 ? pendingCalls : undefined,
    awaitingApproval: pendingCalls.some((call) => !call.decision) || awaitingQuestionnaire,
    promptTokens: snapshot.promptTokens,
    completionTokens: snapshot.completionTokens,
    totalTokens: snapshot.totalTokens,
    elapsedMs: snapshot.elapsedMs,
    phase: snapshot.phase,
    compress: snapshotCompressOf(snapshot.compress),
  };
};

// pendingCallsOfMessage 由消息内的 approval / questionnaire 块反推待处理调用：服务端不下发
// tool_request，块即挂起事实；中断 ID 取自 interaction.waiting（或块的 interruptIDs）。
const pendingCallsOfMessage = (
  msg: ChatMessage,
  interruptIDs: string[],
  blockSeq?: number,
): PendingCallState[] => {
  const fallback = interruptIDs[0];
  const out: PendingCallState[] = [];
  for (const block of msg.blocks ?? []) {
    if (blockSeq !== undefined && blockSeq > 0 && block.seq !== blockSeq) {
      continue;
    }
    if (block.kind === "approval") {
      const payload = block.payload as AgentApprovalBlockPayload;
      const calls = payload?.calls ?? [];
      calls.forEach((call, idx) => {
        out.push({
          callID: call.callID,
          toolName: call.toolName,
          arguments: call.arguments,
          // 同批多个 root interrupt：interrupt.waiting 的 interruptIDs 与审批块 calls 同序下发，
          // 每条调用必须绑定自己的中断 ID。全绑第一个 ID 时续跑只会消费一个目标，其余调用被服务端
          // 再次挂起（同一条调用反复要求审批，且 pendingCalls 里已带旧决策，点击批准 / 拒绝无反应）。
          interruptId: interruptIDs[idx] ?? fallback,
          auditLevel: call.auditLevel,
          approvalRequired: call.approvalRequired,
          execOwner: call.execOwner,
          decision:
            call.decision === true
              ? "approved"
              : call.decision === false
                ? "rejected"
                : call.approvalRequired === false
                  ? "approved"
                  : undefined,
          result: call.result,
          error: call.error,
        });
      });
      continue;
    }
    if (block.kind !== "questionnaire") {
      continue;
    }
    const payload = block.payload as AgentQuestionnaireBlockPayload;
    if (!payload?.awaiting) {
      continue;
    }
    out.push({
      callID: `ask-user-${block.seq}`,
      toolName: ASK_USER_TOOL_NAME,
      arguments: JSON.stringify({ questions: payload.questions }),
      interruptId: payload.interruptIDs?.[0] ?? fallback,
      approvalRequired: false,
      execOwner: "client",
    });
  }
  return out;
};

// interruptOfMessage 从消息的挂起块 / 待处理调用还原续跑定位（刷新后无实时 interaction.waiting）。
const interruptOfMessage = (msg: ChatMessage | undefined): ActiveInterrupt | null => {
  if (!msg) {
    return null;
  }
  const ids = new Set<string>();
  for (const call of msg.pendingCalls ?? []) {
    if (call.interruptId) {
      ids.add(call.interruptId);
    }
  }
  let questionnaire = false;
  for (const block of msg.blocks ?? []) {
    if (block.kind !== "questionnaire") {
      continue;
    }
    const payload = block.payload as AgentQuestionnaireBlockPayload;
    if (payload?.awaiting) {
      questionnaire = true;
    }
    for (const id of payload?.interruptIDs ?? []) {
      ids.add(id);
    }
  }
  const list = [...ids];
  if (list.length === 0) {
    return null;
  }
  return { interruptId: list[0], interruptIds: list, questionnaire };
};

// currentInterrupt 当前挂起定位：优先取实时等待事件，其次从消息块还原。
const currentInterrupt = (msg?: ChatMessage): ActiveInterrupt | null =>
  activeInterrupt ?? interruptOfMessage(msg);

// clearInterrupt 收起挂起提示（提交动作后立即收起；服务端若再次挂起会重新下发）。
const clearInterrupt = (): void => {
  activeInterrupt = null;
};

// userMessageInsertIndex 计算用户消息在同轮助手输出之前应落到的下标：消息主键与后端 seq 同序，
// 故定位首个主键更大的消息；主键缺失时退到当前运行新建的无主键助手占位消息。返回 -1 表示可留在末尾。
const userMessageInsertIndex = (messageID?: number): number => {
  if (messageID !== undefined && messageID > 0) {
    const later = messages.value.findIndex((msg) => msg.id !== undefined && msg.id > messageID);
    if (later >= 0) {
      return later;
    }
  }
  const active = activeAssistantMessage();
  if (active && active.id === undefined) {
    const index = messages.value.indexOf(active);
    return index >= 0 ? index : -1;
  }
  return -1;
};

// placeUserMessage 插入或校正用户消息的位置：turn.started 可能晚于同轮助手快照到达（如中途重放
// 历史后再收到实时事件），直接追加会把提问排到回答之后；此处统一按主键归位到同轮助手输出之前。
const placeUserMessage = (msg: ChatMessage): void => {
  const list = messages.value;
  const target = userMessageInsertIndex(msg.id);
  const current = list.indexOf(msg);
  if (current < 0) {
    if (target >= 0) {
      list.splice(target, 0, msg);
    } else {
      list.push(msg);
    }
    return;
  }
  if (target >= 0 && current > target) {
    list.splice(current, 1);
    list.splice(target, 0, msg);
  }
};

// adoptAssistantUserMessage 用 turn.started 的用户消息事实替换本地回显（按消息主键去重）。
const adoptAssistantUserMessage = (
  user: NonNullable<AssistantEventOf<"turn.started">["payload"]["userMessage"]>,
): void => {
  const existing = messages.value.find((msg) => msg.role === "user" && msg.id === user.messageID);
  if (existing) {
    existing.content = user.content;
    existing.createdAt = user.createdAt;
    existing.skillID = user.skillID;
    existing.skillName = user.skillName;
    placeUserMessage(existing);
    pendingUserEcho = null;
    return;
  }
  if (pendingUserEcho) {
    pendingUserEcho.id = user.messageID;
    pendingUserEcho.content = user.content;
    pendingUserEcho.createdAt = user.createdAt;
    pendingUserEcho.skillID = user.skillID;
    pendingUserEcho.skillName = user.skillName;
    placeUserMessage(pendingUserEcho);
    pendingUserEcho = null;
    return;
  }
  const last = messages.value[messages.value.length - 1];
  if (last?.role === "user" && last.id === undefined && last.content === user.content) {
    last.id = user.messageID;
    last.createdAt = user.createdAt;
    last.skillID = user.skillID;
    last.skillName = user.skillName;
    placeUserMessage(last);
    return;
  }
  placeUserMessage({
    key: nextKey(),
    role: "user",
    content: user.content,
    createdAt: user.createdAt,
    id: user.messageID,
    skillID: user.skillID,
    skillName: user.skillName,
  });
};

// applyAssistantTurnStarted 应用 turn.started：新提问替换本地回显并重置块基线；续跑承接锚点消息
// 与既有块序号，避免续跑内容落到上一条已完成的助手消息上。
const applyAssistantTurnStarted = (
  sessionID: number,
  event: AssistantEventOf<"turn.started">,
): void => {
  const payload = event.payload;
  agentRun.restore(sessionID, { runID: payload.runID, pendingCalls: [] });
  agentRun.setState(sessionID, STATE_RUNNING, "");
  cancelRequested = false;
  if (payload.resumed) {
    const target = assistantMessageFor(payload.assistantMessageID);
    if (!target) {
      return;
    }
    activeAssistantKey = target.key;
    agentRun.adoptAnchorBlocks(sessionID, target.blocks ?? []);
    bindAssistantBlocks(sessionID, target);
    target.streaming = true;
    return;
  }
  // 新回合开始：停止上一轮残留的流式态。仅当确属历史回放（无预置气泡）时把 activeAssistantKey
  // 置空、由 message.started 新建气泡；实时新提问（startStream 已预置气泡）保留该气泡。
  const replayingHistory = historyLoading && activeAssistantKey === null;
  const keepKey = replayingHistory ? null : activeAssistantKey;
  for (const msg of messages.value) {
    if (msg.streaming && msg.key !== keepKey) {
      msg.streaming = false;
      endReasoningStreamingOf(msg);
    }
  }
  if (replayingHistory) {
    activeAssistantKey = null;
  }
  if (payload.userMessage) {
    adoptAssistantUserMessage(payload.userMessage);
  }
  agentRun.adoptAnchorBlocks(sessionID, []);
};

// applyAssistantMessageStarted 应用 message.started：新回合新建气泡，续跑复用锚点气泡。
const applyAssistantMessageStarted = (
  sessionID: number,
  event: AssistantEventOf<"message.started">,
): void => {
  const payload = event.payload;
  let target = assistantMessageFor(payload.messageID);
  if (!target) {
    target = { key: nextKey(), role: "assistant", content: "", streaming: true };
    messages.value.push(target);
  }
  activeAssistantKey = target.key;
  target.streaming = true;
  if (payload.messageID) {
    target.id = payload.messageID;
  }
  bindAssistantBlocks(sessionID, target);
  scrollToBottom();
};

// applyAssistantBlockEvent 应用块事件：创建 / 增量 / 收敛与单条工具调用更新统一写入 store 的
// 运行块数组，再同步到承载消息。
const applyAssistantBlockEvent = (
  sessionID: number,
  event: AssistantEventOf<"block.created" | "block.delta" | "block.updated" | "tool.call.updated">,
): void => {
  if (event.type === "block.created") {
    const payload = event.payload;
    agentRun.upsertBlock(sessionID, {
      seq: payload.blockSeq,
      kind: payload.kind,
      payload: payload.payload,
      created: true,
    });
  } else if (event.type === "block.updated") {
    const payload = event.payload;
    agentRun.upsertBlock(sessionID, {
      seq: payload.blockSeq,
      kind: payload.kind,
      payload: payload.payload,
      created: false,
    });
  } else if (event.type === "block.delta") {
    agentRun.appendBlockText(sessionID, event.payload.kind, event.payload.text);
  } else {
    agentRun.upsertToolCall(sessionID, event.payload.blockSeq, event.payload.call);
  }
  const target = activeAssistantMessage();
  if (target) {
    bindAssistantBlocks(sessionID, target);
    syncAssistantContent(target);
  }
  if (activeAssistantKey !== null) {
    scrollReasoningToBottom(activeAssistantKey);
  }
  scrollToBottom(false);
};

// applyAssistantInteractionWaiting 应用 interaction.waiting：进入等待态并从块反推待处理调用，
// 使审批 / 问卷面板可交互（服务端不再下发 tool_request）。
const applyAssistantInteractionWaiting = (
  sessionID: number,
  event: AssistantEventOf<"interaction.waiting">,
): void => {
  const payload = event.payload;
  const interruptIDs = payload.interruptIDs ?? [];
  agentRun.setState(sessionID, STATE_WAITING, payload.substate);
  activeInterrupt = {
    interruptId: interruptIDs[0],
    interruptIds: [...interruptIDs],
    questionnaire: payload.substate === "answer",
  };
  const target = assistantMessageFor(event.messageID);
  if (!target) {
    return;
  }
  target.streaming = false;
  endReasoningStreamingOf(target);
  const parsed = pendingCallsOfMessage(target, interruptIDs, payload.blockSeq);
  target.pendingCalls = mergePendingCalls(parsed, target.pendingCalls);
  // 免审批调用已预置批准决策：整批都无需人工决策时不再处于「等待授权」，
  // 流结束后由 submitDecisions 直接执行并回传续跑
  target.awaitingApproval = target.pendingCalls.some((call) => !call.decision);
  syncAssistantContent(target);
};

// applyAssistantTurnCompleted 应用 turn.completed：收敛流式态、回填消息 ID 与用量并刷新会话快照。
const applyAssistantTurnCompleted = (
  sessionID: number,
  event: AssistantEventOf<"turn.completed">,
): void => {
  const payload = event.payload;
  const target = assistantMessageFor(event.messageID ?? payload.messageID);
  if (target) {
    target.streaming = false;
    endReasoningStreamingOf(target);
    if (payload.messageID) {
      target.id = payload.messageID;
    }
    if (payload.usage) {
      target.promptTokens = payload.usage.promptTokens;
      target.completionTokens = payload.usage.completionTokens;
      target.totalTokens = payload.usage.totalTokens;
      target.elapsedMs = payload.usage.elapsedMs;
    }
    target.phase = payload.phase;
  }
  agentRun.settle(sessionID);
  activeInterrupt = null;
  void loadSessions();
};

// applyAssistantEvent 分派一条事件：快照 → 消息列表，实时 → store 块 + 承载消息。
const applyAssistantEvent = (sessionID: number, event: AssistantEventUnion): void => {
  switch (event.type) {
    case "snapshot.begin":
      messages.value = [];
      activeAssistantKey = null;
      pendingUserEcho = null;
      questionnaireOverrides.clear();
      approvalOverrides.clear();
      activeInterrupt = null;
      return;
    case "snapshot.message": {
      const message = chatMessageOfSnapshot(event.payload.message);
      messages.value.push(message);
      // 历史回放同样按主键归位用户消息，避免损坏的旧数据把提问排到回答之后
      if (message.role === "user") {
        placeUserMessage(message);
      }
      return;
    }
    case "snapshot.end":
      scrollToBottom();
      return;
    case "resume.begin":
      agentRun.restore(sessionID, { runID: event.payload.runID, pendingCalls: [] });
      return;
    case "resume.reset":
      if (!historyLoading) {
        void loadHistory(sessionID);
      }
      return;
    case "turn.started":
      applyAssistantTurnStarted(sessionID, event);
      return;
    case "turn.preempted":
      // 旧回合被新提问抢占：清除其全部残留流式态。被动订阅方（同一会话的另一标签页）的
      // activeAssistantKey 指向的正是这条旧气泡，故不能按「非 active」过滤，否则会留下
      // "思考中/正文生成中" 的假流式；抢占方标签页订阅的是新回合、收不到本事件，不会被清。
      for (const msg of messages.value) {
        if (msg.streaming) {
          msg.streaming = false;
          endReasoningStreamingOf(msg);
        }
      }
      activeAssistantKey = null;
      // 本连接订阅的是被抢占的旧回合，此后不会再有 turn.completed 到达；必须在此收敛本地运行态，
      // 否则被动订阅方会永久停留在「生成中」并保留可用的停止按钮。
      agentRun.settle(sessionID);
      return;
    case "message.started":
      applyAssistantMessageStarted(sessionID, event);
      return;
    case "turn.error":
      if (event.payload.code === "retrying") {
        // 非终态提示：模型调用将自动重试，不收敛运行态。
        return;
      }
      {
        const target = assistantMessageFor(event.messageID);
        if (target) {
          target.streaming = false;
          target.phase = "failed";
          target.turnError = event.payload.message || undefined;
          endReasoningStreamingOf(target);
        }
        agentRun.settle(sessionID);
        if (!cancelRequested) {
          notifyError(event.payload.message || i18n.global.t("page.aiAssistant.error"));
        }
      }
      return;
    case "block.created":
    case "block.delta":
    case "block.updated":
    case "tool.call.updated":
      applyAssistantBlockEvent(sessionID, event);
      return;
    case "message.citations": {
      const target = assistantMessageFor(event.messageID);
      if (target) {
        target.citations = event.payload.citations;
      }
      return;
    }
    case "message.compress": {
      const target = assistantMessageFor(event.messageID);
      if (!target) {
        return;
      }
      const compress = event.payload.compress;
      const previous = target.compress;
      // compress_start 携带推进原因与前值、无增量；compress_delta 只带增量。增量按序累积，
      // 否则每次事件覆盖会把正文缩成最后一段（表现为只显示一行）。
      const starting = compress.phase === "running" && Boolean(compress.reason) && !compress.draft;
      const draft =
        compress.phase === "running"
          ? starting
            ? ""
            : `${previous?.draft ?? ""}${compress.draft ?? ""}`
          : (previous?.draft ?? "");
      target.compress = {
        phase: compress.phase,
        draft,
        // 终态优先用后端随 compress_done 下发的全文；缺省时保留已累积的增量，保证完成后仍可展开
        summary:
          compress.summary ?? (compress.phase === "running" ? "" : previous?.summary || draft),
        reason: (compress.reason ?? "manual") as CompressState["reason"],
        tokensBefore: compress.tokensBefore,
        foldedCount: compress.foldedCount,
        tokensAfter: compress.tokensAfter,
      };
      // 压缩完成 / 失败即自动收起，回到三行截断态（用户仍可再次点击展开）
      if (compress.phase !== "running") {
        expandedCompressKeys.value.delete(target.key);
      }
      return;
    }
    case "interaction.waiting":
      applyAssistantInteractionWaiting(sessionID, event);
      return;
    case "interaction.resumed":
      activeInterrupt = null;
      agentRun.setState(sessionID, STATE_RUNNING, "");
      {
        const target = assistantMessageFor(event.messageID);
        if (target) {
          target.streaming = true;
          target.awaitingApproval = false;
        }
      }
      return;
    case "message.persisted": {
      const target = assistantMessageFor(event.messageID);
      if (target && event.payload.messageID) {
        target.id = event.payload.messageID;
      }
      return;
    }
    case "turn.completed":
      applyAssistantTurnCompleted(sessionID, event);
      return;
    default:
      // 未知事件按前向兼容约定忽略。
      return;
  }
};

// consumeAssistantStream 消费一条实时流（对话 / 审批续跑 / 事件续播共用）：
// 逐条应用领域事件；EOF 后按是否挂起决定自动提交或收敛运行态。
const consumeAssistantStream = (
  sessionID: number,
  start: (handlers: AssistantStreamHandlers) => () => void,
  opts?: { reloadOnEmpty?: boolean },
): void => {
  cancelRequested = false;
  let unsubscribe: (() => void) | null = null;
  const releaseStreamHandle = (): void => {
    if (unsubscribe) {
      agentRun.release(sessionID, unsubscribe);
    }
  };
  let waiting = false;
  let completed = false;
  let preempted = false;
  let received = false;
  unsubscribe = start({
    onSeq: (seq: number) => agentRun.noteSeq(sessionID, seq),
    onEvent: (event) => {
      received = true;
      if (event.type === "interaction.waiting") {
        waiting = true;
      } else if (event.type === "turn.completed") {
        completed = true;
      } else if (event.type === "turn.preempted") {
        preempted = true;
      }
      applyAssistantEvent(sessionID, event);
    },
    onComplete: () => {
      releaseStreamHandle();
      if (preempted) {
        return;
      }
      if (!received && opts?.reloadOnEmpty) {
        // /events 返回 204：运行恰在 render 与续播之间完成，历史快照未包含终态，重新回放一次。
        void loadHistory(sessionID);
        return;
      }
      const msg = activeAssistantMessage();
      if (waiting) {
        void loadSessions();
        // 免审批调用（approvalRequired=false）已预置决策，但 api_invoke 仍需前端执行：
        // 流结束后立即执行并整体回传续跑，否则会一直停在「等待授权」。
        if (msg) {
          submitDecisions(msg);
        }
        return;
      }
      if (!completed) {
        if (msg) {
          msg.streaming = false;
          endReasoningStreamingOf(msg);
        }
        agentRun.settle(sessionID);
      }
      const submitted = msg ? trySubmitApprovals(msg) : false;
      if (!submitted) {
        drainQueuedQuery(sessionID);
      }
    },
    onError: (text: string) => {
      releaseStreamHandle();
      agentRun.settle(sessionID);
      const msg = activeAssistantMessage();
      if (msg) {
        msg.streaming = false;
        msg.awaitingApproval = false;
        endReasoningStreamingOf(msg);
        msg.phase = "failed";
        msg.turnError = text || undefined;
      }
      if (!cancelRequested) {
        notifyError(text || i18n.global.t("page.aiAssistant.error"));
      }
    },
  });
  agentRun.attach(sessionID, unsubscribe);
};

// prepareAssistantContinuation 续跑前把承载消息的块写入 store 并重置传输游标：续跑的块序号
// 从挂起现场继续（服务端承接），而事件 seq 重新从 1 计数。
const prepareAssistantContinuation = (sessionID: number, msg: ChatMessage): void => {
  agentRun.adoptAnchorBlocks(sessionID, msg.blocks ?? []);
  agentRun.resetSeq(sessionID);
  bindAssistantBlocks(sessionID, msg);
};

// ---- 人工审批（决策 + 结果回传）----

// trySubmitApprovals 当本轮（尚未回传的）待审批调用全部已决策时，一次性回传全部决策并续跑；
// 后端把挂起现场一次性消费，故同一轮必须整体提交，不能逐条提交。仅在有挂起现场（interrupt）
// 时提交：免审批调用的 awaitingApproval 会被卡片状态推导为 false，不能作为提交闸门。返回是否已提交。
const trySubmitApprovals = (msg: ChatMessage): boolean => {
  const sessionID = activeSessionID.value;
  const interrupt = currentInterrupt(msg);
  if (sessionID === null || !interrupt) {
    return false;
  }
  const batch = (msg.pendingCalls ?? []).filter((c) => c.submitted !== true);
  if (batch.length === 0 || batch.some((c) => !c.decision)) {
    return false;
  }
  // 前端调用仍在执行（api_invoke）：等结果写入后再提交，避免把空 result 回传给模型。
  if (batch.some((c) => c.executing === true)) {
    return false;
  }
  const decisions: AssistantApprovalDecision[] = batch.map((c) => ({
    callID: c.callID,
    interruptId: c.interruptId,
    approved: c.decision === "approved",
    toolName: c.toolName,
    arguments: c.arguments,
    execOwner: c.execOwner,
    approvalRequired: c.approvalRequired,
    result: c.result ?? "",
    error: c.error ?? "",
  }));
  batch.forEach((c) => {
    c.submitted = true;
  });
  // 本地同时把决策写回审批块：冷续跑（进程重启后挂起现场无内存块定位）后端不会回推 block.updated，
  // 面板仍应即时显示已批准 / 已拒绝。
  applyApprovalDecisions(msg.blocks ?? [], batch);
  msg.awaitingApproval = false;
  // 续跑同样是一次在途生成：恢复消息的流式态，否则续跑期间的思考块会把标题误判为「思考完成」、
  // 图标也不进入运行态（流结束时由流消费者统一收敛为 false）。
  msg.streaming = true;
  // 挂起提示随即收起；续跑若再次挂起，服务端会重新下发等待事件。
  clearInterrupt();
  activeAssistantKey = msg.key;
  agentRun.setState(sessionID, STATE_RUNNING, "");
  const request: AssistantActionRequest = {
    action: "approval.decide",
    interruptId: interrupt?.interruptId || undefined,
    interruptIds: interrupt?.interruptIds,
    decisions,
  };
  prepareAssistantContinuation(sessionID, msg);
  consumeAssistantStream(sessionID, (handlers) =>
    assistantActionStream(sessionID, request, handlers),
  );
  return true;
};

// syncCallResultToBlock 把前端调用的执行结果 / 错误同步到审批块，面板内即时可见；同时写入本地审批
// 收敛态，使重建后结果不丢失。
const syncCallResultToBlock = (msg: ChatMessage, call: PendingCallState): void => {
  if (call.decision) {
    approvalOverrides.set(call.callID, {
      decision: call.decision,
      result: call.result,
      error: call.error,
    });
  }
  for (const b of msg.blocks ?? []) {
    if (b.kind !== "approval") {
      continue;
    }
    const rec = (b.payload as AgentApprovalBlockPayload)?.calls?.find(
      (c) => c.callID === call.callID,
    );
    if (rec) {
      rec.result = call.result;
      rec.error = call.error;
    }
  }
};

// submitDecisions 回传本轮审批决策：先执行尚未执行的前端调用（api_invoke），再整体提交。
// 免审批调用（approvalRequired=false）已有预置决策但同样需要前端执行，因此不能只依赖用户点击。
const submitDecisions = (msg: ChatMessage): void => {
  const pending = (msg.pendingCalls ?? []).filter(
    (c) =>
      c.submitted !== true &&
      c.decision === "approved" &&
      c.toolName === API_INVOKE_TOOL_NAME &&
      c.result === undefined &&
      c.error === undefined &&
      c.executing !== true,
  );
  if (pending.length === 0) {
    trySubmitApprovals(msg);
    return;
  }
  void (async () => {
    for (const call of pending) {
      await executeApiInvoke(call);
      syncCallResultToBlock(msg, call);
    }
    trySubmitApprovals(msg);
  })();
};

// setApprovalDecision 记录某条待审批调用的决策并尝试提交本轮结果：同时更新 pendingCalls 中该调用的
// decision（提交续跑据其合成结果）与 approval 块内同名记录的 decision（面板徽章即时反映决策——后端
// 从不回写 approval 块的 Decision，故仅本地更新）。
const setApprovalDecision = (msg: ChatMessage, callID: string, approved: boolean): void => {
  const call = (msg.pendingCalls ?? []).find((c) => c.callID === callID);
  if (!call || call.decision) {
    return;
  }
  call.decision = approved ? "approved" : "rejected";
  // 本地收敛态按 callID 留底：挂起卡片随后的重建即使覆盖 pendingCalls 视图态，徽章与等待提示也不回退。
  approvalOverrides.set(callID, {
    decision: call.decision,
    result: call.result,
    error: call.error,
  });
  for (const b of msg.blocks ?? []) {
    if (b.kind !== "approval") {
      continue;
    }
    const rec = (b.payload as AgentApprovalBlockPayload)?.calls?.find((c) => c.callID === callID);
    if (rec) {
      rec.decision = approved;
      // api_invoke 由前端执行：把执行结果 / 错误同步到审批块，批准后即可在面板内看到本次调用的产物。
      rec.result = call.result;
      rec.error = call.error;
    }
  }
  submitDecisions(msg);
};

// executingCallIDsOf 当前消息中正在前端执行的调用 ID：传给审批步骤组件驱动按钮 loading，
// 并随 pendingCalls 的执行态变化重渲染
const executingCallIDsOf = (msg: ChatMessage): string[] =>
  (msg.pendingCalls ?? []).filter((c) => c.executing === true).map((c) => c.callID);

// approveCall 批准该调用（按 callID 定位）：api_invoke 先在前端执行只读接口，执行结束（无论成功
// 或失败）再记录决策并尝试提交本轮结果；其余 client 工具无需本地执行，直接记录决策。
const approveCall = (msg: ChatMessage, callID: string): void => {
  const call = (msg.pendingCalls ?? []).find((c) => c.callID === callID);
  if (!call || call.decision || call.executing) {
    return;
  }
  if (call.toolName === API_INVOKE_TOOL_NAME) {
    void (async () => {
      await executeApiInvoke(call);
      setApprovalDecision(msg, callID, true);
    })();
    return;
  }
  setApprovalDecision(msg, callID, true);
};

// ApiInvokeArgs 是后端解析后的规范入参：method/path 已按能力白名单确定，前端不再接受模型给出的
// 任意 URL；params 为请求体对象。
interface ApiInvokeArgs {
  capability?: string;
  method: string;
  path: string;
  params?: unknown;
}

// parseApiInvokeArgs 解析后端下发的规范入参；缺少 method/path 时返回 null。
const parseApiInvokeArgs = (argumentsText: string): ApiInvokeArgs | null => {
  try {
    const parsed = JSON.parse(argumentsText) as Partial<ApiInvokeArgs>;
    if (typeof parsed.method !== "string" || typeof parsed.path !== "string") {
      return null;
    }
    return {
      capability: typeof parsed.capability === "string" ? parsed.capability : undefined,
      method: parsed.method,
      path: parsed.path,
      params: parsed.params,
    };
  } catch {
    return null;
  }
};

// truncateApiInvokeResult 把序列化后的响应截断到上限，保留明确截断标记。
const truncateApiInvokeResult = (text: string): string =>
  text.length > API_INVOKE_RESULT_MAX_CHARS
    ? `${text.slice(0, API_INVOKE_RESULT_MAX_CHARS)}\n…（结果已截断）`
    : text;

// apiInvokeErrorText 还原能力调用的失败原因：后端统一错误封套（code / summary / details）以缩进
// JSON 原样保留，避免拼接成单行后丢失字段层级与原始类型；响应体不是结构化对象时回落到响应原文，
// 再无响应体时才使用 axios 的 HTTP 状态码消息（如 "Request failed with status code 400"）。
const apiInvokeErrorText = (err: unknown): string => {
  if (axios.isAxiosError(err) && err.response) {
    const body = err.response.data;
    if (typeof body === "string" && body.trim() !== "") {
      return body;
    }
    if (body !== null && typeof body === "object") {
      return JSON.stringify(body, null, 2);
    }
  }
  return err instanceof Error ? err.message : String(err);
};

// executeApiInvoke 在用户自身凭据下执行一次已解析的只读调用：成功时把响应体序列化写入 result，
// 失败时把错误文本写入 error；两种情况均由后续决策提交回传，供模型继续推理。
const executeApiInvoke = async (call: PendingCallState): Promise<void> => {
  const args = parseApiInvokeArgs(call.arguments);
  if (!args) {
    call.error = i18n.global.t("page.aiAssistant.apiInvokeInvalidArgs");
    return;
  }
  call.executing = true;
  call.error = undefined;
  try {
    const response = await request.request({
      // 后端 path 带 /api/v1，baseURL 也含 /api/v1；经 apiRequestURL 重建绝对地址，避免二次拼接。
      url: apiRequestURL(args.path),
      method: args.method,
      data: args.params ?? {},
      // 与其余业务请求一致携带当前用户 JWT：能力调用以用户自身身份执行，后端鉴权照常生效。
      headers: getJwtHeader(),
      // 平台列表接口可能超过全局 1s 超时；能力调用给足 60s（如 ai.skill.create 会在后端同步
      // 调用 LLM 评审正文），仍由后端鉴权与响应上限兜底。
      timeout: 60_000,
    });
    call.result = truncateApiInvokeResult(JSON.stringify(response.data ?? ""));
  } catch (err) {
    call.error = apiInvokeErrorText(err);
  } finally {
    call.executing = false;
  }
};

// rejectCall 拒绝（按 callID 定位）：仅记录决策，不执行任何调用
const rejectCall = (msg: ChatMessage, callID: string): void =>
  setApprovalDecision(msg, callID, false);

// ---- 主动提问（问卷）----
// awaitingQuestionnaireMsg 当前会话中仍待回答的问卷所在助手消息（取最近一条）：非空时该轮问卷已内联在
// 消息流中（见 assistantBlocksOf 的 questionnaire 块），会话阻塞直至用户提交或取消回答。后端保证仅
// 「末轮」可能仍待回答，故这条消息中仍待回答的那一轮恒为其各轮问卷的最后一轮——提交 / 取消据此定位。
const awaitingQuestionnaireMsg = computed<ChatMessage | null>(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    if (awaitingQuestionnaireBlockOf(messages.value[i])) {
      return messages.value[i];
    }
  }
  return null;
});

// questionnaireInteractive 某条消息的问卷面板是否可交互：仍待回答且消息上登记了「未决策的 ask_user
// 调用」时可提交 / 取消（callID 为提交续跑所必需）；现场不可用（已消费 / 已过期 / 未随消息带回）时
// 降级为只读展示，避免出现可点击却无任何反应、也不发请求的假交互
const questionnaireInteractive = (m: ChatMessage | null): boolean =>
  m !== null &&
  (m.pendingCalls ?? []).some((c) => c.toolName === ASK_USER_TOOL_NAME && !c.decision);

// questionnairePayloadOf 取某一轮问卷的提交载荷（题目 + 上次答案），供问卷面板渲染与预填
const questionnairePayloadOf = (q: AgentQuestionnaireBlockPayload): QuestionnairePayload => ({
  questions: q.questions,
  answers: q.answers,
});

// questionnaireAnswerTextOf 取某题的用户作答展示文本：已作答取答案文本，未作答取本地化「未作答」
const questionnaireAnswerTextOf = (
  q: AgentQuestionnaireBlockPayload,
  questionID: string,
): string => {
  const ans = q.answers?.find((a) => a.questionID === questionID);
  if (!ans || !ans.answered) {
    return i18n.global.t("page.aiAssistant.questionnaireNoAnswer");
  }
  return ans.answer;
};

// submitQuestionnaire 提交问卷：以 questionnaire.submit 动作回传作答并续跑。题目 ID 与服务端
// ask_user 挂起现场的题目 ID 一致，故无需再按调用拆分答案。
// 末轮问卷块被乐观地原位更新为已作答（awaiting=false + answers）：问卷收敛不回推块更新，
// 前端经 questionnaireOverrides 记住本地收敛态，避免下一次重建把面板又展开。
const submitQuestionnaire = (payload: { answers: QuestionnaireAnswer[] }): void => {
  const msg = awaitingQuestionnaireMsg.value;
  const sessionID = activeSessionID.value;
  if (!msg || sessionID === null) {
    return;
  }
  const block = awaitingQuestionnaireBlockOf(msg);
  if (!block) {
    return;
  }
  const questionnaire = block.payload as AgentQuestionnaireBlockPayload;
  questionnaireOverrides.set(block.seq - 1, { answers: payload.answers, awaiting: false });
  questionnaire.answers = payload.answers;
  questionnaire.awaiting = false;
  for (const call of msg.pendingCalls ?? []) {
    if (call.toolName === ASK_USER_TOOL_NAME && !call.decision) {
      call.decision = "approved";
      call.submitted = true;
    }
  }
  msg.awaitingApproval = false;
  // 作答续跑同样是在途生成：恢复流式态，避免续跑期间的思考块被误判为已完成。
  msg.streaming = true;
  const interrupt = currentInterrupt(msg);
  clearInterrupt();
  activeAssistantKey = msg.key;
  agentRun.setState(sessionID, STATE_RUNNING, "");
  const answers: AssistantQuestionnaireAnswer[] = payload.answers.map((a) => ({
    questionID: a.questionID,
    answered: a.answered,
    answer: a.answer,
    custom: a.custom,
  }));
  const request: AssistantActionRequest = {
    action: "questionnaire.submit",
    interruptId: interrupt?.interruptId || undefined,
    interruptIds: interrupt?.interruptIds,
    answers,
  };
  prepareAssistantContinuation(sessionID, msg);
  consumeAssistantStream(sessionID, (handlers) =>
    assistantActionStream(sessionID, request, handlers),
  );
};

// cancelQuestionnaire 取消回答：以全未作答的 questionnaire.submit 回传（服务端按「无有效答案」
// 判定为拒绝），并收起末轮问卷面板。须一并清空末轮的用户答案——取消后应回到未作答状态。
const cancelQuestionnaire = (): void => {
  const msg = awaitingQuestionnaireMsg.value;
  const sessionID = activeSessionID.value;
  if (!msg || sessionID === null) {
    return;
  }
  const block = awaitingQuestionnaireBlockOf(msg);
  if (!block) {
    return;
  }
  const questionnaire = block.payload as AgentQuestionnaireBlockPayload;
  questionnaireOverrides.set(block.seq - 1, { awaiting: false });
  questionnaire.awaiting = false;
  questionnaire.answers = undefined;
  for (const call of msg.pendingCalls ?? []) {
    if (call.toolName === ASK_USER_TOOL_NAME && !call.decision) {
      call.decision = "rejected";
      call.submitted = true;
    }
  }
  msg.awaitingApproval = false;
  // 取消作答同样会续跑（按「无有效答案」回传）：恢复流式态，避免续跑期间的思考块被误判为已完成。
  msg.streaming = true;
  const interrupt = currentInterrupt(msg);
  clearInterrupt();
  activeAssistantKey = msg.key;
  agentRun.setState(sessionID, STATE_RUNNING, "");
  const answers: AssistantQuestionnaireAnswer[] = questionnaire.questions.map((q) => ({
    questionID: q.id,
    answered: false,
    answer: "",
  }));
  const request: AssistantActionRequest = {
    action: "questionnaire.submit",
    interruptId: interrupt?.interruptId || undefined,
    interruptIds: interrupt?.interruptIds,
    answers,
  };
  prepareAssistantContinuation(sessionID, msg);
  consumeAssistantStream(sessionID, (handlers) =>
    assistantActionStream(sessionID, request, handlers),
  );
};

// 待确认回退的交互种类与定位 ID：确认后由后端删除该交互之后的回答并重建挂起现场，故须先经弹窗
// 提醒。审批回退按调用 ID 定位交互，问卷回退按根中断 ID 定位（前端不持有交互主键）。
const pendingRerunKind = ref<"answer" | "approval">("answer");
const pendingRerunCallIDs = ref<string[]>([]);
const pendingRerunInterruptIDs = ref<string[]>([]);
const rerunConfirmRef = useTemplateRef("rerunConfirmRef");

// RerunTarget 回退定位参数：审批给 callIDs、问卷给 interruptIDs（由卡片元信息下发）。
interface RerunTarget {
  callIDs?: string[];
  interruptIDs?: string[];
}

// openRerunConfirm 打开「回退到待处理交互」确认弹窗（重新回答 / 重新审批按钮的入口）：忙或定位 ID
// 缺失时不进入确认。确认后审批与问卷都回退到该次交互挂起的待处理状态，不重跑整轮回答。
const openRerunConfirm = (kind: "answer" | "approval", target: RerunTarget): void => {
  if (busy.value) {
    return;
  }
  if (kind === "approval") {
    const callIDs = target.callIDs ?? [];
    if (callIDs.length === 0) {
      return;
    }
    pendingRerunCallIDs.value = callIDs;
    pendingRerunInterruptIDs.value = [];
  } else {
    const interruptIDs = target.interruptIDs ?? [];
    if (interruptIDs.length === 0) {
      return;
    }
    pendingRerunInterruptIDs.value = interruptIDs;
    pendingRerunCallIDs.value = [];
  }
  pendingRerunKind.value = kind;
  rerunConfirmRef.value?.open({});
};

// confirmRerun 确认回退：审批 / 问卷都由后端把会话回退到该次交互的待处理状态（删除该交互之后的
// 回答并重建挂起现场），前端重放 /render 后重新展示批准 / 拒绝按钮或待作答问卷。
const confirmRerun = async (): Promise<void> => {
  const kind = pendingRerunKind.value;
  const callIDs = pendingRerunCallIDs.value;
  const interruptIDs = pendingRerunInterruptIDs.value;
  pendingRerunCallIDs.value = [];
  pendingRerunInterruptIDs.value = [];
  const sessionID = activeSessionID.value;
  if (sessionID === null) {
    return;
  }
  const target = kind === "approval" ? { callIDs } : { interruptIDs };
  if (kind === "approval" ? callIDs.length === 0 : interruptIDs.length === 0) {
    return;
  }
  await getSimpleResponse(
    agentInteractionReopen(sessionID, target),
    async (rsp) => {
      // 回退后由 /render 重放历史：交互之后的回答已删除，交互卡片回到待处理（按钮 / 问卷重现）。
      agentRun.settle(sessionID);
      await loadSessions();
      await loadHistory(sessionID);
      await syncSessionState(sessionID);
      appStore.notify({ text: rsp.summary, timeout: 3000, color: "success" });
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// startStream 推送用户消息与流式占位消息并发起一轮对话：领域事件落到消息块模型；
// 挂起时由 interaction.waiting 转为等待审批 / 作答。
// chat 流回放的历史不包含本轮提问，故用户气泡由前端本地补出（pendingUserEcho）。
// echoedUserKey 为首条消息在「创建会话」等待期已本地回显的气泡 key（见 echoUserQuery）：
// 气泡仍在时直接复用，避免同一提问出现两次；若等待期消息区被清空（如创建途中点
// 「新建会话」）致回显丢失，则回落为按常规补推用户气泡，保证会话内容与服务端一致
const startStream = (
  sessionID: number,
  q: string,
  echoedUserKey?: number,
  skill?: SkillOption | null,
): void => {
  const echoed =
    echoedUserKey !== undefined ? messages.value.find((m) => m.key === echoedUserKey) : undefined;
  const echo: ChatMessage = echoed ?? {
    key: nextKey(),
    role: "user",
    content: q,
    createdAt: new Date().toISOString(),
  };
  // 技能随气泡记录：气泡顶部据此展示本轮技能，回退时也从同一字段回填输入区
  echo.skillID = skill?.id;
  echo.skillName = skill?.name;
  if (!echoed) {
    messages.value.push(echo);
  }
  pendingUserEcho = echo;
  const assistantKey = nextKey();
  activeAssistantKey = assistantKey;
  messages.value.push({ key: assistantKey, role: "assistant", content: "", streaming: true });
  query.value = "";
  scrollToBottom();
  // 新运行开始：重置该会话的运行态与续播游标，并立即置为运行中以闭合闸门空档
  agentRun.begin(sessionID);
  const chatRequest: AgentChatIn = {
    query: q,
    // 本次生成选中的技能 ID：后端据此解析内置/自定义技能并把正文装配进系统提示
    //（未选技能时不带该字段）
    skillID: skill?.id,
    // 本次生成的显式档位（为空时由后端回退到会话档位与默认档位参数）
    slot: slot.value || undefined,
    // 思考开关：仅当当前档位绑定模型支持时注入（开启 on / 关闭 off），不支持则不带该字段
    thinking: slotThinkingToggle.value ? (thinking.value ? "on" : "off") : undefined,
  };
  consumeAssistantStream(sessionID, (handlers) =>
    assistantChatStream(sessionID, chatRequest, handlers),
  );
};

// echoUserQuery 首条消息本地回显：在「创建会话」等待期先把用户气泡推进消息区并清空输入框。
// 后端创建会话需同步调用模型生成主题、耗时明显，若等创建成功再推气泡，等待期消息区毫无反馈，
// 用户会误以为回车未被受理；返回该气泡 key，供创建未成功时回滚（见 rollbackUserQuery）
const echoUserQuery = (q: string, skill?: SkillOption | null): number => {
  const key = nextKey();
  messages.value.push({
    key,
    role: "user",
    content: q,
    createdAt: new Date().toISOString(),
    skillID: skill?.id,
    skillName: skill?.name,
  });
  query.value = "";
  scrollToBottom();
  return key;
};

// rollbackUserQuery 创建会话未成功时回滚本地回显：撤回该用户气泡并把正文还原到输入框，
// 既不留下一条实际未发出的消息，也不丢失用户刚输入的内容
const rollbackUserQuery = (key: number, q: string): void => {
  messages.value = messages.value.filter((m) => m.key !== key);
  query.value = q;
};

// createSessionForQuery 未选中会话时据首个问题创建会话：主题由后端经 LLM 生成并随响应回传，
// 成功后立即发起本次流式生成；失败则中止本次发送并回滚本地回显的用户气泡（见 rollbackUserQuery）
const createSessionForQuery = async (
  firstQuery: string,
  userKey: number,
  skill?: SkillOption | null,
): Promise<void> => {
  creating.value = true;
  // created 会话是否创建成功（成功回调内已接续发起生成）：未成功（失败 / 无响应）时回滚回显，
  // 避免消息区留下一条并未真正发出的提问
  let created = false;
  try {
    await getResponse<AgentSessionItem>(
      agentSessionCreate({
        // 首个用户问题：后端据此经 LLM 生成会话主题（失败时按其截断兜底）
        query: firstQuery,
        // 会话缺省档位随当前档位下拉传入，作为后续生成的回退档位
        slot: slot.value || undefined,
        // 会话所属 Agent 类型：由状态接口下发的通用助手类型决定，前端不声明取值字面量
        agentKind: aiStatus.value?.assistantAgentKind || undefined,
        // 会话级审核阀门随当前下拉传入，作为该会话内调用的合成阀门
        reviewValve: reviewValve.value || undefined,
      }),
      async (rsp) => {
        created = true;
        sessions.value = [rsp.data, ...sessions.value];
        activeSessionID.value = rsp.data.id;
        draftMode.value = false;
        // 首条消息已本地回显：startStream 据其补流式占位消息，不再重复推送用户气泡
        startStream(rsp.data.id, firstQuery, userKey, skill);
      },
      async (rsp) => {
        notifyError(rsp.summary);
      },
      notifyNoResponse,
    );
  } finally {
    creating.value = false;
    if (!created) {
      rollbackUserQuery(userKey, firstQuery);
    }
  }
};

// composerComposing 输入框的输入法组字态：组字期间（含用回车确认候选词的那一次）一律不触发发送。
// Chrome / Firefox 确认候选词那次 keydown 的 isComposing 为 true，而 Safari 会先派发
// keydown（isComposing 为 false）再派发 compositionend，故另以该标记兜底。
const composerComposing = ref(false);

// onComposerEnter 输入框回车：组字期间的语义是「确认候选词」，此时既不发送也不拦截默认行为
// （让输入法正常上屏）；其余情况下拦截换行并触发发送（Shift+Enter 由 .exact 排除在外，用于换行）
const onComposerEnter = (event: KeyboardEvent): void => {
  if (event.isComposing || composerComposing.value) {
    return;
  }
  // 技能下拉可见时回车语义是「选中当前高亮技能」：只改写输入区，不发送提问
  if (skillMenuVisible.value) {
    event.preventDefault();
    selectSkillAt(skillActiveIndex.value);
    return;
  }
  event.preventDefault();
  // 优化请求在途时输入将被整体改写，此时任何提交都不成立（与 canSend 闸门一致），保持输入原样；
  // 其余情形与发送按钮同一入口（见 onSubmitClick）：运行中转入待发队列，被闸住时提示原因
  if (optimizing.value) {
    return;
  }
  onSubmitClick();
};

// sendQuery 发起一次流式问答：先校验输入，未选中会话时先本地回显再创建会话，
// 会话已存在时直接推送用户消息与流式占位消息
const sendQuery = async (): Promise<void> => {
  // 输入闸门关闭（运行中 / 等待中）时不接受新的生成请求：运行中由 onSubmitClick 转入待发队列，
  // 此处不会再被触达；已锁定时给出提示（须先回滚历史消息才能继续提问）；创建会话途中同理拒绝，
  // 避免同一问题重复建会话
  if (inputGated.value || creating.value) {
    if (locked.value) {
      notifyError(i18n.global.t("page.aiAssistant.lockedReject"));
    } else if (runActive.value) {
      notifyError(i18n.global.t("page.aiAssistant.stateGuardReject"));
    }
    return;
  }
  const q = query.value.trim();
  if (!q) {
    notifyError(i18n.global.t("page.aiAssistant.emptyQuery"));
    return;
  }
  // 消息已发出：撤回现场随之失效（正文在 startStream 清空，watch 亦会兜底）
  optimizeUndo.value = null;
  // 本次提问选中的技能：随请求透传，发送后再清除（避免请求发出前被清空导致技能丢失）
  const skill = selectedSkill.value;
  // 未选中会话（含点「新建会话」后的待创建态）：先据首个问题创建会话
  // （主题由后端生成并回传），创建成功后才发起流式生成。创建会话需后端同步调模型生成主题、
  // 耗时明显，故先本地回显用户气泡并让发送按钮转入加载态（见 canSend / creating），
  // 让「回车已被受理」在等待期即刻可见；创建未成功再回滚回显
  if (activeSessionID.value === null) {
    const userKey = echoUserQuery(q, skill);
    await createSessionForQuery(q, userKey, skill);
    selectedSkill.value = null;
    return;
  }
  startStream(activeSessionID.value, q, undefined, skill);
  selectedSkill.value = null;
};

// optimizePrompt 优化提问：调用提示词优化接口，把当前输入改写为更清晰、更易被正确理解的提问。
// 成功后整体替换输入框内容（不自动发送，交由用户确认）；空输入直接提示且不发请求
const optimizePrompt = async (): Promise<void> => {
  const query_ = query.value.trim();
  if (!query_) {
    notifyError(i18n.global.t("page.aiAssistant.emptyQuery"));
    return;
  }
  optimizing.value = true;
  try {
    await getResponse<AgentPromptOptimizeOut>(
      agentPromptOptimize({ query: query_ }),
      async (rsp) => {
        // 先登记撤回现场（优化前原文），再整体替换输入框：不自动发送，交由用户确认或撤回
        optimizeUndo.value = { origin: query_, optimized: rsp.data.query };
        query.value = rsp.data.query;
      },
      async (rsp) => {
        notifyError(rsp.summary || i18n.global.t("page.aiAssistant.optimizePromptFailed"));
      },
      notifyNoResponse,
    );
  } finally {
    optimizing.value = false;
  }
};

// undoOptimizePrompt 撤回优化：把输入框恢复为优化前原文，并作废撤回现场（按钮回到「优化提问」）。
// 先清现场再写回，避免 watch 因原文与优化结果不同而重复处理
const undoOptimizePrompt = (): void => {
  const undo = optimizeUndo.value;
  if (!undo) {
    return;
  }
  optimizeUndo.value = null;
  query.value = undo.origin;
};

// cancelGeneration 点「停止」：请求服务端中止当前 TurnLoop。运行中不退订流——cancelled 卡片需要
// 经当前订阅送达；等待审批 / 作答时本轮已无订阅，中止成功后由本地收敛为 cancelled。
const cancelGeneration = (): void => {
  const sessionID = activeSessionID.value;
  if (sessionID === null) {
    return;
  }
  // 等待审批 / 作答时本轮流已结束、没有可接收 cancelled 事件的订阅；中止成功后必须在本地
  // 收敛为 cancelled，否则界面会一直停在「等待授权」，直到刷新才看到取消结果。
  const wasWaiting = interactionState.value === STATE_WAITING;
  cancelRequested = true;
  clearInterrupt();
  const settleWaitingCancel = (): void => {
    if (!wasWaiting) {
      return;
    }
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const msg = messages.value[i];
      if (msg.role !== "assistant") {
        continue;
      }
      msg.streaming = false;
      msg.awaitingApproval = false;
      msg.phase = "cancelled";
      // 挂起现场已随中止作废：问卷退出待作答态，避免取消后还能继续提交到一个已取消的交互
      for (const block of msg.blocks ?? []) {
        if (block.kind !== "questionnaire") {
          continue;
        }
        const payload = block.payload as AgentQuestionnaireBlockPayload;
        if (payload) {
          payload.awaiting = false;
        }
      }
      msg.pendingCalls = undefined;
      endReasoningStreamingOf(msg);
      break;
    }
    agentRun.settle(sessionID);
    void loadSessions();
  };
  // 显式停止以服务端为准，客户端失败（如运行已结束）不阻断本地复位
  void agentAbort(sessionID)
    .then(settleWaitingCancel)
    .catch(() => undefined);
  // 停止后残留的流式占位消息停止闪烁，保留已生成的增量内容
  const last = messages.value[messages.value.length - 1];
  if (last && last.streaming) {
    last.streaming = false;
  }
};

// gotoLlmConfig 前往 LLM 配置页完成开通，并关闭抽屉让用户聚焦配置
const gotoLlmConfig = (): void => {
  opened.value = false;
  void router.push({ name: "Admin LLM Config", params: { language: route.params.language } });
};

// ---- 在途运行续播 ----
// fetchLiveRun 查询某会话的在途运行态（在跑 / 待审批）；无在途运行或查询失败时返回 null。
const fetchLiveRun = async (id: number): Promise<AgentLiveRunOut | null> => {
  let live: AgentLiveRunOut | null = null;
  await getResponse<AgentStatusOut>(agentStatusGet(id), async (rsp) => {
    live = rsp.data?.liveRun ?? null;
  });
  return live;
};

// syncSessionState 按状态接口的在途运行快照恢复会话交互状态：
//   - 运行中：登记运行标识，复用承载消息并订阅 /events 续接实时增量（render 已回放当前缓冲快照，
//     事件流从 0 重放同一批 envelope 是幂等的）；
//   - 等待中：渲染阶段已由挂起卡片 + interruptRequest 恢复可交互现场，此处只落快照；
//   - 无在途运行：以会话列表快照收敛本地运行态，避免残留「生成中」。
const syncSessionState = async (id: number): Promise<void> => {
  const live = await fetchLiveRun(id);
  if (!live || !live.runID) {
    const session = sessions.value.find((s) => s.id === id);
    if (session) {
      agentRun.setSnapshot(id, {
        state: session.interactionState,
        substate: session.interactionSubstate,
        locked: session.locked,
        userTurns: session.userTurns,
        maxUserTurns: session.maxUserTurns,
      });
    } else {
      agentRun.settle(id);
    }
    return;
  }
  agentRun.setSnapshot(id, {
    state: live.interactionState,
    substate: live.interactionSubstate,
    locked: live.locked,
    userTurns: live.userTurns,
    maxUserTurns: live.maxUserTurns,
  });
  agentRun.restore(id, { runID: live.runID, pendingCalls: [] });
  if (live.interactionState !== STATE_RUNNING) {
    // 等待态恢复（刷新 / 重进）：免审批调用可能已获自动批准但尚未回传，这里补一次自动提交。
    // 挂起定位来自快照里的 pendingCalls / 问卷块，无需实时等待事件。
    const msg = assistantMessageFor();
    if (msg) {
      activeInterrupt = interruptOfMessage(msg);
      submitDecisions(msg);
    }
    return;
  }
  // 运行中：复用当前承载消息（同页重开时仍在流式 / 待审批），否则末条尚未收尾的助手消息，
  // 再否则新建占位气泡——不把本轮增量并进已完成的历史助手消息。
  let target =
    activeAssistantKey !== null
      ? messages.value.find((m) => m.key === activeAssistantKey)
      : undefined;
  if (!target) {
    const last = messages.value[messages.value.length - 1];
    if (last?.role === "assistant" && (last.streaming === true || last.awaitingApproval === true)) {
      target = last;
    } else {
      target = { key: nextKey(), role: "assistant", content: "", streaming: true };
      messages.value.push(target);
    }
    activeAssistantKey = target.key;
  }
  target.streaming = true;
  // render 已回放当前缓冲（含实时块）；没有缓冲时以空块起步，避免把上一轮历史块并进新气泡。
  if (agentRun.stateOf(id).blocks.length === 0) {
    agentRun.adoptAnchorBlocks(id, []);
  }
  bindAssistantBlocks(id, target);
  consumeAssistantStream(
    id,
    (handlers) => assistantEventsStream(id, agentRun.lastSeqOf(id), handlers),
    { reloadOnEmpty: true },
  );
};

// ---- 生命周期 ----
// 抽屉全局常驻但按需初始化：首次展开时才请求枚举 / 状态 / 会话，
// 避免在未登录（如登录页）时就发出鉴权请求；ensureInitialized 以单例 Promise 复用，
// 使「展开抽屉」与「按指定会话打开」两条入口共用同一次初始化，避免会话列表清空选中态。
let initPromise: Promise<void> | undefined;
const ensureInitialized = (): Promise<void> => (initPromise ??= initialize());
const initialize = async (): Promise<void> => {
  void ensureEnum(EnumClass.llmSlot);
  // 工具调用阶段名与档位标签经统一枚举注册表渲染
  void ensureEnum(EnumClass.evalStage);
  // 审核阀门下拉与审批面板重要级徽章经统一枚举注册表渲染
  void ensureEnum(EnumClass.reviewValve);
  void ensureEnum(EnumClass.auditLevel);
  // 会话交互主状态与子状态：输入区状态条 / 锁定提示的 label 经统一枚举注册表渲染
  void ensureEnum(EnumClass.agentInteractionState);
  void ensureEnum(EnumClass.agentInteractionSubstate);
  await Promise.all([loadStatus(), loadSessions()]);
};

// 当前会话跨刷新持久化：刷新后据该记录恢复会话并自动续播（见 initialize）
watch(activeSessionID, (value) => {
  localManager.setAgentActiveSession(value);
});

watch(opened, async (value) => {
  if (value) {
    // 首开需等初始化（状态 + 会话列表）就绪后再载入历史与续播
    await ensureInitialized();
    // 重开抽屉：以 render 重建历史与当前缓冲快照，再按在途运行快照恢复交互状态并续接事件流
    // （宽限期内重连即停表续跑）。完成后（busy 已如实反映在途运行）再投递队首待发提问——
    // 关抽屉期间完成的运行不会再触发流回调，仅在此补投递。
    const id = activeSessionID.value;
    if (id !== null) {
      const target = sessions.value.find((s) => s.id === id);
      if (target?.slot) {
        slot.value = target.slot;
      }
      if (target?.reviewValve) {
        reviewValve.value = target.reviewValve;
      }
      await loadHistory(id);
      await syncSessionState(id);
      // 重开抽屉即进入会话：显式贴底一次，不依赖历史回放流的完成时机（抽屉过渡期间流可能已收尾）
      scrollToBottom();
      ensureQueueLoaded(id);
      drainQueuedQuery(id);
    }
    return;
  }
  // 关闭抽屉：仅退订（不取消服务端运行）。退订本身即触发服务端退订宽限计时——宽限内重开即
  // 停表续跑，宽限内始终无订阅者才由服务端自动取消；会话与消息由组件自身持有，重开时仍可见。
  if (activeSessionID.value !== null) {
    agentRun.detach(activeSessionID.value);
  }
});

onMounted(() => {
  window.addEventListener("resize", onViewportResize);
});

// 组件卸载（登出等）时移除监听并退订在途流；订阅由 agentRun store 持有，此处仅断连接、
// 不取消服务端运行，让宽限窗口吸收瞬时卸载 / 重挂载
onUnmounted(() => {
  window.removeEventListener("resize", onViewportResize);
  if (activeSessionID.value !== null) {
    agentRun.detach(activeSessionID.value);
  }
});
</script>

<style scoped>
/* 布局定位层：颜色 / 字号 / 圆角 / 边框 / 阴影等视觉一律由外壳层
   （styles/main.scss 的 .zk-chat-*）与 Vuetify 工具类承载，本块只保留
   flex / grid、尺寸约束、溢出与定位。 */

/* 抽屉内容容器撑满可用高度，供消息区独立滚动；同时作为左缘拖拽手柄的定位基准 */
.assistant-drawer {
  position: relative;
  min-height: 0;
}

/* 拖拽期间关闭抽屉自身的 width 过渡：Vuetify 默认给 width 0.2s 过渡，每帧写入的宽度
   都被动画追赶，表现为拖动不跟手、松手后还要再滑一段才落位 */
.assistant-drawer--resizing {
  transition: none;
}

/* 左缘拖拽手柄按 placement="outside" 贴到抽屉外侧，其 8px 命中条会越出内容区左缘；
   而内容区默认 overflow-x: hidden（Vuetify 的 $navigation-drawer-content-overflow-x），
   会把整条命中条裁掉，故此处放开裁剪。本抽屉内容由 .assistant-drawer 的 h-100 精确撑满，
   内容区自身不产生纵向溢出（真正的滚动容器是 .assistant-messages），故放开 overflow-y
   不改变滚动行为——顺带说明：CSS 不允许 overflow-x: visible 与 overflow-y: auto 共存，
   这里的 overflow: visible 是唯一写法 */
/* stylelint-disable selector-class-pattern -- Vuetify 内部 BEM 元素名 */
:deep(.v-navigation-drawer__content) {
  overflow: visible;
}
/* stylelint-enable selector-class-pattern */

/* 消息区容器：只承载滚动正文（提问轨绝对定位在抽屉左缘，不占这里的流）。
   min-height: 0 必需——纵向 flex 子项默认 min-height: auto 会被内容高度撑破，
   滚动交给 .assistant-messages 自身承担 */
.assistant-conversation {
  position: relative;
  min-height: 0;
}

/* 平滑滚动供提问轨的跳转使用（见 scrollToTurn） */
.assistant-messages {
  scroll-behavior: smooth;
}

/* 块间跳读控件：漂浮在消息区右缘、纵向居中，不参与正文布局（不压缩正文横向空间）。
   与右缘留出 --zk-space-4：滚动条常驻在该侧，内缩后既不遮滚动条，也不贴边显得局促 */
.assistant-block-nav-slot {
  position: absolute;
  inset-inline-end: var(--zk-space-4);
  top: 50%;
  z-index: 1;
  transform: translateY(-50%);
}

/* 用户消息气泡：宽度上限保留 85% 比例；圆角由 .zk-chat-bubble* 外壳类承载 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与 .zk-chat-bubble--user 同风格 */
.assistant-bubble--user {
  max-width: 85%;
}

/* 助手回答不包气泡：抽屉宽度本就有上限，而回答往往很长，气泡会把正文压成窄长条；
   改为占满消息区宽度，与下方常驻操作行一起读作一整段回答 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与 .zk-chat-bubble--assistant 同风格 */
.assistant-bubble--assistant {
  width: 100%;
}

/* 单轮消息：用户轮靠右，助手轮靠左 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.assistant-turn--user {
  align-items: flex-end;
}

/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.assistant-turn--assistant {
  align-items: flex-start;
}

/* 操作图标簇：三个图标紧密成组，与左侧时间保持一档以上的间隔 */
.assistant-message-buttons {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
}

/* 用户消息正文保留换行与空格（纯文本） */
.assistant-plain {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.assistant-stage {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
}

/* 一轮工具调用的展示组：组内各调用行自上而下排列；相邻组之间留一丝间距以区分轮次 */
.assistant-stage-group {
  display: flex;
  flex-direction: column;
}

.assistant-stage-group + .assistant-stage-group {
  margin-top: var(--zk-space-1);
}

/* 思考过程折叠区块：外层描边盒由 .zk-panel 提供，此处只裁掉盒内面板溢出四角 */
.assistant-reasoning {
  overflow: hidden;
}

/* 思考正文（折叠块展开区）：限高滚动，长思考不撑高消息区。
   高度上限取思考区专属令牌 --zk-reasoning-max-height；约束落在正文自己的容器上
   （而非面板正文根元素），滚动条只随正文出现，面板内边距不参与滚动。
   流式增长时的粘底由 scrollReasoningToBottom 负责（见 onReasoning） */
.assistant-reasoning-body {
  max-height: var(--zk-reasoning-max-height);
  overflow-y: auto;
}

/* 压缩摘要三行预览：仅进行中展示；完成后完全收起，由标题行展开查看全文 */
.assistant-compress-draft {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符（双短横线） */
.assistant-compress-draft--expanded {
  display: block;
  max-height: var(--zk-inline-preview-max-height);
  overflow-y: auto;
}

/* 压缩标题行在可展开时表现为可点击单元 */
.assistant-compress-toggle {
  cursor: pointer;
}

/* 引用正文两行截断，避免单条引用撑爆气泡 */
.assistant-citation-text {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 思考开关：按内容取宽、不参与收缩。
   只压外框高度：滑块容器（--v-selection-control-size，compact 档 28px）尺寸不动、
   在其中居中，开关自身的滑块与轨道位置不变，与同排按钮等高后整行由下拉定高 */
.assistant-composer-thinking {
  flex: 0 0 auto;
}

/* stylelint-disable selector-class-pattern -- Vuetify 内部 BEM 元素名 */
.assistant-composer-thinking :deep(.v-selection-control) {
  min-height: 0;
  height: 20px;
}
/* stylelint-enable selector-class-pattern */

/* 推荐下一步：整行可点击直接发送；左侧主按钮占满剩余宽度 */
.assistant-next-step-activator {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
}

.zk-chat-next-step-main {
  flex: 1 1 auto;
  min-width: 0;
}

.assistant-next-step-text {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-1);
  flex: 1 1 auto;
  min-width: 0;
  overflow-wrap: anywhere;
}

/* 队列正文单行截断：超长追问不撑高堆叠区，完整内容可经「修改」回填到输入框查看 */
.zk-chat-queue-text {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

/* 技能下拉：限高滚动，避免技能较多时撑高输入区 */
.zk-chat-skill-menu {
  max-height: 240px;
  overflow-y: auto;
  margin: 8px;
}

/* 会话列表面板：行内 hover / 选中底色铺满盒宽，裁掉四角以贴合面板圆角 */
.session-panel {
  overflow: hidden;
}
</style>

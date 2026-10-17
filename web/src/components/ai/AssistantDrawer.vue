<template>
  <v-navigation-drawer v-model="opened" location="right" temporary :width="drawerWidth">
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
            class="session-row zk-row-hover"
            :class="{
              'zk-row-selected': activeSessionID === s.id,
              'zk-hairline-top': idx > 0,
            }"
            @click="pickSession(s.id)"
          >
            <div class="session-main">
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
           少于两个提问时轨道不渲染，左内边距仍保留（值不足以按提问数变化） -->
      <div v-if="!sessionListMode" class="assistant-conversation d-flex flex-grow-1 min-h-0">
        <assistant-question-rail
          v-if="userQuestions.length >= 2"
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
              class="assistant-turn d-flex flex-column mb-4"
              :class="m.role === 'user' ? 'assistant-turn--user' : 'assistant-turn--assistant'"
            >
              <!-- 仅用户消息用气泡框包裹；助手回答内容多，气泡会把正文压成窄长条，
                 故改为直接铺满消息区宽度（见 .assistant-bubble--assistant） -->
              <div
                class="assistant-bubble"
                :class="
                  m.role === 'user'
                    ? 'zk-chat-bubble bg-primary zk-chat-bubble--user assistant-bubble--user'
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
                <!-- 消息体内按时序排布的内容块（见 assistantBlocksOf）：工具调用阶段按轮聚组——该轮
                   思考折叠块渲染一次、置于组上方，组内各调用行自上而下成行；一轮内并发多次调用时以
                   并行计数与左侧细线框成一组，体现「一次思考 → N 次并行调用」。人工审批与主动提问两块
                   按其触发轮次内联插入，使整条消息自上而下严格按时序阅读——作答 / 批准之后新增的思考
                   与调用必然落在交互块之后，不再出现「后续过程反在提问之上」的错位 -->
                <div v-if="assistantBlocksOf(m).length > 0" class="assistant-stages">
                  <template v-for="b in assistantBlocksOf(m)" :key="b.key">
                    <!-- 该轮思考折叠块：位于本组调用行上方（历史消息无分段时整条思考落到正文上方那块） -->
                    <div v-if="b.kind === 'reasoning'" class="zk-panel assistant-reasoning">
                      <v-expansion-panels>
                        <v-expansion-panel :value="REASONING_PANEL">
                          <expansion-panel-title
                            icon="mdi-brain"
                            size="sm"
                            :title="$t('page.aiAssistant.reasoningDone')"
                          />
                          <v-expansion-panel-text>
                            <div class="assistant-reasoning-body">
                              <assistant-markdown :content="b.text ?? ''" />
                            </div>
                          </v-expansion-panel-text>
                        </v-expansion-panel>
                      </v-expansion-panels>
                    </div>
                    <div
                      v-else-if="b.kind === 'stages'"
                      class="assistant-stage-group"
                      :class="{ 'assistant-stage-group--parallel': (b.parallelCount ?? 0) > 1 }"
                    >
                      <div
                        v-if="(b.parallelCount ?? 0) > 1"
                        class="zk-hint assistant-stage-parallel-label"
                      >
                        {{ $t("page.aiAssistant.parallelCalls", { count: b.parallelCount ?? 0 }) }}
                      </div>
                      <div v-for="s in b.stages" :key="s.key" class="assistant-stage">
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
                    <!-- 主动提问：待回答时渲染交互问卷面板（会话被阻塞，由输入区禁用体现），
                         已作答（含取消）时渲染问答记录卡片并提供「重新回答」回退入口。
                         面板自身保持完全可交互——不传会话级 busy：问卷挂起时 awaitingApproval 为
                         true、busy 亦为 true，若以 busy 闸住面板会把「下一题 / 提交 / 取消」一并锁死 -->
                    <div
                      v-else-if="b.kind === 'questionnaire' && m.questionnaire"
                      class="assistant-interaction"
                    >
                      <ask-user-questionnaire
                        v-if="m.questionnaire.awaiting"
                        :payload="questionnairePayloadOf(m)"
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
                          <!-- 重新回答：会话正忙（续跑仍在生成 / 等待审批等）时禁用——此刻重答会与在途
                               运行冲突（截断并重生成必须先中止它），仅以提示说明原因；空闲时可用。
                               禁用按钮自身不派发指针事件，故套一层 span 承载气泡激活器 -->
                          <v-tooltip
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
                                  @click="openReanswerConfirm(m)"
                                >
                                  {{ $t("page.aiAssistant.questionnaireReanswer") }}
                                </v-btn>
                              </span>
                            </template>
                          </v-tooltip>
                        </div>
                        <div
                          v-for="(q, idx) in m.questionnaire.questions"
                          :key="q.id || idx"
                          class="zk-panel--subtle pa-2 mb-2"
                        >
                          <div class="text-body-medium">{{ q.question }}</div>
                          <div class="zk-hint mt-1">
                            {{ questionnaireAnswerTextOf(m.questionnaire, q.id) }}
                          </div>
                        </div>
                      </div>
                    </div>
                    <!-- 人工审批：模型请求调用平台原生接口，逐条展示并需用户批准执行 / 拒绝。
                         ask_user 问卷类调用豁免审批，经 approvalCallsOf 过滤后不出现在此面板 -->
                    <div v-else-if="b.kind === 'approval'" class="assistant-interaction">
                      <div class="zk-hairline-top pt-2">
                        <div class="d-flex align-center mb-1">
                          <v-icon icon="mdi-shield-key-outline" size="small" class="me-1" />
                          <span class="zk-label-caps">{{
                            $t("page.aiAssistant.approvalTitle")
                          }}</span>
                          <v-spacer />
                          <span v-if="m.awaitingApproval" class="zk-hint">
                            {{ $t("page.aiAssistant.approvalWaiting") }}
                          </span>
                        </div>
                        <div
                          v-for="c in approvalCallsOf(m)"
                          :key="c.callID"
                          class="zk-panel--subtle pa-2 mb-2"
                        >
                          <div class="d-flex align-center">
                            <span class="zk-mono-emphasis">{{ c.toolName }}</span>
                            <!-- 审核重要级徽章（label 经枚举类 auditLevel 按当前语言渲染） -->
                            <v-chip
                              v-if="c.auditLevel"
                              size="x-small"
                              color="info"
                              variant="tonal"
                              class="ms-2"
                            >
                              {{ enumLabelOf(EnumClass.auditLevel, c.auditLevel) }}
                            </v-chip>
                            <v-spacer />
                            <!-- 免审批调用：自动批准标识，不展示批准 / 拒绝按钮 -->
                            <v-chip
                              v-if="c.approvalRequired === false"
                              size="x-small"
                              color="info"
                              class="me-2"
                            >
                              {{ $t("page.aiAssistant.autoApproved") }}
                            </v-chip>
                            <v-chip v-if="c.decision === 'approved'" size="x-small" color="success">
                              {{ $t("page.aiAssistant.approved") }}
                            </v-chip>
                            <v-chip
                              v-else-if="c.decision === 'rejected'"
                              size="x-small"
                              color="error"
                            >
                              {{ $t("page.aiAssistant.rejected") }}
                            </v-chip>
                          </div>
                          <!-- 后端执行的调用：前端仅审批，工具由服务端执行，用户无需（也无法）本地执行 -->
                          <div v-if="c.execOwner === 'backend'" class="zk-hint mt-1">
                            {{ $t("page.aiAssistant.approvalBackendNote") }}
                          </div>
                          <template v-if="c.arguments">
                            <div class="zk-hint mt-1">
                              {{ $t("page.aiAssistant.approvalParams") }}
                            </div>
                            <div class="zk-code-preview font-mono assistant-approval-block">
                              {{ c.arguments }}
                            </div>
                          </template>
                          <div v-if="c.error" class="zk-error-text mt-1">{{ c.error }}</div>
                          <div
                            v-else-if="c.result"
                            class="zk-code-preview font-mono assistant-approval-block"
                          >
                            {{ c.result }}
                          </div>
                          <div v-if="!c.decision" class="d-flex mt-2">
                            <v-btn
                              size="small"
                              variant="tonal"
                              color="primary"
                              @click="approveCall(m, c)"
                            >
                              {{ $t("page.aiAssistant.approve") }}
                            </v-btn>
                            <v-btn
                              size="small"
                              variant="text"
                              class="ms-2"
                              @click="rejectCall(m, c)"
                            >
                              {{ $t("page.aiAssistant.reject") }}
                            </v-btn>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
                <!-- 思考过程折叠区块（末段，即产出回答前那一轮）：展示模型的思考过程，复用统一 Markdown
                   渲染入口；流式期间默认展开，结束后自动折叠，用户可手动切换。此前各轮的思考块已渲染在
                   对应调用行上方（见上）。
                   标题不另设文案：状态（思考中… / 思考完成）本身已说明该区块的主体内容，
                   再叠一句「思考过程」属重复信息，故直接以状态作标题、不再挂说明行 -->
                <div
                  v-if="m.role === 'assistant' && topReasoningText(m)"
                  class="zk-panel assistant-reasoning"
                >
                  <v-expansion-panels
                    :model-value="m.reasoningExpanded ? REASONING_PANEL : ''"
                    @update:model-value="(v: unknown) => setReasoningExpanded(m, v)"
                  >
                    <v-expansion-panel :value="REASONING_PANEL">
                      <expansion-panel-title
                        icon="mdi-brain"
                        size="sm"
                        :title="
                          m.reasoningStreaming
                            ? $t('page.aiAssistant.reasoningStreaming')
                            : $t('page.aiAssistant.reasoningDone')
                        "
                      >
                        <!-- 思考来源说明：当前档位模型未配置思考开关（开关已隐藏），却仍产出思考内容，
                           说明是该模型默认开启思考。以警示图标 + 悬浮提示解释来源，并指引管理员在
                           「LLM 模型」高级配置中填写思考开关字段与开 / 关取值，从而手动控制思考 -->
                        <v-tooltip
                          v-if="!slotThinkingToggle"
                          location="top"
                          :max-width="REASONING_NOTE_MAX_WIDTH"
                          :text="$t('page.aiAssistant.reasoningDefaultOnHint')"
                        >
                          <template #activator="{ props: reasonNoteProps }">
                            <v-icon
                              v-bind="reasonNoteProps"
                              icon="mdi-alert-circle-outline"
                              size="x-small"
                              color="warning"
                            />
                          </template>
                        </v-tooltip>
                      </expansion-panel-title>
                      <v-expansion-panel-text>
                        <!-- 只有本块（正文上方那一段）会在流式期间增长：登记其滚动区，
                             增量到达后把最新思考粘到底部（上方各轮思考块已锚定、不再增长） -->
                        <div
                          :ref="(el) => setReasoningBodyRef(m.key, el)"
                          class="assistant-reasoning-body"
                        >
                          <assistant-markdown
                            :content="topReasoningText(m)"
                            :streaming="m.reasoningStreaming === true"
                          />
                        </div>
                      </v-expansion-panel-text>
                    </v-expansion-panel>
                  </v-expansion-panels>
                </div>
                <!-- 助手消息渲染 Markdown 富文本；用户消息按纯文本展示 -->
                <assistant-markdown
                  v-if="m.role === 'assistant' && m.content"
                  :content="m.content"
                  :streaming="m.streaming === true"
                />
                <div v-else-if="m.content" class="assistant-plain text-body-medium">
                  {{ m.content }}
                </div>
                <!-- 运行未正常结束提示：助手消息 phase 为 failed / cancelled 时展示，按阶段取文案
                   （由前端 i18n 提供，不落库、不混入正文）；以警示图标 + 弱化底与正文区分 -->
                <div
                  v-if="interruptedTextOf(m)"
                  class="assistant-interrupted zk-panel--subtle mt-2"
                >
                  <v-icon icon="mdi-alert-circle-outline" size="small" color="warning" />
                  <span class="zk-hint ms-2">{{ interruptedTextOf(m) }}</span>
                </div>
                <template v-if="citationsOf(m).length > 0">
                  <v-divider class="my-2" />
                  <div class="zk-label-caps mb-1">{{ $t("page.aiAssistant.citations") }}</div>
                  <v-list class="bg-transparent" density="compact">
                    <v-list-item v-for="(c, idx) in citationsOf(m)" :key="idx">
                      <v-list-item-title class="text-body-medium">
                        <!-- 来源命名空间可溯源（含历史消息缺失该字段的降级）时标题渲染为链接，
                           直接跳转文档详情页；缺失则保持纯文本，避免产生无效跳转 -->
                        <a
                          v-if="c.namespaceName"
                          class="assistant-citation-link"
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
                </template>
              </div>
              <!-- 助手回答快捷操作：回答完（非生成中）后常驻展示——助手内容多且已去掉气泡，
                 操作不必再靠悬停去发现；复制整段回答 / 重试本轮 / 以本轮为结尾分支出新会话 -->
              <div
                v-if="m.role === 'assistant' && !m.streaming"
                class="assistant-message-actions assistant-message-actions--persistent"
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
                    :disabled="busy || m.compress?.phase === 'running'"
                    :title="$t('page.aiAssistant.compressToHere')"
                    :aria-label="$t('page.aiAssistant.compressToHere')"
                    @click="compactToHere(m)"
                  />
                </div>
                <span v-if="usageTextOf(m)" class="zk-hint">{{ usageTextOf(m) }}</span>
              </div>
              <!-- 上下文压缩展示块：自动压缩挂本轮助手消息、手动压缩挂被点击的助手消息，
                 仅当该消息带压缩态时展示。属运行元信息（手动压缩语义即「压缩到此」），
                 故置于该轮快捷操作行下方、作为整轮最后一条提示——既不与正文 / 思考块抢位置，
                 也让操作行紧贴它所操作的正文 -->
              <div
                v-if="m.compress"
                class="assistant-compress zk-panel--subtle mt-2"
                :class="{ 'assistant-compress--failed': m.compress.phase === 'failed' }"
              >
                <div class="d-flex align-center">
                  <v-icon
                    :icon="compressIconOf(m.compress)"
                    :color="compressColorOf(m.compress)"
                    size="small"
                    :class="{ 'mdi-spin': m.compress.phase === 'running' }"
                  />
                  <span class="zk-hint ms-2">{{ compressTextOf(m) }}</span>
                </div>
                <!-- 摘要增量：压缩进行中实时流式呈现，等宽 / 引用样式；为空时不占位 -->
                <div
                  v-if="m.compress.phase === 'running' && m.compress.draft"
                  class="zk-code-preview font-mono assistant-compress-draft"
                >
                  {{ m.compress.draft }}
                </div>
              </div>
              <!-- 用户消息快捷操作：悬停该轮时淡入（左侧创建时间 + 复制 / 回退 / 删除）；生成中禁用，
                 避免截断在途运行正在使用的消息 -->
              <div v-if="m.role === 'user'" class="assistant-message-actions">
                <span class="zk-hint">{{ messageTimeOf(m) }}</span>
                <div class="assistant-message-buttons">
                  <v-btn
                    icon="mdi-content-copy"
                    size="small"
                    variant="text"
                    :disabled="busy"
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
      </div>

      <!-- 输入区：与消息区之间不设分隔线，让输入框直接落在对话内部，读作同一整体 -->
      <div v-if="!sessionListMode" class="pa-3">
        <v-alert v-if="aiStatus && !aiStatus.enabled" type="warning" density="compact" class="mb-3">
          <div>{{ aiStatus.reason || $t("page.aiAssistant.disabled") }}</div>
          <v-btn variant="text" size="small" class="mt-1" @click="gotoLlmConfig">
            {{ $t("page.featureUnavailable.activate", { page: $t("navbar.admin.llmConfig") }) }}
          </v-btn>
        </v-alert>
        <!-- 单一边框容器：正文在上，档位选择与发送按钮内置其底部（对齐主流对话产品）。
             底色取弱化面板（on-surface 6% + hairline），让输入区在消息流中凸显出来 -->
        <div
          class="assistant-composer zk-panel zk-panel--subtle"
          :class="{ 'assistant-composer--blocked': questionnairePending }"
        >
          <!-- 问卷待作答提示行：问卷已内联在消息流中（见 assistantBlocksOf），会话被阻塞，
               输入区整体禁用；此处置一条提示并给出「查看问题」入口，把用户视线引回消息流里的问卷 -->
          <div v-if="questionnairePending" class="assistant-composer-pending zk-hint">
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
          <div v-if="activeQueue.length > 0" class="assistant-queue">
            <div v-for="item in activeQueue" :key="item.key" class="assistant-queue-item">
              <v-icon icon="mdi-clock-outline" size="x-small" class="assistant-queue-icon" />
              <span class="assistant-queue-text">{{ item.text }}</span>
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
          <v-textarea
            v-model="query"
            variant="plain"
            density="compact"
            rows="2"
            auto-grow
            hide-details
            :disabled="questionnairePending"
            :title="$t('model.aiSession.field.query')"
            @keydown.enter.exact="onComposerEnter"
            @compositionstart="composerComposing = true"
            @compositionend="composerComposing = false"
          />
          <div class="assistant-composer-toolbar">
            <v-select
              v-model="slot"
              :items="slotOptions"
              variant="plain"
              density="compact"
              hide-details
              :disabled="questionnairePending"
              :title="slotFullTitle"
              class="assistant-composer-slot zk-control-hover"
            />
            <!-- 审核阀门（会话级）：切换即持久化，草稿态随创建请求写入。
                 关闭态以图标标识当前阀门，完整名称与说明由展开项与悬浮提示承载 -->
            <v-select
              :model-value="reviewValve"
              :items="reviewValveOptions"
              variant="plain"
              density="compact"
              hide-details
              :disabled="questionnairePending"
              :title="reviewValveTooltip"
              class="assistant-composer-valve zk-control-hover"
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
                <div v-bind="activatorProps" class="assistant-composer-usage">
                  <v-progress-circular
                    :model-value="contextUsagePercent"
                    :size="16"
                    :width="2"
                    :color="contextUsageColor"
                  />
                  <span
                    class="assistant-composer-usage-percent"
                    :class="`text-${contextUsageColor}`"
                  >
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
                <div v-bind="thinkingProps" class="assistant-composer-thinking">
                  <v-switch
                    v-model="thinking"
                    density="compact"
                    color="primary"
                    hide-details
                    :disabled="questionnairePending"
                    :aria-label="$t('page.aiAssistant.thinking')"
                  />
                </div>
              </template>
            </v-tooltip>
            <!-- 右侧动作组：优化提问与发送 / 停止成组贴右，与左侧档位 / 阀门 / 占用指示 / 思考开关分区 -->
            <div class="assistant-composer-actions">
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
                v-if="streaming"
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

    <!-- 重新回答确认对话框：提醒确认后本问之后的全部内容将被清空并重新生成 -->
    <confirm-dialog
      ref="reanswerConfirmRef"
      :title="$t('page.aiAssistant.questionnaireReanswer')"
      prepend-icon="mdi-refresh"
      @confirm="confirmReanswer"
    >
      <template #body>
        <confirm-alert
          :action="$t('page.aiAssistant.questionnaireReanswer')"
          :text="$t('page.aiAssistant.confirmReanswerQuestionnaire')"
        />
      </template>
    </confirm-dialog>
  </v-navigation-drawer>
</template>

<script setup lang="ts" name="assistant-drawer">
import { DateTime } from "luxon";
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  agentMessageListGet,
  agentMessageTruncate,
  agentPromptOptimize,
  agentQuestionnaireReanswer,
  agentRunCancel,
  agentSessionBranch,
  agentSessionCompactStream,
  agentSessionCreate,
  agentSessionDelete,
  agentSessionListGet,
  agentSessionRename,
  agentSessionValveSet,
  agentStatusGet,
  agentStreamGenerate,
  agentStreamResume,
  agentToolResultStream,
  type AgentStreamHandlers,
} from "@/apis/ai";
import type {
  AgentLiveRunOut,
  AgentMessageItem,
  AgentMessageQuestionnaire,
  AgentPromptOptimizeOut,
  AgentSessionItem,
  AgentStageEvent,
  AgentStatusOut,
  CompressDeltaPayload,
  CompressDonePayload,
  CompressStartPayload,
  EvalStage,
  EvalStageStatus,
  QuestionnaireAnswer,
  QuestionnairePayload,
  QuestionnaireQuestion,
  RetrievalResult,
  ToolRequestPayload,
  ToolResultItemIn,
} from "@/apis/@types/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import { useAgentRunStore } from "@/stores/agentRun";
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

// opened 双向绑定：向上透传抽屉开关状态
const opened = computed<boolean>({
  get: () => props.modelValue,
  set: (value) => emits("update:modelValue", value),
});

// ---- 抽屉宽度 ----
// 默认随视口收放，但保持在可读区间内（窄屏不小于 320px，宽屏不超过 560px）；
// 用户可在左缘拖拽手动设定宽度，手动值优先于视口比例值，并在视口收窄时同步收敛
const DRAWER_WIDTH_RATIO = 0.42;
const DRAWER_WIDTH_MIN = 320;
const DRAWER_WIDTH_MAX = 560;

const viewportWidth = ref<number>(window.innerWidth);

// maxDrawerWidth 当前视口下允许的最大宽度：不超过设计上限，也不超出视口本身
const maxDrawerWidth = computed<number>(() =>
  Math.min(DRAWER_WIDTH_MAX, Math.max(DRAWER_WIDTH_MIN, viewportWidth.value)),
);

// autoDrawerWidth 未手动设定时的宽度：按视口比例取宽并收敛到可读区间
const autoDrawerWidth = computed<number>(() =>
  Math.min(
    DRAWER_WIDTH_MAX,
    Math.max(DRAWER_WIDTH_MIN, Math.round(viewportWidth.value * DRAWER_WIDTH_RATIO)),
  ),
);

// manualWidth 用户拖拽设定的宽度；为 null 时回退视口比例值
const manualWidth = ref<number | null>(null);

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

// AgentStageState 工具调用阶段视图模型：一次工具调用一行，同一次调用的「发起（toolCall）」与
// 「结果（toolResult）」按 callID 递进收敛到同一行；无 callID 时回退按「阶段 + 工具名」归并
interface AgentStageState {
  key: string;
  stage: EvalStage;
  tool: string;
  status?: EvalStageStatus;
  elapsedMs: number;
  error?: string;
  // callID 本次工具调用的唯一标识（命中该值的事件归并到本行）；缺失时为空，按旧键回退归并
  callID?: string;
  // round 本次调用所属轮次（从 1 起）：同一轮内并发发起的多次调用据此聚为一组；缺失时该行独成一组
  round?: number;
}

// AgentStageGroup 一轮工具调用的展示分组：同轮（round 相同且相邻）的调用行归为一组，组上方渲染
// 该轮思考折叠块一次，组内多行以并行计数与左侧细线呈现「一次思考 → N 次并行调用」的从属关系
interface AgentStageGroup {
  key: string;
  round?: number;
  stages: AgentStageState[];
  // reasoningText 该轮思考文本：取锚定到本组任一次调用的思考分段（同轮思考只归属首个调用行）
  reasoningText: string;
}

// ReasoningSegment 一段模型思考：live 期间按「轮」切分。anchorCallID 非空表示该段紧随其后的那次
// 工具调用（渲染在该调用行上方）；为空表示最后一轮（产出回答前），维持在回答正文上方。
// 与阶段行同为 live-only 视图态，不落库；刷新后回退为整条 reasoning 单块展示
interface ReasoningSegment {
  key: string;
  text: string;
  streaming: boolean;
  anchorCallID?: string;
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
  submitted?: boolean;
}

// CompressState 上下文压缩的展示态（自动挂在当前助手消息、手动挂在被点击的助手消息、静默滚动
// 与其余推进一样在刷新后随历史回显挂在被折叠的最后一条消息）：
//   phase       压缩阶段（running 进行中 / done 完成 / failed 失败）
//   draft       compress_delta 累积的摘要增量（流式呈现）
//   reason      推进原因（auto 自动 / manual 手动 / rolling 静默滚动）
//   tokensBefore 推进前占用 token（实时的取自 compress_start，历史的取自压缩检查点）
//   foldedCount  本次折叠的消息条数（实时的取自 compress_done，历史的取自压缩检查点）
//   tokensAfter  压缩后占用 token（下一次生成 done.promptTokens 回填，供前后对比；历史回显无此项）
interface CompressState {
  phase: "running" | "done" | "failed";
  draft: string;
  reason: "auto" | "manual" | "rolling";
  tokensBefore?: number;
  foldedCount?: number;
  tokensAfter?: number;
}

// QuestionnaireState 主动提问问卷的视图态：questions 为题目、answers 为用户作答（未提交时缺省）、
// awaiting 表示是否仍待回答。待回答时由输入区问卷面板承载（遮蔽输入框、阻塞会话），已作答（含取消）
// 后改由消息下方的问卷卡片展示并提供「重新回答」入口。
interface QuestionnaireState {
  questions: QuestionnaireQuestion[];
  answers?: QuestionnaireAnswer[];
  awaiting: boolean;
}

// ChatMessage 会话消息视图模型：会话历史消息与流式占位消息统一形态。
// citations 由 done 事件回填；stages 为本次生成实时累积的工具调用阶段（历史消息不携带，
// 后端不持久化阶段）；pendingCalls/awaitingApproval 承载「前端执行 + 人工审批」链路的状态；
// compress 承载本轮（自动）或手动触发的上下文压缩展示态
interface ChatMessage {
  key: number;
  // id 服务端消息主键：历史消息自列表接口带回；仅存于内存的即时气泡（尚未回读服务端）无此项。
  // 续播中断运行时就地复用该行重建本轮内容，避免与已落库的中间产物重影
  id?: number;
  role: "user" | "assistant" | "system";
  content: string;
  // createdAt 消息创建时间（ISO 文本）：历史消息取自后端，即时发出、尚未回读服务端的消息取本地时间
  createdAt?: string;
  citations?: RetrievalResult[];
  streaming?: boolean;
  // reasoning 助手思考过程全文：流式期间为累积增量，done 到达后收敛为全文（历史消息取自列表接口）
  reasoning?: string;
  // reasoningSegments 思考过程按「轮」的分段（live-only）：每段锚定到其后的那次工具调用并渲染在
  // 该调用行上方，末段（产出回答前）维持在正文上方；历史消息不携带，回退整条 reasoning 单块展示
  reasoningSegments?: ReasoningSegment[];
  // reasoningStreaming 思考过程是否仍在流式生成（驱动折叠区块的展开态与状态文案）
  reasoningStreaming?: boolean;
  // reasoningExpanded 思考过程区块当前是否展开：流式期间默认展开、结束自动折叠，用户可手动切换
  reasoningExpanded?: boolean;
  stages?: AgentStageState[];
  // pendingCalls 本次挂起、待用户审批并执行的接口调用（为空表示无待审批项）
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
  // questionnaire 本条助手消息承载的「主动提问」问卷视图态（ask_user 调用的锚点行）：待回答时由
  // 输入区问卷面板承载并阻塞会话，用户提交 / 取消后改为消息下方卡片展示；其待执行调用随
  // pendingCalls 一并登记（callID 为提交续跑所必需）
  questionnaire?: QuestionnaireState;
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
// contextUsedTokens 当前会话占用的 token：优先取最新一条已带用的助手消息的 promptTokens
// （done 到达即回填），无则回落到会话列表项记录的 contextTokens；草稿态无会话为 0
const contextUsedTokens = computed<number>(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const m = messages.value[i];
    if (m.role === "assistant" && m.promptTokens) {
      return m.promptTokens;
    }
  }
  const id = activeSessionID.value;
  if (id !== null) {
    const session = sessions.value.find((s) => s.id === id);
    if (session) {
      return session.contextTokens ?? 0;
    }
  }
  return 0;
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

// applyAgentStage 应用一次工具调用阶段事件：优先按 callID 归并到该次调用所在行，使同一次调用的
// 「发起（toolCall）」与「结果（toolResult）」递进收敛到同一行；无 callID 时回退按「阶段 + 工具名」
// 归并（如达到轮数上限的整体失败事件）。命中同一行时同时更新阶段标签、状态、耗时与失败原因
const applyAgentStage = (key: number, ev: AgentStageEvent): void => {
  const msg = messages.value.find((m) => m.key === key);
  if (!msg) {
    return;
  }
  const stages = (msg.stages ??= []);
  const stageKey = ev.callID ?? `${ev.stage}:${ev.tool ?? ""}`;
  const target = stages.find((s) => s.key === stageKey);
  if (target) {
    target.stage = ev.stage;
    target.status = ev.status ?? target.status;
    if (ev.elapsedMs > 0) {
      target.elapsedMs = ev.elapsedMs;
    }
    if (ev.error) {
      target.error = ev.error;
    }
  } else {
    stages.push({
      key: stageKey,
      stage: ev.stage,
      tool: ev.tool ?? "",
      status: ev.status,
      elapsedMs: ev.elapsedMs,
      error: ev.error,
      callID: ev.callID,
      round: ev.round,
    });
  }
  scrollToBottom();
};

// stageGroupsOf 把给定阶段行按轮聚组：round 相同且相邻的行归为一组（后端每轮从 1 递增下发 round），
// 使「一次思考 → N 次并行调用」的从属关系一眼可读；无 round 的行（旧数据 / 回退键）各自成组。
// 组内思考取锚定到本组任一次调用的分段文本（同轮思考只归属首个调用行，故取到即该轮思考）
const stageGroupsOf = (m: ChatMessage, stages: AgentStageState[]): AgentStageGroup[] => {
  const groups: AgentStageGroup[] = [];
  for (const s of stages) {
    const last = groups[groups.length - 1];
    if (last && last.round !== undefined && s.round === last.round) {
      last.stages.push(s);
      continue;
    }
    groups.push({ key: `g-${s.key}`, round: s.round, stages: [s], reasoningText: "" });
  }
  for (const g of groups) {
    for (const s of g.stages) {
      if (!s.callID) {
        continue;
      }
      const text = anchoredReasoningTextOf(m, s.callID);
      if (text) {
        g.reasoningText = text;
        break;
      }
    }
  }
  return groups;
};

// awaitingFirstResponse 判断该助手消息是否仍处于「请求已发出、尚无任何产出」的等待首包阶段：
// 流式已开始但正文 / 思考 / 工具阶段均未到达（含模型首包前的长等待与被中断前的空档）。
// 此阶段以「等待响应中…」替代「生成中…」，避免长等待被误读为服务异常而触发刷新或重复提交
const awaitingFirstResponse = (m: ChatMessage): boolean =>
  m.streaming === true &&
  m.content.trim() === "" &&
  !m.reasoning &&
  !(m.stages && m.stages.length > 0);

// ---- 思考过程分段 ----
// reasoningSegmentSeq 思考分段的自增键序列：分段为 live-only 视图态，键仅需在本消息内唯一
let reasoningSegmentSeq = 0;
const nextReasoningSegmentKey = (): string => `rs-${(reasoningSegmentSeq += 1)}`;

// appendReasoning 把一段思考增量累加进消息的思考分段：追加到当前未锚定的末段；若末段已锚定
// （该轮思考已归属其后的那次调用）则新开一段。同时维持整条 reasoning 累积与展开态，供上方
// 折叠区块驱动（末段即「正文上方那块」）
const appendReasoning = (m: ChatMessage, delta: string): void => {
  const segments = (m.reasoningSegments ??= []);
  let seg = segments[segments.length - 1];
  if (!seg || seg.anchorCallID) {
    seg = { key: nextReasoningSegmentKey(), text: "", streaming: true };
    segments.push(seg);
  }
  seg.text += delta;
  seg.streaming = true;
  m.reasoning = (m.reasoning ?? "") + delta;
  m.reasoningStreaming = true;
  m.reasoningExpanded = true;
};

// anchorReasoningSegment 把当前未锚定的思考分段锚定到刚发起的那次工具调用（渲染在该调用行上方）。
// 已锚定或尚无文本的分段不改动：一轮内多个调用时该轮思考只归属首个调用行
const anchorReasoningSegment = (m: ChatMessage, callID: string): void => {
  const segments = m.reasoningSegments;
  if (!segments || segments.length === 0) {
    return;
  }
  const seg = segments[segments.length - 1];
  if (seg.anchorCallID || seg.text === "") {
    return;
  }
  seg.anchorCallID = callID;
  seg.streaming = false;
};

// endReasoningStreaming 结束思考流式：闭合末段并收起折叠区块（保留已累积内容）
const endReasoningStreaming = (m: ChatMessage): void => {
  m.reasoningStreaming = false;
  m.reasoningExpanded = false;
  const segments = m.reasoningSegments;
  if (segments && segments.length > 0) {
    segments[segments.length - 1].streaming = false;
  }
};

// anchoredReasoningTextOf 返回某次调用行上方要渲染的思考文本（锚定到该次调用的分段）；
// 无分段或该调用未锚定任何分段时返回空串（不占位）
const anchoredReasoningTextOf = (m: ChatMessage, callID?: string): string => {
  if (!callID || !m.reasoningSegments) {
    return "";
  }
  return m.reasoningSegments.find((s) => s.anchorCallID === callID)?.text ?? "";
};

// topReasoningText 返回正文上方那块的思考文本：末段（无 anchorCallID，即产出回答前那一轮）。
// 历史消息无分段时回退整条 reasoning，保持刷新后行为不变
const topReasoningText = (m: ChatMessage): string => {
  if (!m.reasoningSegments) {
    return m.reasoning ?? "";
  }
  const unanchored = m.reasoningSegments.filter((s) => !s.anchorCallID);
  return unanchored[unanchored.length - 1]?.text ?? "";
};

const scrollToBottom = (): void => {
  void nextTick(() => {
    const el = messagesRef.value;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
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

// 消息区首帧挂载、消息条数变化、抽屉宽度变化都会改变各问位置，需重算当前提问
watch(messagesRef, () => void nextTick(updateActiveQuestion));
watch(
  () => messages.value.length,
  () => void nextTick(updateActiveQuestion),
);
watch(drawerWidth, () => void nextTick(updateActiveQuestion));

// ---- 会话加载 ----
const notifyError = (text: string): void => {
  appStore.notify({ text, timeout: 3000, color: "error" });
};

// notifyNoResponse 请求已发出却未取得响应（超时 / 网络异常）时的统一提示：
// getResponse 不传该回调会静默吞掉，用户看不到任何反馈
const notifyNoResponse = async (): Promise<void> => {
  notifyError(i18n.global.t("page.aiAssistant.noResponse"));
};

const loadSessions = async (): Promise<void> => {
  await getResponse<AgentSessionItem[]>(
    agentSessionListGet(),
    async (rsp) => {
      sessions.value = rsp.data ?? [];
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

const loadMessages = async (id: number): Promise<void> => {
  await getResponse<AgentMessageItem[]>(
    agentMessageListGet(id),
    async (rsp) => {
      messages.value = (rsp.data ?? []).map((m) => ({
        key: nextKey(),
        id: m.id,
        role: m.role as ChatMessage["role"],
        content: m.content ?? "",
        createdAt: m.createdAt,
        citations: m.citations,
        reasoning: m.reasoning,
        promptTokens: m.promptTokens,
        completionTokens: m.completionTokens,
        totalTokens: m.totalTokens,
        elapsedMs: m.elapsedMs,
        phase: m.phase,
        // 主动提问问卷：按出参回显锚点行承载的题目与作答；仍待回答时（phase=suspended）由
        // 输入区问卷面板接管，已作答时消息下方渲染历史卡片并可「重新回答」回退
        questionnaire: m.questionnaire
          ? {
              questions: m.questionnaire.questions,
              answers: m.questionnaire.answers,
              awaiting: m.questionnaire.awaiting,
            }
          : undefined,
        // 历史压缩记录：后端按压缩游标把过往的压缩推进（含静默滚动 rolling）锚定到被折叠的最后
        // 一条消息，回显为完成态（无流式摘要增量、无压缩后占用对比），复用现有压缩块渲染
        compress: m.compress
          ? {
              phase: "done" as const,
              draft: "",
              reason:
                m.compress.reason === "manual"
                  ? "manual"
                  : m.compress.reason === "rolling"
                    ? "rolling"
                    : "auto",
              tokensBefore: m.compress.tokensBefore,
              foldedCount: m.compress.foldedCount,
            }
          : undefined,
      }));
      scrollToBottom();
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
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
  streaming.value = false;
  // 会话记录了缺省档位时同步到档位下拉；未记录时保持当前选择
  const target = sessions.value.find((s) => s.id === id);
  if (target && target.slot) {
    slot.value = target.slot;
  }
  // 会话级审核阀门随会话回显，避免沿用上一会话的选择
  if (target && target.reviewValve) {
    reviewValve.value = target.reviewValve;
  }
  await loadMessages(id);
  // 载入历史后探测该会话是否有在途运行（在跑 / 待审批），有则续播
  await resumeLiveRun(id);
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
  streaming.value = false;
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
  streaming.value = false;
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
const streaming = ref(false);
// creating 是否正在创建会话：未选中会话时首条消息需先建会话再发起流式生成，
// 这段时间既不属 streaming 也无待审批项，单独标记以免同一问题被重复提交；
// 同时驱动发送按钮转入加载态，把创建等待期（后端同步生成主题，耗时明显）显式反馈给用户
const creating = ref(false);
// optimizing 是否正在优化提问：调用提示词优化接口改写当前输入，
// 与 streaming / 待审批 / 创建会话同为运行态，期间禁止提交（见 canSend）
const optimizing = ref(false);
// compacting 是否正在手动压缩上下文（「压缩到此」触发的独立一次性 SSE 流）：
// 与 streaming / 待审批 / 创建会话同为运行态——在途期间不得再向后端发起生成（见 canSend），
// 但提问仍可转入待发队列排队（见 canEnqueueQueued / onSubmitClick）
const compacting = ref(false);

// awaitingApproval 是否存在等待人工审批的接口调用：为 true 时本轮生成已挂起，禁止再发起新生成
const awaitingApproval = computed<boolean>(() =>
  messages.value.some((m) => m.awaitingApproval === true),
);

// canSend 是否可发起生成：AI 可用、不在等待审批中、也不在创建会话途中。
// 不要求已有会话——未选中会话时发送即先创建会话（主题由后端据该问题生成），
// 因此打开抽屉后可直接输入并发出首条消息，无需先点「新建会话」。
// creating 必须一并纳入闸门：创建途中再次回车/点击会带着同一问题重复建会话
// （正文已在回显时清空，见 echoUserQuery）；optimizing 同理，优化途中输入框内容即将被整体替换；
// compacting 同理，压缩流在途时上下文正被折叠，此时新提问只能进待发队列（见 canEnqueueQueued）；
// questionnairePending 同理，问卷待作答时会话被阻塞，不得另起新生成
const canSend = computed<boolean>(
  () =>
    aiEnabled.value &&
    !awaitingApproval.value &&
    !questionnairePending.value &&
    !creating.value &&
    !optimizing.value &&
    !compacting.value,
);

// composerActionsEnabled 输入区动作（优化 / 撤回）的公共闸门：AI 可用，且无在途生成 / 待审批 /
// 会话创建 / 优化请求——这些运行态下输入框内容或将被整体替换、或无权改写
const composerActionsEnabled = computed<boolean>(
  () =>
    aiEnabled.value &&
    !streaming.value &&
    !awaitingApproval.value &&
    !creating.value &&
    !optimizing.value,
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

// ---- 用户消息快捷操作（复制 / 回退 / 删除）----
// busy 当前会话是否正忙（生成中 / 等待审批 / 创建会话中 / 手动压缩中）：忙时禁止截断，避免删除在途运行正在使用的
// 消息，导致服务端与本地内容错位；发送入口亦据此把新提问转入待发队列（见 onSubmitClick / onComposerEnter）
const busy = computed<boolean>(
  () => streaming.value || awaitingApproval.value || creating.value || compacting.value,
);

// ---- 待发提问队列（生成中提交的追问）----
// QueuedQuery 队列中的一条待发提问；key 仅用于列表渲染与操作定位，不随正文持久化
interface QueuedQuery {
  key: number;
  text: string;
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
// 创建会话（尚无会话可归属）与优化途中（输入将被整体改写）仍禁止；
// 手动压缩在途则允许——此时新提问发不出去，排队正是唯一去处（见 canSend / busy）；
// 问卷待作答时会话被阻塞，亦不得入队
const canEnqueueQueued = computed<boolean>(
  () =>
    aiEnabled.value &&
    activeSessionID.value !== null &&
    query.value.trim() !== "" &&
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
  agentQueues.value[sessionID].push({ key: nextKey(), text });
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

// drainQueuedQuery 当前回答结束后自动投递队首提问：仅当目标会话即当前会话且无在途运行时执行
// （切走的会话其队列留待切回时再投递，避免把消息推入非当前会话的消息区）；先出队持久化再发起生成
const drainQueuedQuery = (sessionID: number): void => {
  if (sessionID !== activeSessionID.value || busy.value) {
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
  startStream(sessionID, head.text);
};

// onSubmitClick 发送按钮 / 回车统一入口：空闲时直接发送，运行中（生成 / 待审批 / 建会话 / 压缩）则转入待发队列排队
const onSubmitClick = (): void => {
  if (busy.value) {
    enqueueQuery();
    return;
  }
  void sendQuery();
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
      startStream(sessionID, question.content);
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
  pendingMessage.value = { key: m.key, userIndex, content: m.content, rollback };
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
      const localIndex = messages.value.findIndex((m) => m.key === target.key);
      if (localIndex >= 0) {
        messages.value.splice(localIndex);
      }
      if (target.rollback) {
        query.value = target.content;
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

// StreamStarter 发起一次 Agent SSE 流并返回退订函数：首轮生成用 agentStreamGenerate，
// 审批续跑用 agentToolResultStream，刷新 / 重开抽屉续播用 agentStreamResume；
// 三者共用 consumeAgentStream 的回调处理，退订句柄由 agentRun store 持有。
type StreamStarter = (handlers: AgentStreamHandlers) => () => void;

// pruneSubmittedCalls 续跑流一旦产生输出，即移除已回传后端的历史调用（保留当前轮待展示项）；
// 待审批项清空后整体置空，避免气泡长期残留审批面板
const pruneSubmittedCalls = (msg: ChatMessage): void => {
  if (!msg.pendingCalls || msg.pendingCalls.length === 0) {
    return;
  }
  const remain = msg.pendingCalls.filter((c) => c.submitted !== true);
  msg.pendingCalls = remain.length > 0 ? remain : undefined;
};

// ASK_USER_TOOL_NAME 主动提问客户端工具名（与后端登记一致）：命中即豁免人工审批，改由问卷面板承载。
const ASK_USER_TOOL_NAME = "ask_user";

// parseQuestionnaireArgs 把 ask_user 调用的入参文本解析为问卷载荷（题目列表）；无法解析为含题目的
// 合法问卷时返回 null（退回按普通调用处理）。
const parseQuestionnaireArgs = (argumentsText: string): QuestionnairePayload | null => {
  try {
    const parsed = JSON.parse(argumentsText) as QuestionnairePayload;
    if (!parsed || !Array.isArray(parsed.questions) || parsed.questions.length === 0) {
      return null;
    }
    return { questions: parsed.questions };
  } catch {
    return null;
  }
};

// approvalCallsOf 某条助手消息中需人工审批的调用（排除 ask_user：问卷类调用豁免审批、不出现于审批
// 面板，改由问卷面板承载），供审批面板按剩余调用渲染
const approvalCallsOf = (m: ChatMessage): PendingCallState[] =>
  (m.pendingCalls ?? []).filter((c) => c.toolName !== ASK_USER_TOOL_NAME);

// AssistantBlock 助手消息体内按时序排布的内容块。助手一轮回答在时间上由「若干轮思考 + 工具调用」
// 串成，中途可能因人工审批 / 主动提问暂停并等待用户操作、随后续跑——故审批与问卷须按其触发轮次
// 内联插入，三者同处一条时间线上，作答 / 批准之后新增的思考与调用必然落在交互块之后，整条消息
// 自上而下严格按时序阅读（此前二者被固定渲染在正文之后，导致作答后的思考反而落到问卷上方）
interface AssistantBlock {
  key: string;
  kind: "reasoning" | "stages" | "questionnaire" | "approval";
  // text 仅 kind=reasoning 使用：该轮思考文本
  text?: string;
  // stages / parallelCount 仅 kind=stages 使用：该轮可见调用行与其数量（>1 时展示并行计数）
  stages?: AgentStageState[];
  parallelCount?: number;
}

// anchorRoundOf 求若干次调用的内联锚点轮次：取这些调用对应阶段行所属轮次的最小值（最早那轮）。
// 无对应阶段行、或该行缺 round 时返回 undefined，由调用方回落为「所有调用组之后」
const anchorRoundOf = (m: ChatMessage, callIDs: string[]): number | undefined => {
  let round: number | undefined;
  for (const callID of callIDs) {
    const r = (m.stages ?? []).find((s) => s.callID === callID)?.round;
    if (r === undefined) {
      continue;
    }
    if (round === undefined || r < round) {
      round = r;
    }
  }
  return round;
};

// assistantBlocksOf 把一条助手消息体内的内容块按时序展开：逐轮产出「该轮思考块 + 该轮可见调用组」，
// 并在问卷 / 审批各自的锚点轮次后插入其交互块。ask_user 恒不自成工具调用行——问卷面板即该调用的
// 呈现——故仅从**可见调用行**中剔除（聚组仍以全量阶段行进行：锚定到 ask_user 的那段思考须照常解析，
// 渲染为该轮思考块、置于问卷之上，否则该段思考会因锚点行被剔除而无处安放、整段丢失）。锚点轮次取不到
// （历史消息不携带阶段行）时，交互块回落排在所有调用组之后（仍居于正文之上，保持「先提问 / 审批、
// 后出结果」的时序）
const assistantBlocksOf = (m: ChatMessage): AssistantBlock[] => {
  const blocks: AssistantBlock[] = [];
  const askUserCallIDs = (m.stages ?? [])
    .filter((s) => s.tool === ASK_USER_TOOL_NAME)
    .map((s) => s.callID)
    .filter((id): id is string => !!id);
  const approvalCallIDs = approvalCallsOf(m).map((c) => c.callID);
  const hasQuestionnaire = !!m.questionnaire;
  const hasApproval = approvalCallIDs.length > 0;
  const questionRound = anchorRoundOf(m, askUserCallIDs);
  const approvalRound = anchorRoundOf(m, approvalCallIDs);
  let placedQuestionnaire = false;
  let placedApproval = false;
  // 聚组传入全量阶段行（含 ask_user），逐组再筛出可展示的调用行（剔除 ask_user）
  for (const g of stageGroupsOf(m, m.stages ?? [])) {
    if (g.reasoningText) {
      blocks.push({ key: `r-${g.key}`, kind: "reasoning", text: g.reasoningText });
    }
    const visibleStages = g.stages.filter((s) => s.tool !== ASK_USER_TOOL_NAME);
    if (visibleStages.length > 0) {
      blocks.push({
        key: `s-${g.key}`,
        kind: "stages",
        stages: visibleStages,
        parallelCount: visibleStages.length,
      });
    }
    if (hasQuestionnaire && questionRound !== undefined && g.round === questionRound) {
      blocks.push({ key: `q-${g.key}`, kind: "questionnaire" });
      placedQuestionnaire = true;
    }
    if (hasApproval && approvalRound !== undefined && g.round === approvalRound) {
      blocks.push({ key: `a-${g.key}`, kind: "approval" });
      placedApproval = true;
    }
  }
  if (hasQuestionnaire && !placedQuestionnaire) {
    blocks.push({ key: "q-tail", kind: "questionnaire" });
  }
  if (hasApproval && !placedApproval) {
    blocks.push({ key: "a-tail", kind: "approval" });
  }
  return blocks;
};

// applyToolRequest 应用一次挂起事件：把待审批调用累积到消息（按 callID 幂等，回放重复下发
// 不重复入列），置为等待审批态；免审批调用直接预置为已批准。问卷类调用（ask_user）豁免审批：不入
// 审批面板，改为据其入参重建待回答问卷（decision 留空，待用户提交 / 取消后再定），其待执行调用仍
// 登记在 pendingCalls 中（提交续跑需原样回传 callID），本轮全部调用在问卷作答后一次性整体提交。
const applyToolRequest = (assistantKey: number, req: ToolRequestPayload): void => {
  const msg = messages.value.find((m) => m.key === assistantKey);
  if (!msg) {
    return;
  }
  // 挂起锚点消息 ID：挂起时后端已把本回合的助手消息落库为锚点行，并随每条 tool_request 一并下发其 ID。
  // 实时会话的本地气泡原本不带 id（只有 loadMessages 回读历史时才带），登记它后「重新回答」才能按锚点
  // 截断其后消息并重答；缺省（旧服务端未下发）时保持无 id，行为与引入该字段前一致
  if (req.messageID !== undefined) {
    msg.id = req.messageID;
  }
  pruneSubmittedCalls(msg);
  const calls = (msg.pendingCalls ??= []);
  if (!calls.some((c) => c.callID === req.callID)) {
    const call: PendingCallState = {
      callID: req.callID,
      toolName: req.toolName,
      arguments: req.arguments,
      auditLevel: req.auditLevel,
      approvalRequired: req.approvalRequired,
      execOwner: req.execOwner,
    };
    if (req.toolName === ASK_USER_TOOL_NAME) {
      // 问卷类调用：不预置已批准，把本条调用的题目并入该消息的问卷（一轮内可能有多条 ask_user
      // 调用，合并为一张问卷、只弹一个面板，作答后逐条回传结果）。按题目 id 去重，使刷新后的
      // 回放（消息出参已带来锚点载荷，事件重放再叠加）不重复累积；保留历史答案以支持回退后预填。
      const payload = parseQuestionnaireArgs(req.arguments);
      if (payload) {
        const prev = msg.questionnaire;
        const seen = new Set((prev?.questions ?? []).map((q) => q.id));
        const fresh = payload.questions.filter((q) => !seen.has(q.id));
        msg.questionnaire = {
          questions: [...(prev?.questions ?? []), ...fresh],
          answers: prev?.answers,
          awaiting: true,
        };
      }
    } else if (req.approvalRequired === false) {
      // 免审批调用（approvalRequired=false）：不弹人工审批，直接置为已批准；
      // 与需人工审批的调用同处一批，待全部决策后整体提交
      call.decision = "approved";
    }
    calls.push(call);
  }
  msg.awaitingApproval = true;
  msg.streaming = false;
  scrollToBottom();
};

// ---- 上下文压缩 ----
// （手动压缩在途标记 compacting 与 streaming / creating 等运行态一并声明，见上方输入区）
// pendingCompact 手动压缩的待补全项：压缩那一刻尚无后续生成，先展示「压缩前占用 + 折叠条数」，
// 待该会话随后首次生成的 done.promptTokens 到达再补上「压缩后占用」并清空
const pendingCompact = ref<{
  sessionID: number;
  messageKey: number;
  tokensBefore?: number;
  foldedCount?: number;
} | null>(null);

// applyCompressStart 初始化某条助手消息的压缩态（自动挂在当前助手消息、手动挂在被点击消息）
const applyCompressStart = (key: number, p: CompressStartPayload): void => {
  const msg = messages.value.find((m) => m.key === key);
  if (!msg) {
    return;
  }
  msg.compress = {
    phase: "running",
    draft: "",
    reason: p.reason === "manual" ? "manual" : "auto",
    tokensBefore: p.tokens,
  };
  scrollToBottom();
};

// applyCompressDelta 把摘要增量追加到压缩态（流式呈现）
const applyCompressDelta = (key: number, p: CompressDeltaPayload): void => {
  const msg = messages.value.find((m) => m.key === key);
  if (msg?.compress) {
    msg.compress.draft += p.delta;
    scrollToBottom();
  }
};

// applyCompressDone 置压缩完成态并写入折叠条数（foldedCount 为 0 或缺失时仍置 done）；
// 完成后刷新会话列表以更新摘要与占用
const applyCompressDone = (key: number, p: CompressDonePayload): void => {
  const msg = messages.value.find((m) => m.key === key);
  if (msg?.compress) {
    msg.compress.phase = "done";
    msg.compress.foldedCount = p.foldedCount;
    if (p.tokensBefore !== undefined) {
      msg.compress.tokensBefore = p.tokensBefore;
    }
  }
  scrollToBottom();
  void loadSessions();
};

// applyPendingCompact 用随后首次生成的 done.promptTokens 补全手动压缩的「压缩后占用」，
// 仅当生成所属会话与待补全会话一致时生效；补全后清空待补全项
const applyPendingCompact = (sessionID: number, promptTokens?: number): void => {
  const pending = pendingCompact.value;
  if (!pending || pending.sessionID !== sessionID || promptTokens === undefined) {
    return;
  }
  const msg = messages.value.find((m) => m.key === pending.messageKey);
  if (msg?.compress) {
    msg.compress.tokensAfter = promptTokens;
  }
  pendingCompact.value = null;
};

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

// compressTextOf 压缩展示态文案：进行中提示压缩中、失败提示失败、完成优先展示前后对比
// （已补全 tokensAfter 且当前档位容量已知时），否则仅展示压缩前占用与折叠条数
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
  if (c.tokensAfter !== undefined && contextLimitTokens.value > 0) {
    return i18n.global.t("page.aiAssistant.compressCompare", {
      before: c.tokensBefore ?? 0,
      after: c.tokensAfter,
      percent: Math.round((c.tokensAfter / contextLimitTokens.value) * 100),
    });
  }
  return i18n.global.t("page.aiAssistant.compressDone", {
    before: c.tokensBefore ?? 0,
    count: c.foldedCount ?? 0,
  });
};

// finishCompact 手动压缩收尾：落下在途标记并随即投递排队中的提问（压缩期间不允许直接发起生成，
// 提问只能进待发队列，见 canSend / canEnqueueQueued）。压缩流的完成 / 失败 / 关闭三处均经此收尾
const finishCompact = (sessionID: number): void => {
  compacting.value = false;
  drainQueuedQuery(sessionID);
};

// compactToHere 手动把上下文压缩到该助手消息所属轮次：以该轮提问的用户消息序号为折叠边界调
// 独立 SSE 压缩流，事件复用自动压缩的展示块逻辑挂到该消息；压缩后占用待随后首次生成补全
const compactToHere = (m: ChatMessage): void => {
  const sessionID = activeSessionID.value;
  // busy 已含 compacting：手动压缩亦属在途运行，串行触发由同一闸门兜住
  if (sessionID === null || busy.value) {
    return;
  }
  const question = precedingUserMessageOf(m.key);
  const idx = question ? userMessageIndexOf(question.key) : 0;
  if (idx < 1) {
    return;
  }
  compacting.value = true;
  agentSessionCompactStream(
    sessionID,
    { userMessageIndex: idx },
    {
      onCompressStart: (p) => applyCompressStart(m.key, p),
      onCompressDelta: (p) => applyCompressDelta(m.key, p),
      onCompressDone: (p) => {
        applyCompressDone(m.key, p);
        // 压缩已完成但尚无后续生成：暂存待补全项，待该会话随后首次生成 done 补上压缩后占用
        pendingCompact.value = {
          sessionID,
          messageKey: m.key,
          tokensBefore: p.tokensBefore ?? m.compress?.tokensBefore,
          foldedCount: p.foldedCount,
        };
        appStore.notify({
          text: i18n.global.t("page.aiAssistant.compressSucceeded"),
          timeout: 3000,
          color: "success",
        });
        finishCompact(sessionID);
      },
      onError: (message) => {
        const target = messages.value.find((x) => x.key === m.key);
        if (target) {
          if (target.compress) {
            target.compress.phase = "failed";
          } else {
            target.compress = { phase: "failed", draft: "", reason: "manual" };
          }
        }
        // 优先展示后端本地化摘要，缺失时回落到通用压缩失败文案
        notifyError(message || i18n.global.t("page.aiAssistant.compressFailed"));
        finishCompact(sessionID);
      },
      onComplete: () => {
        finishCompact(sessionID);
      },
    },
  );
};

// consumeAgentStream 消费一次 Agent SSE 流（首轮生成 / 审批续跑 / 续播共用同一套回调）：
// 增量累积到消息、阶段事件归并到阶段行、done 回填结果；收到 tool_request 则挂起等待审批；
// onSeq 持久化续播游标；退订句柄交由 agentRun store 持有（组件重挂载不影响在途流）。
// opts.historical 为真表示本次是回放已收敛运行（中断后重开抽屉 / 刷新续播）：中断已是既成事实，
// 不再弹错误提示打扰用户；中断提示由模板据消息的 phase（failed/cancelled）静态渲染，无需回放补写
const consumeAgentStream = (
  sessionID: number,
  assistantKey: number,
  start: StreamStarter,
  opts?: { historical?: boolean },
): void => {
  streaming.value = true;
  const unsubscribe = start({
    // 事件序号：更新续播游标（供刷新 / 重连后 agentStreamResume 的 after 参数）
    onSeq: (seq: number) => {
      agentRun.noteSeq(sessionID, seq);
    },
    onDelta: (delta: string) => {
      agentRun.appendBuffer(sessionID, delta);
      const msg = messages.value.find((m) => m.key === assistantKey);
      if (msg) {
        pruneSubmittedCalls(msg);
        msg.content += delta;
        scrollToBottom();
      }
    },
    // 思考过程增量事件：累积到当前助手消息的思考分段（按「轮」切分），流式期间默认展开折叠区块。
    // 除消息区外，思考正文自身的限高滚动区也要贴底，否则超出限高后最新思考留在视野之外
    onReasoning: (delta: string) => {
      const msg = messages.value.find((m) => m.key === assistantKey);
      if (msg) {
        pruneSubmittedCalls(msg);
        appendReasoning(msg, delta);
        scrollToBottom();
        scrollReasoningToBottom(assistantKey);
      }
    },
    // 工具调用阶段事件：一次调用发起（toolCall/start）时把当前未锚定的思考分段锚定到该次调用，
    // 使其渲染在该调用行上方；阶段行按 callID 归并递进
    onStage: (ev: AgentStageEvent) => {
      const msg = messages.value.find((m) => m.key === assistantKey);
      if (msg) {
        pruneSubmittedCalls(msg);
        if (ev.stage === "toolCall" && ev.status === "start" && ev.callID) {
          anchorReasoningSegment(msg, ev.callID);
        }
      }
      applyAgentStage(assistantKey, ev);
    },
    // 挂起事件：累积本轮待审批调用（同一轮可能多条，按 callID 累加而非覆盖）。
    // 流已正常 EOF（无 done / error），故不视为失败：保持 streaming=false
    onToolRequest: (req: ToolRequestPayload) => {
      agentRun.addPendingCall(sessionID, req);
      applyToolRequest(assistantKey, req);
      // 本轮挂起：思考过程流随之结束，折叠区块自动收起
      const msg = messages.value.find((m) => m.key === assistantKey);
      if (msg) {
        endReasoningStreaming(msg);
      }
      streaming.value = false;
      void loadSessions();
    },
    // 上下文压缩事件（自动压缩经 RunHub 发布、带 id 触发 onSeq）：展示块挂在当前助手消息上，
    // 与手动压缩复用同一套 applyCompress* 逻辑
    onCompressStart: (p) => applyCompressStart(assistantKey, p),
    onCompressDelta: (p) => applyCompressDelta(assistantKey, p),
    onCompressDone: (p) => applyCompressDone(assistantKey, p),
    onDone: (out) => {
      agentRun.settle(sessionID);
      const msg = messages.value.find((m) => m.key === assistantKey);
      if (msg) {
        pruneSubmittedCalls(msg);
        msg.content = out.content ?? msg.content;
        msg.citations = out.citations;
        // 用量元信息随 done 事件回填（后端已同时落库，故刷新后仍可展示）
        msg.promptTokens = out.promptTokens;
        msg.completionTokens = out.completionTokens;
        msg.totalTokens = out.totalTokens;
        msg.elapsedMs = out.elapsedMs;
        // 思考过程随 done 收敛为全文并结束流式；折叠区块自动收起（无思考时保持既有值不变）
        msg.reasoning = out.reasoning ?? msg.reasoning;
        endReasoningStreaming(msg);
        msg.streaming = false;
        // 自动压缩已于本轮完成：本轮 done.promptTokens 即压缩后的真实占用，补全前后对比
        if (msg.compress?.phase === "done") {
          msg.compress.tokensAfter = out.promptTokens;
        }
        scrollToBottom();
      }
      streaming.value = false;
      // 手动压缩的待补全项：本轮生成即该会话随后首次生成，用其 promptTokens 补全压缩后占用
      applyPendingCompact(sessionID, out.promptTokens);
      // 会话活跃时间变化，刷新列表以维持最近活跃在前的排序
      void loadSessions();
    },
    // 流正常 EOF：本轮挂起调用已全部送达。若全部为免审批调用（或用户已全部决策），
    // 在此统一触发整体提交；仍需人工决策的调用保持等待，不受影响。
    // 同时在此（流已完全关闭、可安全另起一条连接）自动投递队首待发提问
    onComplete: () => {
      const msg = messages.value.find((m) => m.key === assistantKey);
      if (msg) {
        trySubmitApprovals(msg);
      }
      if (!opts?.historical) {
        drainQueuedQuery(sessionID);
      }
    },
    onError: (message: string, phase?: string) => {
      agentRun.settle(sessionID);
      const msg = messages.value.find((m) => m.key === assistantKey);
      // 优先展示后端本地化摘要，缺失时回落到通用失败文案
      const text = message || i18n.global.t("page.aiAssistant.error");
      if (msg) {
        msg.streaming = false;
        // 思考过程随失败结束流式，折叠区块自动收起（保留已累积的思考内容）
        endReasoningStreaming(msg);
        // 恢复失败的调用已不可执行，清空待审批态避免界面卡在等待中
        msg.awaitingApproval = false;
        msg.pendingCalls = undefined;
        // 运行生命周期阶段随 error 事件回填：failed/cancelled 时由模板据阶段渲染中断提示横幅
        // （文案由前端 i18n 提供，不再拼接进正文，避免提示语混入后续喂回模型的内容）
        if (phase) {
          msg.phase = phase;
        }
      }
      streaming.value = false;
      // 回放已收敛运行的中断（续播）不再弹提示：该中断在用户离开时即已发生，重开抽屉时再提示属打扰
      if (!opts?.historical) {
        notifyError(text);
        // 本轮以失败收尾（不再触发 onComplete 时）同样投递队首待发提问；重复调用由 busy 闸门兜住
        drainQueuedQuery(sessionID);
      }
    },
  });
  agentRun.attach(sessionID, unsubscribe);
};

// ---- 人工审批（决策 + 结果回传）----

// trySubmitApprovals 当本轮（尚未回传的）待审批调用全部已决策时，一次性回传全部结果并续跑；
// 后端把运行现场一次性消费，故同一轮必须整体提交，不能逐条提交
const trySubmitApprovals = (msg: ChatMessage): void => {
  if (activeSessionID.value === null || msg.awaitingApproval !== true) {
    return;
  }
  const batch = (msg.pendingCalls ?? []).filter((c) => c.submitted !== true);
  if (batch.length === 0 || batch.some((c) => !c.decision)) {
    return;
  }
  const results: ToolResultItemIn[] = batch.map((c) => ({
    callID: c.callID,
    approved: c.decision === "approved",
    result: c.result ?? "",
    error: c.error ?? "",
  }));
  batch.forEach((c) => {
    c.submitted = true;
  });
  msg.awaitingApproval = false;
  const sessionID = activeSessionID.value;
  agentRun.markAwaiting(sessionID, false);
  consumeAgentStream(sessionID, msg.key, (handlers) =>
    agentToolResultStream(sessionID, { results }, handlers),
  );
};

// approveCall 批准该调用：记录决策并尝试提交本轮结果
const approveCall = (msg: ChatMessage, call: PendingCallState): void => {
  if (call.decision) {
    return;
  }
  call.decision = "approved";
  trySubmitApprovals(msg);
};

// rejectCall 拒绝：仅记录决策，不执行任何调用
const rejectCall = (msg: ChatMessage, call: PendingCallState): void => {
  if (call.decision) {
    return;
  }
  call.decision = "rejected";
  trySubmitApprovals(msg);
};

// ---- 主动提问（问卷）----
// awaitingQuestionnaireMsg 当前会话中仍待回答的问卷所在助手消息（取最近一条）：非空时问卷已内联在
// 消息流中（见 assistantBlocksOf 的 questionnaire 块），会话阻塞直至用户提交或取消回答
const awaitingQuestionnaireMsg = computed<ChatMessage | null>(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const m = messages.value[i];
    if (m.questionnaire?.awaiting) {
      return m;
    }
  }
  return null;
});

// questionnairePending 当前会话是否正等待问卷作答：为 true 时输入区整体禁用并给出跳转提示
// （会话阻塞的语义不变，只是把「遮蔽输入区」改为「禁用输入区」——问卷本身已内联进消息流）
const questionnairePending = computed<boolean>(() => awaitingQuestionnaireMsg.value !== null);

// questionnairePayloadOf 取某条消息的问卷载荷（题目 + 上次答案），供问卷面板渲染与预填
const questionnairePayloadOf = (m: ChatMessage): QuestionnairePayload => ({
  questions: m.questionnaire?.questions ?? [],
  answers: m.questionnaire?.answers,
});

// questionnaireAnswerTextOf 取某题的用户作答展示文本：已作答取答案文本，未作答取本地化「未作答」
const questionnaireAnswerTextOf = (q: QuestionnaireState, questionID: string): string => {
  const ans = q.answers?.find((a) => a.questionID === questionID);
  if (!ans || !ans.answered) {
    return i18n.global.t("page.aiAssistant.questionnaireNoAnswer");
  }
  return ans.answer;
};

// submitQuestionnaire 提交问卷：把作答归一化为各条 ask_user 调用的执行结果（approved=true、result 为
// 答案 JSON）并逐条标记已决策，随后在本轮全部调用决策齐备时一次性整体回传续跑（见 trySubmitApprovals）。
// 一轮内可能有多条 ask_user 调用（题目前端已合并为一张问卷），故按题目归属把答案拆回各条调用：
// 每条调用只取自己 arguments 中声明的题目所对应的答案。
const submitQuestionnaire = (payload: { answers: QuestionnaireAnswer[] }): void => {
  const msg = awaitingQuestionnaireMsg.value;
  if (!msg) {
    return;
  }
  const calls = (msg.pendingCalls ?? []).filter(
    (c) => c.toolName === ASK_USER_TOOL_NAME && !c.decision,
  );
  if (calls.length === 0) {
    return;
  }
  msg.questionnaire = { ...msg.questionnaire!, answers: payload.answers, awaiting: false };
  for (const call of calls) {
    const ids = new Set(parseQuestionnaireArgs(call.arguments)?.questions.map((q) => q.id) ?? []);
    call.decision = "approved";
    call.result = JSON.stringify({
      answers: payload.answers.filter((a) => ids.has(a.questionID)),
    });
  }
  trySubmitApprovals(msg);
};

// cancelQuestionnaire 取消回答：逐条以 approved=false 回传（模型收到既有拒绝文案），并收起问卷面板
const cancelQuestionnaire = (): void => {
  const msg = awaitingQuestionnaireMsg.value;
  if (!msg) {
    return;
  }
  const calls = (msg.pendingCalls ?? []).filter(
    (c) => c.toolName === ASK_USER_TOOL_NAME && !c.decision,
  );
  if (calls.length === 0) {
    return;
  }
  msg.questionnaire = { ...msg.questionnaire!, awaiting: false };
  for (const call of calls) {
    call.decision = "rejected";
  }
  trySubmitApprovals(msg);
};

// 待确认「重新回答」的问卷锚点消息 key：确认后会截断该锚点之后的全部消息并重新生成，故须先经弹窗
// 提醒。仅存 key 而不存消息对象——确认时按 key 取回（其间消息列表可能已被重载）
const pendingReanswerKey = ref<number | null>(null);
const reanswerConfirmRef = useTemplateRef("reanswerConfirmRef");

// openReanswerConfirm 打开「重新回答」确认弹窗（问卷卡片按钮的入口）：忙或锚点未就绪时不进入确认
const openReanswerConfirm = (m: ChatMessage): void => {
  if (busy.value || m.id === undefined) {
    return;
  }
  pendingReanswerKey.value = m.key;
  reanswerConfirmRef.value?.open({});
};

// confirmReanswer 确认重新回答：按 key 取回锚点消息后执行（截断与重建见 reanswerQuestionnaire）
const confirmReanswer = async (): Promise<void> => {
  const key = pendingReanswerKey.value;
  pendingReanswerKey.value = null;
  if (key === null) {
    return;
  }
  const m = messages.value.find((x) => x.key === key);
  if (!m) {
    return;
  }
  await reanswerQuestionnaire(m);
};

// reanswerQuestionnaire 回退（重新回答）一条已作答的问卷：调用回退接口截断该锚点之后的消息、重置
// 该轮回答与关联现场，并按返回载荷预填重开问卷面板（含待执行调用，供重答后经 tool-result 续跑）
const reanswerQuestionnaire = async (m: ChatMessage): Promise<void> => {
  const sessionID = activeSessionID.value;
  if (sessionID === null || m.id === undefined || busy.value) {
    return;
  }
  await getResponse<AgentMessageQuestionnaire>(
    agentQuestionnaireReanswer(sessionID, { messageID: m.id }),
    async (rsp) => {
      // 本地同构截断：删除锚点之后的全部消息（该回合此前的最终回答及其后各轮）
      const index = messages.value.findIndex((x) => x.key === m.key);
      if (index >= 0) {
        messages.value.splice(index + 1);
      }
      // 锚点行清空正文并回到待回答：重建待执行调用（callID 为提交续跑所必需），并按返回载荷预填。
      // 非问卷调用按后端给出的「需审批」结论保持未决策，问卷调用留待用户作答后再定
      m.content = "";
      m.phase = "suspended";
      m.streaming = false;
      m.awaitingApproval = true;
      m.pendingCalls = (rsp.data.pendingCalls ?? []).map((c) => ({
        callID: c.callID,
        toolName: c.toolName,
        arguments: c.arguments,
        auditLevel: c.auditLevel,
        approvalRequired: c.approvalRequired,
        execOwner: c.execOwner,
        decision:
          c.approvalRequired === false && c.toolName !== ASK_USER_TOOL_NAME
            ? "approved"
            : undefined,
      }));
      m.questionnaire = {
        questions: rsp.data.questions,
        answers: rsp.data.answers,
        awaiting: true,
      };
      scrollToBottom();
    },
    async (rsp) => {
      notifyError(rsp.summary);
    },
    notifyNoResponse,
  );
};

// startStream 推送用户消息与流式占位消息并发起首轮流式生成：SSE 增量经 onDelta 累积，
// done 事件回填内容与引用（会话此时已存在）；挂起时转为等待审批。
// echoedUserKey 为首条消息在「创建会话」等待期已本地回显的气泡 key（见 echoUserQuery）：
// 气泡仍在时只补流式占位消息，避免同一提问出现两次；若等待期消息区被清空（如创建途中点
// 「新建会话」）致回显丢失，则回落为按常规补推用户气泡，保证会话内容与服务端一致
const startStream = (sessionID: number, q: string, echoedUserKey?: number): void => {
  const assistantKey = nextKey();
  const echoed = echoedUserKey !== undefined && messages.value.some((m) => m.key === echoedUserKey);
  if (!echoed) {
    messages.value.push({
      key: nextKey(),
      role: "user",
      content: q,
      createdAt: new Date().toISOString(),
    });
  }
  messages.value.push({ key: assistantKey, role: "assistant", content: "", streaming: true });
  query.value = "";
  scrollToBottom();
  // 新运行开始：重置该会话的运行态与续播游标
  agentRun.begin(sessionID);

  consumeAgentStream(sessionID, assistantKey, (handlers) =>
    agentStreamGenerate(
      sessionID,
      {
        query: q,
        // 本次生成的显式档位（为空时由后端回退到会话档位与默认档位参数）
        slot: slot.value || undefined,
        // 思考开关：仅当当前档位绑定模型支持时注入（开启 on / 关闭 off），不支持则不带该字段
        thinking: slotThinkingToggle.value ? (thinking.value ? "on" : "off") : undefined,
      },
      handlers,
    ),
  );
};

// echoUserQuery 首条消息本地回显：在「创建会话」等待期先把用户气泡推进消息区并清空输入框。
// 后端创建会话需同步调用模型生成主题、耗时明显，若等创建成功再推气泡，等待期消息区毫无反馈，
// 用户会误以为回车未被受理；返回该气泡 key，供创建未成功时回滚（见 rollbackUserQuery）
const echoUserQuery = (q: string): number => {
  const key = nextKey();
  messages.value.push({
    key,
    role: "user",
    content: q,
    createdAt: new Date().toISOString(),
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
const createSessionForQuery = async (firstQuery: string, userKey: number): Promise<void> => {
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
        startStream(rsp.data.id, firstQuery, userKey);
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
  event.preventDefault();
  // 运行中（生成 / 待审批 / 建会话 / 压缩）回车即把当前输入转入待发队列，让用户可连续抛出多个追问，
  // 而不必等本轮回答结束
  if (busy.value) {
    enqueueQuery();
    return;
  }
  if (!streaming.value && !awaitingApproval.value && !creating.value && !optimizing.value) {
    void sendQuery();
  }
};

// sendQuery 发起一次流式问答：先校验输入，未选中会话时先本地回显再创建会话，
// 会话已存在时直接推送用户消息与流式占位消息
const sendQuery = async (): Promise<void> => {
  // 等待审批期间本轮生成已挂起，不接受新的生成请求（须先完成或拒绝全部待审批调用）；
  // 创建会话途中同理拒绝，避免同一问题重复建会话
  if (streaming.value || awaitingApproval.value || creating.value) {
    return;
  }
  const q = query.value.trim();
  if (!q) {
    notifyError(i18n.global.t("page.aiAssistant.emptyQuery"));
    return;
  }
  // 消息已发出：撤回现场随之失效（正文在 startStream 清空，watch 亦会兜底）
  optimizeUndo.value = null;
  // 未选中会话（含点「新建会话」后的待创建态）：先据首个问题创建会话
  // （主题由后端生成并回传），创建成功后才发起流式生成。创建会话需后端同步调模型生成主题、
  // 耗时明显，故先本地回显用户气泡并让发送按钮转入加载态（见 canSend / creating），
  // 让「回车已被受理」在等待期即刻可见；创建未成功再回滚回显
  if (activeSessionID.value === null) {
    const userKey = echoUserQuery(q);
    await createSessionForQuery(q, userKey);
    return;
  }
  startStream(activeSessionID.value, q);
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

// cancelGeneration 点「停止」：显式请求服务端取消当前运行（真正中止，区别于仅断连接），
// 并退订当前流；本地停止闪烁占位消息，保留已生成的增量内容
const cancelGeneration = (): void => {
  const sessionID = activeSessionID.value;
  if (sessionID !== null) {
    // 显式停止以服务端为准，客户端失败（如运行已结束）不阻断本地复位
    void agentRunCancel(sessionID).catch(() => undefined);
    agentRun.detach(sessionID);
    agentRun.settle(sessionID);
  }
  streaming.value = false;
  // 停止后残留的流式占位消息停止闪烁，保留已生成的增量内容
  const last = messages.value[messages.value.length - 1];
  if (last && last.streaming) {
    last.streaming = false;
  }
  // 主动停止后同样自动投递队首待发提问（已退订旧流，可安全另起连接）
  if (sessionID !== null) {
    drainQueuedQuery(sessionID);
  }
};

// gotoLlmConfig 前往 LLM 配置页完成开通，并关闭抽屉让用户聚焦配置
const gotoLlmConfig = (): void => {
  opened.value = false;
  void router.push({ name: "Admin LLM Config", params: { language: route.params.language } });
};

// ---- 在途运行续播 ----
// fetchLiveRun 查询某会话的在途运行态（在跑 / 待审批）；无在途运行或查询失败时返回 null
const fetchLiveRun = async (id: number): Promise<AgentLiveRunOut | null> => {
  let live: AgentLiveRunOut | null = null;
  await getResponse<AgentStatusOut>(agentStatusGet(id), async (rsp) => {
    live = rsp.data?.liveRun ?? null;
  });
  return live;
};

// abandonLocalRun 会话已无在途运行却残留「生成中」态（如服务端退订宽限取消后重开）时，
// 清理本地运行态，避免界面永远卡在生成中
const abandonLocalRun = (id: number): void => {
  const state = agentRun.runs[id];
  if (!state || (!state.streaming && !state.awaitingApproval)) {
    return;
  }
  streaming.value = false;
  const last = messages.value[messages.value.length - 1];
  if (last && last.role === "assistant") {
    last.streaming = false;
    last.awaitingApproval = false;
    // 思考过程随运行中断结束流式并收起折叠区块（保留已累积的思考内容），避免残留「思考中…」
    endReasoningStreaming(last);
  }
  agentRun.settle(id);
};

// resetAssistantProgress 清空一条助手消息的「本轮增量态」（正文、思考分段、工具调用行、待审批项
// 与压缩展示态）：续播前进度未知时先把承载消息清空，再从持久化事件从头重建，避免重建出的增量与
// 已落库的正文相互叠加（重影）；历史消息为只读展示，清空后即由回放补齐。id / createdAt / phase 为
// 落库的既有事实（非本轮增量），一并保留——phase 为 failed / cancelled 时终端仍据此展示中断提示，
// 即使回放无法重现 error 事件也不丢该事实。
const resetAssistantProgress = (msg: ChatMessage): void => {
  msg.content = "";
  msg.reasoning = undefined;
  msg.reasoningSegments = undefined;
  msg.reasoningStreaming = undefined;
  msg.reasoningExpanded = undefined;
  msg.stages = undefined;
  msg.pendingCalls = undefined;
  msg.awaitingApproval = false;
  msg.citations = undefined;
  msg.compress = undefined;
  msg.promptTokens = undefined;
  msg.completionTokens = undefined;
  msg.totalTokens = undefined;
  msg.elapsedMs = undefined;
};

// resumeLiveRun 会话打开 / 切换 / 刷新后：探测在途运行态并复用 consumeAgentStream 回放续播。
// 在跑则续接实时流；待审批则据 live-run 重建待审批项（回放的 tool_request 亦会重建，按 callID
// 幂等）；已中断（failed / cancelled）则据已落库事件回放，重建本轮已生成的正文 / 思考 / 工具行
// （中断提示由模板据该行落库的 phase 静态渲染）——后端已把中断前的中间产物阶段性落库，回放即从零
// 重建该行。
const resumeLiveRun = async (id: number): Promise<void> => {
  // 重订阅前先退订旧流，保证同一会话至多一条订阅
  agentRun.detach(id);
  const live = await fetchLiveRun(id);
  if (!live || !live.runID) {
    abandonLocalRun(id);
    return;
  }
  const isStreaming = live.phase === "running";
  const isAwaiting = live.awaitingApproval === true;
  // 已中断（failed / cancelled）：本轮已产出的中间产物已落库，回放即可重建，故本地无在途态时
  // 仍继续续播（区别于已正常收尾的 done）
  const isInterrupted = !isStreaming && !isAwaiting && live.phase !== "done";
  // 服务端运行已收敛终态（done / failed / cancelled）且本地无在途态、亦非中断回放：无可续播内容
  // （本轮作答已收尾后关抽屉再重开即属此列），直接返回，避免误建一个空的助手占位气泡
  const local = agentRun.runs[id];
  if (
    !isStreaming &&
    !isAwaiting &&
    !local?.streaming &&
    !local?.awaitingApproval &&
    !isInterrupted
  ) {
    return;
  }
  // 决定承载消息与续播起点：
  //   同页关抽屉后重开（该气泡仍在流式 / 待审批）→ 复用该气泡，只补游标之后的增量；
  //   其余情形（刷新 / 切换会话 / 中断后重开）→ 复用已落库的末条助手消息（中断时后端已把中间产物
  //   落库为该行），或新建占位气泡，并从序号 0 完整回放重建——内存正文为空却只收游标之后的增量
  //   会出现头部缺口。
  let msg = messages.value[messages.value.length - 1];
  let after = agentRun.lastSeqOf(id);
  const resumedInPlace =
    !!msg && msg.role === "assistant" && (msg.streaming === true || msg.awaitingApproval === true);
  if (!resumedInPlace) {
    if (!msg || msg.role !== "assistant" || msg.id === undefined) {
      // 无已落库的助手行可供承载：运行仍在途时新建占位气泡；已中断则本轮未产出任何正文 / 思考，
      // 无产物可续播，不再造空气泡
      if (isInterrupted) {
        return;
      }
      msg = { key: nextKey(), role: "assistant", content: "" };
      messages.value.push(msg);
    } else {
      resetAssistantProgress(msg);
    }
    after = 0;
  }
  msg.streaming = isStreaming;
  msg.awaitingApproval = isAwaiting;
  if (isAwaiting && (live.pendingCalls?.length ?? 0) > 0) {
    msg.pendingCalls = live.pendingCalls.map((c) => ({
      callID: c.callID,
      toolName: c.toolName,
      arguments: c.arguments,
      auditLevel: c.auditLevel,
      approvalRequired: c.approvalRequired,
      execOwner: c.execOwner,
      // ask_user 问卷类调用豁免审批：不预置批准，留待用户在问卷面板作答后再定；
      // 其余免审批调用维持自动批准
      decision:
        c.approvalRequired === false && c.toolName !== ASK_USER_TOOL_NAME ? "approved" : undefined,
    }));
  }
  agentRun.restore(id, {
    runID: live.runID,
    streaming: isStreaming,
    awaitingApproval: isAwaiting,
    pendingCalls: msg.pendingCalls ?? [],
  });
  streaming.value = isStreaming;
  scrollToBottom();
  // 从上述起点回放：同页重开无缺口，跨页由序号 0 完整重建；historical 抑制回放中断的错误提示
  consumeAgentStream(id, msg.key, (handlers) => agentStreamResume(id, after, handlers), {
    historical: !isStreaming && !isAwaiting,
  });
  // 待审批续播时并非「生成中」：consumeAgentStream 默认置为生成中，这里按实际阶段校正
  streaming.value = isStreaming;
};

// ---- 生命周期 ----
// 抽屉全局常驻但按需初始化：首次展开时才请求枚举 / 状态 / 会话，
// 避免在未登录（如登录页）时就发出鉴权请求
let initialized = false;
const initialize = async (): Promise<void> => {
  void ensureEnum(EnumClass.llmSlot);
  // 工具调用阶段名与档位标签经统一枚举注册表渲染
  void ensureEnum(EnumClass.evalStage);
  // 审核阀门下拉与审批面板重要级徽章经统一枚举注册表渲染
  void ensureEnum(EnumClass.reviewValve);
  void ensureEnum(EnumClass.auditLevel);
  await Promise.all([loadStatus(), loadSessions()]);
  // 刷新后恢复上次会话：会话仍在则回填其档位 / 阀门并载入历史，供随后 watch(opened) 续播；
  // 会话已删除时 loadSessions 已把 activeSessionID 清空，此处自然跳过
  const restored = activeSessionID.value;
  if (restored !== null) {
    const target = sessions.value.find((s) => s.id === restored);
    if (target?.slot) {
      slot.value = target.slot;
    }
    if (target?.reviewValve) {
      reviewValve.value = target.reviewValve;
    }
    await loadMessages(restored);
    // 恢复该会话本地记忆的待发提问队列，供随后 watch(opened) 续播判定后投递
    ensureQueueLoaded(restored);
  }
};

// 当前会话跨刷新持久化：刷新后据该记录恢复会话并自动续播（见 initialize）
watch(activeSessionID, (value) => {
  localManager.setAgentActiveSession(value);
});

watch(opened, async (value) => {
  if (value) {
    if (!initialized) {
      initialized = true;
      // 首开需等初始化（会话列表 + 历史消息）就绪后再续播，否则续播会缺少可复用的占位消息
      await initialize();
    }
    // 重开抽屉：若当前会话有在途运行则续播（宽限期内重连即停表续跑）；
    // 续播判定完成后（busy 已如实反映在途运行）再投递队首待发提问——关抽屉期间完成的运行
    // 不会再触发流回调，仅在此补投递
    const id = activeSessionID.value;
    if (id !== null) {
      void resumeLiveRun(id).then(() => {
        ensureQueueLoaded(id);
        drainQueuedQuery(id);
      });
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
/* 抽屉内容容器撑满可用高度，供消息区独立滚动；同时作为左缘拖拽手柄的定位基准 */
.assistant-drawer {
  position: relative;
  min-height: 0;
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

.assistant-messages {
  /* 平滑滚动供提问轨的跳转使用（见 scrollToTurn） */
  scroll-behavior: smooth;
}

/* 消息区容器：只承载滚动正文（提问轨绝对定位在抽屉左缘，不占这里的流）。
   min-height: 0 必需——纵向 flex 子项默认 min-height: auto 会被内容高度撑破，
   滚动交给 .assistant-messages 自身承担 */
.assistant-conversation {
  min-height: 0;
}

/* 输入框：外观基线取自 .zk-panel + .zk-panel--subtle（带上灰度底以凸显输入区），
   这里只做聚焦反馈与内部布局 */
.assistant-composer {
  transition: border-color 0.2s;
}

.assistant-composer:focus-within {
  border-color: rgb(var(--v-theme-primary));
}

/* 问卷待作答时：输入区整体禁用（见模板中的 :disabled），仅弱化呈现、不隐藏/不成遮罩——
   用户仍能看到自己此前的输入，作答后即恢复可用 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.assistant-composer--blocked {
  opacity: 0.6;
}

/* 问卷待作答提示行：与队列区共用 8px 横向内边距，图标 + 提示文案 + 右端「查看问题」入口；
   置于输入框上方，把用户视线引回消息流中内联的问卷。纵向内边距与队列项取齐 */
.assistant-composer-pending {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
  padding: var(--zk-space-2) var(--zk-space-2) 0;
}

/* 正文输入区：起始高度下限交给 Vuetify 的 auto-grow（rows="2" 定起始行数，
   内容增多自动撑高），此处不得再写死 min-height——textarea 自身即
   .v-field__input，写死的 min-height 会以更高特异性压过 Vuetify 用于撑高的
   .v-field__input { min-height: max(...) }，把高度永久钉死、退化成内部滚动。
   仅补回 12px 左右对称内边距——plain 变体把取值区内边距归零，而工具条首个
   控件的内缩是「容器内边距 8px + 控件自身 4px」共 12px；两者取齐后正文与
   控件文字落在同一条竖线上
   （见 AGENTS.md「无背景 / 无边框的独立控件必须自带对称内边距」）。
   经 :deep(textarea) 下发可同时命中 auto-grow 用于测高的 .v-textarea__sizer，
   保证换行宽度与可视区一致 */
.assistant-composer :deep(.v-textarea textarea) {
  padding-inline: var(--zk-space-3);
}

/* 待发提问队列：堆叠在正文输入区上方，与正文同处一个边框容器内；层级靠间距与略强于面板的
   灰度底区分（抬升底色叠于 .zk-panel--subtle 的弱化底色之上），不在 div 之间加分隔线 */
.assistant-queue {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-1);
  padding: var(--zk-space-2) var(--zk-space-2) 0;
}

.assistant-queue-item {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
  padding: 2px var(--zk-space-1) 2px var(--zk-space-2);
  border-radius: var(--zk-radius-sm);
  background-color: rgb(var(--v-theme-on-surface), var(--zk-surface-raised-opacity));
}

/* 队列正文单行截断：超长追问不撑高堆叠区，完整内容可经「修改」回填到输入框查看 */
.assistant-queue-text {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-size: var(--zk-font-size-caption);
}

.assistant-queue-icon {
  flex: 0 0 auto;
  opacity: 0.55;
}

/* 容器底部工具条：控件自左依次排列，末位按钮（发送 / 停止）靠右，
   中间留白供后续审批等级、skill 按钮插入。
   纵向内边距必须上下对称：行内最低控件按自身高度撑满内容盒，若只留下边距，
   控件会贴住行顶、偏低不居中的观感（行中心与内容盒中心差 padding 的一半） */
.assistant-composer-toolbar {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-1) var(--zk-space-2);
}

/* 右侧动作组：优化提问与发送 / 停止成组贴右。推右由本组承担（而非 :last-child），
   使组的边界不随 v-if / v-else 切换的具体按钮变化，工具条其余控件留在左侧 */
.assistant-composer-actions {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  flex: 0 0 auto;
  margin-inline-start: auto;
}

/* 档位选择与审核阀门共用同一紧凑外观：宽度贴合内容，让取值与下拉箭头紧邻成一个控件。
   固定宽度会把 .v-field 的 1fr 空档撑开，把箭头推到最右，视觉上二者失去关联；
   Vuetify 的 .v-input 自身没有固有宽度（靠 flex 撑开），fit-content 让它按
   「取值名 + 箭头」取宽，同时也为同排后续控件（skill 引入按钮）留出空间。
   一并抹掉密度带来的 --v-input-padding-top（compact 下 8px）：本控件 hide-details，
   该内边距无处平衡，只把输入内容往下推 4px，使文本低于控件中心 */
.assistant-composer-slot,
.assistant-composer-valve {
  width: fit-content;

  --v-input-padding-top: 0px;
}

/* 档位下拉是本行唯一含长文本的成员，也是抽屉收窄时唯一能让出宽度的成员：
   允许收缩到内容以下（min-width: 0 解除 flex 项的自动最小尺寸），
   取值文本便由 Vuetify 自带的 .v-select__selection-text ellipsis 截断，
   完整取值经 title 悬浮回看（见 slotFullTitle）。
   内层 .v-field / .v-field__field 自带 min-width: auto，其 min-content 下限
   同样会阻止收缩，须一并归零，截断才真正落到文本上 */
.assistant-composer-slot {
  flex: 0 1 auto;
  min-width: 0;
}

/* stylelint-disable selector-class-pattern -- Vuetify 内部 BEM 元素名 */
.assistant-composer-slot :deep(.v-field),
.assistant-composer-slot :deep(.v-field__field) {
  min-width: 0;
}
/* stylelint-enable selector-class-pattern */

/* 审核阀门为纯图标控件，宽度即图标本身，始终按内容取宽、不参与收缩 */
.assistant-composer-valve {
  flex: 0 0 auto;
}

/* 思考开关：位于占用指示右侧、居左簇末位的紧凑开关，按内容取宽、不参与收缩。
   不展示字段名与语义图标（可访问名由 aria-label 提供，含义与说明由悬浮提示承担）。
   左侧补一档内边距：compact 档的开关控制盒（--v-selection-control-size）只有 28px，
   比 36px 宽的轨道还窄，关闭态的滑块位移量（Vuetify 编译期常量 10px）又把滑块推出
   控制盒左缘，字形溢出正好吃掉工具条 8px 的行间距（可见仅约 2px，看上去没有间隔）；
   补一档后开关与占用指示之间的可见间距回到本行其余控件的量级 */
.assistant-composer-thinking {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
  padding-inline-start: var(--zk-space-2);
}

/* 开关外框收至同排按钮的尺度：VSwitch 把 .v-selection-control 的最小高度钉在
   --v-input-control-height（compact 档 40px），比右侧 size="small" 的图标按钮（20px）
   高出一倍，开关因此成为本行最高的控件、把整行连同底边距一并撑高。
   这里只压外框高度：滑块容器（--v-selection-control-size，compact 档 28px）尺寸不动、
   在其中居中，开关自身的滑块与轨道位置不变，与两个按钮等高后整行由档位 / 阀门下拉定高 */
.assistant-composer-thinking :deep(.v-selection-control) {
  min-height: 0;
  height: 20px;
}

/* 上下文占用指示：与档位 / 阀门同排的非交互控件，小环与百分比文字紧邻成组。
   环与文字同色（弱化色 / 达阈值转警示色），由 contextUsageColor 统一驱动 */
.assistant-composer-usage {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
  flex: 0 0 auto;
}

.assistant-composer-usage-percent {
  font-size: var(--zk-font-size-caption);
  line-height: 1;
}

/* 取值文本、下拉箭头与控件左右边界应当读作一个控件。Vuetify 的 plain 变体把取值区左右
   内边距都归零（plain 面向「与周围文字齐平」的内联输入场景）；全站其余输入控件都在
   vuetify.ts 登记为 outlined、自带 16px 对称内边距，故工具条这两个下拉是全站仅有的例外。
   叠加 select 恒带 append-inner 插槽而命中的 .v-field--appended 6px 右内边距后，
   取值文本贴死左边界、与右箭头却隔 12px。这里在 .v-field 上补回 4px 对称内边距，
   并把 append 的 6px 归零，只留箭头自身的 4px 左边距，
   得到「4 ｜ 文本 ｜ 4 ｜ 箭头 ｜ 4」的等距节奏，内容不再贴死任一边界。
   自定义属性 --v-field-padding-end 声明在 .v-field 上、由后代 .v-field__input 消费；
   字号同样须落在 .v-field 上——它自带 font-size: 16px（与 .zk-section-title 同级），
   放在工具条里偏大，写 .v-input 会被 .v-field 自身的声明吞掉 */
.assistant-composer-slot :deep(.v-field),
.assistant-composer-valve :deep(.v-field) {
  font-size: var(--zk-font-size-caption);
  padding-inline: var(--zk-space-1);

  --v-field-padding-end: 0px;
}

/* 取值区自身的 2px 尾距（.v-select--dirty 为曾经的隐形 input 预留）会打破上面的等距节奏，
   一并归零；选择器带 .v-field 前缀以压过 Vuetify 的
   .v-select .v-field--dirty .v-select__selection */
/* stylelint-disable selector-class-pattern -- Vuetify 内部 BEM 元素名 */
.assistant-composer-slot :deep(.v-field .v-select__selection),
.assistant-composer-valve :deep(.v-field .v-select__selection) {
  margin-inline-end: 0;
}
/* stylelint-enable selector-class-pattern */

/* 下拉箭头与取值文本一起居于控件中心：Vuetify 对 plain 变体的 affix 容器写成
   align-items: flex-start（源序压过其自身的 center-affix 规则），箭头停在内容盒顶部，
   比文本高约 3px。仅在本工具条的单行下拉上居中——不能上提为全站规则，
   否则多行明文输入框的 affix 会被错误地居中 */
/* stylelint-disable selector-class-pattern -- Vuetify 内部 BEM 元素名 */
.assistant-composer-slot :deep(.v-field__append-inner),
.assistant-composer-valve :deep(.v-field__append-inner) {
  align-items: center;
}
/* stylelint-enable selector-class-pattern */

/* stylelint-disable selector-class-pattern -- BEM 修饰符：气泡外壳按角色分用户 / 助手 */

/* 用户消息气泡：宽度上限保留 85% 比例；圆角由 .zk-chat-bubble* 外壳类承载 */
.assistant-bubble--user {
  max-width: calc(100% * 0.85);
  padding: var(--zk-space-2) var(--zk-space-3);
}

/* 助手回答不包气泡：抽屉宽度本就有上限，而回答往往很长，气泡会把正文压成窄长条；
   改为占满消息区宽度，与下方常驻操作行一起读作一整段回答 */
.assistant-bubble--assistant {
  width: 100%;
}
/* stylelint-enable selector-class-pattern */

/* 单轮消息：气泡与其下方快捷操作纵向排列；用户轮靠右，助手轮靠左 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与 .zk-chat-bubble--user 同风格 */
.assistant-turn--user {
  align-items: flex-end;
}

/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与 .zk-chat-bubble--assistant 同风格 */
.assistant-turn--assistant {
  align-items: flex-start;
}

/* 用户消息快捷操作行：创建时间在左、操作图标在右，两组间留一档间距以读作同一行内的两个部分；
   默认淡出，悬停该轮（或键盘聚焦其中控件）时淡入。
   用 opacity 而非 display，避免悬停时因行高出现而抖动。
   整行取副标题灰：text 变体的 v-btn 为 color: inherit，由本行统一下发色值，
   与左侧 .zk-hint 时间同为一档灰 */
.assistant-message-actions {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  margin-top: var(--zk-space-1);
  color: rgb(var(--v-theme-on-surface-variant));
  opacity: 0;
  transition: opacity 0.15s;
}

/* 操作图标簇：三个图标紧密成组，与左侧时间保持一档以上的间隔 */
.assistant-message-buttons {
  display: flex;
  align-items: center;
  gap: var(--zk-space-1);
}

/* 助手回答的操作行常驻展示：不再依赖悬停触发（回答内容多，操作应始终可见可点） */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与 .zk-chat-bubble--user 同风格 */
.assistant-message-actions--persistent {
  opacity: 1;
}

.assistant-turn:hover .assistant-message-actions,
.assistant-turn:focus-within .assistant-message-actions {
  opacity: 1;
}

/* 用户消息正文保留换行与空格（纯文本） */
.assistant-plain {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.6;
}

/* 工具调用阶段列表 */
.assistant-stages {
  margin-top: var(--zk-space-1);
}

.assistant-stage {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-1) 0;
}

/* 一轮工具调用的展示组：组内各调用行自上而下排列；相邻组之间留一丝间距以区分轮次 */
.assistant-stage-group {
  display: flex;
  flex-direction: column;
}

.assistant-stage-group + .assistant-stage-group {
  margin-top: var(--zk-space-1);
}

/* 并行调用组：以左侧细线（边界包含关系）体现「同一轮内 N 次并发调用」，而非在行间加分隔边框 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.assistant-stage-group--parallel {
  padding-left: var(--zk-space-3);
  border-left: var(--zk-hairline-width) solid
    rgb(var(--v-theme-on-surface), var(--zk-hairline-opacity-strong));
}

/* 并行调用计数提示：仅多行时展示，与首行留出间距并弱化呈现 */
.assistant-stage-parallel-label {
  margin-bottom: var(--zk-space-1);
}

/* 内联交互块（问卷 / 人工审批）：按其触发轮次插入调用组序列中，与相邻调用组留出间距，
   使整条消息自上而下严格按时序阅读。块内沿用 zk-hairline-top 提供与上下内容的弱分隔 */
.assistant-interaction {
  margin: var(--zk-space-2) 0;
}

/* 上下文压缩展示块：外观基线取自 .zk-panel--subtle（弱化底 + 圆角），失败态补一条错误色描边。
   位于该轮快捷操作行下方（模板上的 mt-2 提供间距）。width: 100% 必需：它位于 .assistant-turn
   这一 align-items: flex-start 的纵向 flex 内，不撑满会被收缩成内容宽，running 态的摘要增量
   预览随之被压成窄条 */
.assistant-compress {
  width: 100%;
  padding: var(--zk-space-2);
}

/* 运行未正常结束提示：警示图标 + 文案横向排布，与正文之间靠间距与弱化底区分 */
.assistant-interrupted {
  display: flex;
  align-items: center;
  padding: var(--zk-space-2);
}

/* 思考过程折叠区块：外层描边盒由 .zk-panel 提供（展开面板族「描边盒 + 盒内平铺条目」约定），
   此处仅补消息流内的上下间距，并裁掉盒内面板溢出四角以贴合圆角 */
.assistant-reasoning {
  margin: var(--zk-space-2) 0;
  overflow: hidden;
}

/* 思考正文（折叠块展开区）：限高滚动，长思考不撑高消息区。高度上限取思考区专属令牌
   --zk-reasoning-max-height（全站长文本预览上限的 1/3：思考是流式增量长文本，独占一屏高度
   会把正文挤出视口）；约束落在正文自己的容器上（而非面板正文根元素），滚动条只随正文出现，
   面板内边距不参与滚动。流式增长时的粘底由 scrollReasoningToBottom 负责（见 onReasoning） */
.assistant-reasoning-body {
  max-height: var(--zk-reasoning-max-height);
  overflow-y: auto;
}

/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.assistant-compress--failed {
  border: var(--zk-hairline-width) solid rgb(var(--v-theme-error));
}

/* 压缩摘要增量：限高滚动，避免长摘要撑高气泡 */
.assistant-compress-draft {
  max-height: calc(var(--zk-editor-min-height) * 1.5);
  overflow: auto;
  margin-top: var(--zk-space-1);
}

/* 引用标题链接：命名空间可溯源时标题渲染为链接，颜色走主题 primary，默认无下划线，
   悬停显现下划线，与站内其它文本链接风格一致 */
.assistant-citation-link {
  color: rgb(var(--v-theme-primary));
  text-decoration: none;
}

.assistant-citation-link:hover {
  text-decoration: underline;
}

/* 引用正文两行截断，避免单条引用撑爆气泡 */
.assistant-citation-text {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 审批参数 / 结果 JSON 视区：限高滚动，避免长响应撑高气泡 */
.assistant-approval-block {
  max-height: calc(var(--zk-editor-min-height) * 1.5);
  overflow: auto;
  margin-top: var(--zk-space-1);
}

/* 会话列表面板：行内 hover / 选中底色铺满盒宽，裁掉四角以贴合面板圆角 */
.session-panel {
  overflow: hidden;
}

/* 会话行：整行可点击切换；右侧常驻重命名 / 删除按钮 */
.session-row {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-2) var(--zk-space-3);
  cursor: pointer;
}

/* 会话主体：标题 + 更新时间；占满行内剩余宽度并允许标题截断 */
.session-main {
  flex: 1 1 auto;
  min-width: 0;
}
</style>

<template>
  <main-layout current-nav="ai">
    <template #body>
      <!--
        知识范围分栏：当前命名空间（可写）与系统知识库（只读共享）各为一个 tab、各挂一张独立列表卡片。
        分栏形式与 /admin/llm-config 的 tab 栏一致；两张卡片各自持有筛选 / 列设置 / 勾选状态，
        切换分栏即切换卡片，无需在切换时清空勾选或做骨架化处理。
        只读分栏不挂新建 / 批量 / 页面动作三处入口，故其列表恒为只读。
      -->
      <tab-view
        v-model="scope"
        :items="[
          { key: 'current', label: $t('page.aiKnowledge.scopeCurrent') },
          { key: 'system', label: $t('page.aiKnowledge.scopeSystem') },
        ]"
      >
        <template #current>
          <search-table
            ref="searchTableRef"
            :list-request="knowledgeDocumentListGet"
            :options-request="knowledgeDocumentListOptions"
            :actions="getActions"
            :batch-actions="getBatchActions"
            :create="openCreateDialogAction"
            :toolbar-actions="toolbarActions"
            :project-fields="projectFields"
            model-name="aiDocument"
          >
            <!-- 状态列：按处理状态着色，进行中（parsing）附加载指示 -->
            <template #cell-status="{ item }">
              <v-chip :color="statusColor(item.status)">
                <v-progress-circular
                  v-if="statusSpinning(item.status)"
                  indeterminate
                  size="12"
                  width="2"
                  class="me-1"
                />
                {{ enumLabelOf(EnumClass.knowledgeDocStatus, item.status) }}
              </v-chip>
            </template>

            <!-- 原件大小列：字节数经本地 formatBytes 转为人类可读文本（手工录入无原件，展示为空） -->
            <template #cell-sizeBytes="{ item }">
              {{ formatBytes(item.sizeBytes) }}
            </template>
          </search-table>
        </template>

        <template #system>
          <!-- 只读说明：与注册表类页面（如 LLM 模型面板）的作用域提醒同构——常驻列表上方，
               避免用户误把系统知识库当作可编辑的作用域内数据 -->
          <div class="px-4 pt-4">
            <v-alert type="info">
              {{ $t("page.aiKnowledge.scopeSystemHint") }}
            </v-alert>
          </div>
          <search-table
            :list-request="knowledgeSystemDocumentListGet"
            :options-request="knowledgeSystemDocumentListOptions"
            :actions="getSystemActions"
            :toolbar-actions="[]"
            :project-fields="projectFields"
            model-name="aiDocument"
          >
            <template #cell-status="{ item }">
              <v-chip :color="statusColor(item.status)">
                <v-progress-circular
                  v-if="statusSpinning(item.status)"
                  indeterminate
                  size="12"
                  width="2"
                  class="me-1"
                />
                {{ enumLabelOf(EnumClass.knowledgeDocStatus, item.status) }}
              </v-chip>
            </template>

            <template #cell-sizeBytes="{ item }">
              {{ formatBytes(item.sizeBytes) }}
            </template>
          </search-table>
        </template>
      </tab-view>

      <!-- 删除确认对话框：单条删除与批量删除共用 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.aiDocument.name')"
        prepend-icon="mdi-book-open-variant"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            v-if="batchState"
            :action="$t('action.delete')"
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
          <confirm-alert
            v-else
            :action="$t('action.delete')"
            :text="
              $t('notice.deleteConfirm', {
                category: $t('model.aiDocument.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 重新索引 / 重新解析确认对话框：同一接口，按文档来源切换文案（手工文档=重新索引，导入文档=重新解析） -->
      <confirm-dialog
        ref="reindexConfirmRef"
        :title="$t(`action.${reindexActionKey}`)"
        prepend-icon="mdi-database-sync-outline"
        @confirm="itemReindexAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t(`action.${reindexActionKey}`)"
            :text="
              $t(`notice.${reindexActionKey}Confirm`, {
                category: $t('model.aiDocument.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 导入知识文档对话框：标题可选（留空则后端取文件名去扩展名）+ 选择原件；
           提交时 file 必须最后 append（后端契约）。提交中经 v-request 展示加载态 -->
      <temp-editable-dialog
        ref="uploadDialogRef"
        :title="$t('page.aiKnowledge.importDocument')"
        :confirm-disabled="!uploadFile"
        force-editable
        prepend-icon="mdi-file-import-outline"
        @confirm="uploadAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="uploadTitle"
                  :label="$t('model.aiDocument.field.title')"
                  :placeholder="$t('page.aiKnowledge.importTitlePlaceholder')"
                  hide-details="auto"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-file-input
                  v-model="uploadFile"
                  accept=".md,.markdown,.txt,.pdf,.docx"
                  :label="`${$t('page.aiKnowledge.importFile')} *`"
                  :hint="$t('page.aiKnowledge.importFileHint')"
                  persistent-hint
                  show-size
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 创建知识文档对话框 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.aiDocument.name')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-book-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createTitle"
                  v-bind="createTitleProps"
                  :label="$t('model.aiDocument.field.title')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createContent"
                  v-bind="createContentProps"
                  :label="$t('model.aiDocument.field.content')"
                  rows="10"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑知识文档对话框：手工文档标题与正文均可修改；导入文档正文只读（来自原件），仅标题可改 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.aiDocument.name')"
        :confirm-disabled="updateConfirmDisabled"
        prepend-icon="mdi-book-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <!-- 解析失败原因：仅失败文档展示，便于用户判断是否重新解析 -->
            <v-alert v-if="editParseError" type="error" density="compact" class="mb-2">
              {{ $t("page.aiKnowledge.parseError") }}: {{ editParseError }}
            </v-alert>

            <!-- 正文只读提示：仅导入文档展示（正文由解析流水线从原件生成，只能改标题）；
                 置于字段上方——先说明填写约束再进入表单，与注册表类页面的作用域提醒同构 -->
            <v-alert v-if="editDocumentImported" type="info" class="mb-4">
              {{ $t("page.aiKnowledge.importedContentReadOnly") }}
            </v-alert>

            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="editTitle"
                  v-bind="editTitleProps"
                  :label="$t('model.aiDocument.field.title')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="editContent"
                  v-bind="editContentProps"
                  :label="$t('model.aiDocument.field.content')"
                  :readonly="editDocumentImported"
                  :disabled="!editable"
                  rows="10"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 分块明细对话框：展示文档级索引摘要与逐块构建明细（读时派生，不改表） -->
      <knowledge-chunks-dialog
        v-model="chunksDialog"
        :documentID="chunksDocumentID"
        :document-title="chunksDocumentTitle"
        :chunks-request="chunksRequest"
      />

      <!-- 检索试运行对话框：输入查询与可选参数，运行完整检索链路（混合检索 → 生成回答 → 四项量化指标）并展示结果 -->
      <!-- 运行中 persistent：点背景 / ESC 不关闭，避免等待期间误触丢失进度（X / 关闭按钮仍可主动关闭）。
           关闭对话框既中止流也不清空状态，运行在后台继续，重开即可续看（状态挂页面级，见 openDryRunDialog） -->
      <app-dialog
        v-model="dryRunDialog"
        :title="$t('page.aiKnowledge.dryRun')"
        icon="mdi-flask-outline"
        size="md"
        :persistent="dryRunRunning"
      >
        <v-form @submit.prevent="runDryRunAction">
          <v-row density="compact">
            <v-col cols="12">
              <v-text-field
                v-model="dryRunQuery"
                :label="$t('page.aiKnowledge.query')"
                hide-details="auto"
              />
            </v-col>
          </v-row>
          <!-- 高级参数：默认折叠隐藏；未展开时展示后端默认值提示，展开后表单已预填默认值 -->
          <v-row density="compact" class="align-center">
            <v-col cols="auto">
              <v-btn
                variant="text"
                size="small"
                class="ms-n2"
                @click="showAdvancedParams = !showAdvancedParams"
              >
                <v-icon
                  size="small"
                  :icon="showAdvancedParams ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  class="me-1"
                />
                {{ $t("page.aiKnowledge.advancedParams") }}
              </v-btn>
            </v-col>
            <v-col v-if="!showAdvancedParams && dryRunDefaults" cols="auto">
              <span class="zk-hint">
                {{
                  $t("page.aiKnowledge.advancedParamsHint", {
                    topKLabel: $t("page.aiKnowledge.topK"),
                    candidatesLabel: $t("page.aiKnowledge.candidates"),
                    fusionLabel: $t("page.aiKnowledge.fusion"),
                    topK: dryRunDefaults.topK,
                    candidates: dryRunDefaults.candidates,
                    fusion: fusionTitleOf(dryRunDefaults.fusion),
                  })
                }}
              </span>
            </v-col>
          </v-row>
          <template v-if="showAdvancedParams">
            <div class="advanced-params zk-hairline-top">
              <v-row density="compact">
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model.number="dryRunTopK"
                    :label="$t('page.aiKnowledge.topK')"
                    type="number"
                    min="1"
                    hide-details="auto"
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model.number="dryRunCandidates"
                    :label="$t('page.aiKnowledge.candidates')"
                    type="number"
                    min="1"
                    hide-details="auto"
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-select
                    v-model="dryRunFusion"
                    :label="$t('page.aiKnowledge.fusion')"
                    :items="fusionOptions"
                    item-title="title"
                    item-value="value"
                    hide-details="auto"
                  />
                </v-col>
              </v-row>
              <v-row density="compact">
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model.number="dryRunWeightVector"
                    :label="$t('page.aiKnowledge.weightVector')"
                    type="number"
                    step="0.1"
                    min="0"
                    max="1"
                    hide-details="auto"
                  />
                </v-col>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model.number="dryRunWeightKeyword"
                    :label="$t('page.aiKnowledge.weightKeyword')"
                    type="number"
                    step="0.1"
                    min="0"
                    max="1"
                    hide-details="auto"
                  />
                </v-col>
              </v-row>
            </div>
          </template>
          <v-btn color="primary" :loading="dryRunRunning" type="submit" class="mt-2">
            {{ dryRunRunning ? $t("page.aiKnowledge.running") : $t("page.aiKnowledge.run") }}
          </v-btn>
          <v-btn
            v-if="dryRunRunning"
            variant="text"
            color="error"
            class="mt-2 ms-2"
            @click="stopDryRunAction"
          >
            {{ $t("page.aiKnowledge.stop") }}
          </v-btn>
        </v-form>

        <!-- 运行过程：阶段进度面板组（手风琴单开，同一时刻只展开当前查看的阶段并实时滚动其输出）；
             描边盒由外层 .zk-panel 提供，面板组自身裸放（见 AGENTS.md「展开面板族」） -->
        <div v-if="dryRunStages.length > 0" class="dry-run-process zk-panel">
          <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.process") }}</div>
          <v-expansion-panels v-model="expandedStage">
            <v-expansion-panel
              v-for="stage in dryRunStages"
              :key="stage.stage"
              :value="stage.stage"
            >
              <expansion-panel-title
                :icon="stageIconOf(stage)"
                :color="stageColorOf(stage)"
                :title="stageLabelOf(stage.stage)"
                :spin="dryRunRunning && stage.status === 'start'"
              >
                <!-- 阶段元信息（模型 / 耗时 / Token / 失败原因）不在标题行常驻：长文本会挤占阶段标题，
                     改为 info 图标按需 hover 展示（见 AGENTS.md「展开面板标题区」）；无内容时不渲染图标 -->
                <template #actions>
                  <v-tooltip
                    v-if="stageMetaLinesOf(stage).length > 0"
                    :max-width="STAGE_META_TOOLTIP_MAX_WIDTH"
                    location="top"
                  >
                    <template #activator="{ props: tooltipProps }">
                      <v-icon
                        v-bind="tooltipProps"
                        icon="mdi-information-outline"
                        size="x-small"
                        color="primary"
                        :aria-label="stageMetaLinesOf(stage).join(' · ')"
                      />
                    </template>
                    <div
                      v-for="(line, idx) in stageMetaLinesOf(stage)"
                      :key="idx"
                      class="stage-meta-line"
                    >
                      {{ line }}
                    </div>
                  </v-tooltip>
                </template>
              </expansion-panel-title>
              <v-expansion-panel-text>
                <pre
                  v-if="stage.output"
                  :ref="(el) => bindStageOutputEl(stage.stage, el)"
                  class="zk-code-preview stage-output font-mono text-body-small"
                  >{{ stage.output }}</pre>
                <p v-else class="zk-hint">{{ $t("page.aiKnowledge.stageNoOutput") }}</p>
              </v-expansion-panel-text>
            </v-expansion-panel>
          </v-expansion-panels>
          <!-- 汇总脚注：检索与总计耗时、全部调用合计 token（运行结束后由汇总结果回填） -->
          <div v-if="dryRunResult" class="metric-footnote zk-hint">
            {{ $t("page.aiKnowledge.metaRetrieveElapsed") }}:
            {{ dryRunResult.meta.retrieveElapsedMs }} ms · {{ $t("page.aiKnowledge.metaTotal") }}:
            {{ dryRunResult.meta.totalElapsedMs }} ms ·
            {{ $t("page.aiKnowledge.metaTotalTokens") }}:
            {{ formatTokenCount(dryRunResult.meta.usage.totalTokens) }}
          </div>
        </div>

        <template v-if="dryRunResult">
          <!-- 评估警告与评估不可用提示：置顶展示，先于回答与指标 -->
          <v-alert v-if="dryRunWarnings.length > 0" type="warning" class="mt-4">
            <div class="text-body-small on-surface-variant">
              <div>{{ $t("page.aiKnowledge.warnings") }}:</div>
              <div v-for="(warning, idx) in dryRunWarnings" :key="idx">
                {{ warning }}
              </div>
            </div>
          </v-alert>
          <p v-if="dryRunResult.evalError" class="text-body-small on-surface-variant mt-3">
            {{ $t("page.aiKnowledge.evalError") }}: {{ dryRunResult.evalError }}
          </p>

          <!-- 评估成功：回答引用块 + 量化指标卡片网格 + 明细折叠面板 -->
          <div v-if="dryRunResult.evaluation" class="dry-run-result zk-panel">
            <div class="result-block">
              <div class="zk-code-preview answer-block">
                <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.answer") }}</div>
                <p class="answer-body text-body-medium">
                  {{ dryRunResult.evaluation.answer || $t("model.__base.value.blank") }}
                </p>
              </div>
            </div>

            <div class="result-block">
              <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.metrics") }}</div>
              <div class="metric-grid">
                <div v-for="metric in dryRunMetrics" :key="metric.key" class="metric-card zk-panel">
                  <div class="zk-hint">{{ metric.label }}</div>
                  <div class="metric-value zk-section-title">{{ percentOf(metric.value) }}</div>
                  <v-progress-linear
                    :model-value="metric.value * 100"
                    height="6"
                    rounded
                    color="primary"
                  />
                </div>
              </div>
              <div class="metric-footnote zk-hint">
                {{
                  $t("page.aiKnowledge.relevantOfTotal", {
                    relevant: dryRunResult.evaluation.relevantCount,
                    total: dryRunResult.evaluation.totalChunks,
                  })
                }}
              </div>
            </div>

            <!-- 改进建议：按封闭归类给出可执行动作（知识覆盖 / 排序调参 / 生成质量 / 上下文利用） -->
            <div class="result-block">
              <div class="result-label zk-label-caps">{{ $t("page.aiKnowledge.suggestions") }}</div>
              <div class="metric-footnote zk-hint">
                {{ $t("page.aiKnowledge.suggestionsSubtitle") }}
              </div>
              <p v-if="dryRunSuggestions.length === 0" class="zk-hint">
                {{ $t("page.aiKnowledge.suggestionsEmpty") }}
              </p>
              <v-list v-else class="bg-transparent">
                <v-list-item v-for="(suggestion, idx) in dryRunSuggestions" :key="idx">
                  <template #prepend>
                    <v-icon :icon="suggestionIconOf(suggestion.category)" size="small" />
                  </template>
                  <v-list-item-title class="text-body-medium">
                    {{ suggestionCategoryLabelOf(suggestion.category) }}
                  </v-list-item-title>
                  <p class="text-body-medium mt-1">{{ suggestion.detail }}</p>
                </v-list-item>
              </v-list>
            </div>

            <div class="result-block">
              <v-expansion-panels v-model="resultPanel">
                <v-expansion-panel value="chunks">
                  <expansion-panel-title
                    icon="mdi-file-document-multiple-outline"
                    :title="$t('page.aiKnowledge.retrievedChunks')"
                    :subtitle="$t('page.aiKnowledge.retrievedChunksSubtitle')"
                  >
                    <v-chip size="x-small">{{ dryRunResult.results.length }}</v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item v-for="(result, idx) in dryRunResult.results" :key="idx">
                        <v-list-item-title class="text-body-medium">
                          {{ result.title }}
                        </v-list-item-title>
                        <v-list-item-subtitle class="text-body-small on-surface-variant">
                          #{{ result.chunkIndex }} · {{ result.documentID }} ·
                          {{ result.score.toFixed(2) }}
                        </v-list-item-subtitle>
                        <p class="text-body-medium mt-1">{{ result.content }}</p>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="judgments">
                  <expansion-panel-title
                    icon="mdi-check-decagram-outline"
                    :title="$t('page.aiKnowledge.judgments')"
                    :subtitle="$t('page.aiKnowledge.judgmentsSubtitle')"
                  >
                    <v-chip size="x-small">
                      {{ dryRunResult.evaluation.chunkJudgments.length }}
                    </v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item
                        v-for="(judgment, idx) in dryRunResult.evaluation.chunkJudgments"
                        :key="idx"
                      >
                        <template #prepend>
                          <v-icon
                            :icon="judgment.relevant ? 'mdi-check' : 'mdi-close'"
                            :color="judgment.relevant ? 'success' : 'error'"
                            size="small"
                          />
                        </template>
                        <v-list-item-title class="text-body-medium">
                          #{{ judgment.index }} · {{ judgment.reason }}
                        </v-list-item-title>
                        <template #append>
                          <span class="text-body-small on-surface-variant">
                            {{ judgment.score.toFixed(2) }}
                          </span>
                        </template>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="claims">
                  <expansion-panel-title
                    icon="mdi-format-quote-open"
                    :title="$t('page.aiKnowledge.claims')"
                    :subtitle="$t('page.aiKnowledge.claimsSubtitle')"
                  >
                    <v-chip size="x-small">
                      {{ dryRunResult.evaluation.claims.length }}
                    </v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item
                        v-for="(claim, idx) in dryRunResult.evaluation.claims"
                        :key="idx"
                      >
                        <template #prepend>
                          <v-icon
                            :icon="claim.supported ? 'mdi-check' : 'mdi-close'"
                            :color="claim.supported ? 'success' : 'error'"
                            size="small"
                          />
                        </template>
                        <v-list-item-title class="text-body-medium">
                          {{ claim.statement }}
                        </v-list-item-title>
                        <template #append>
                          <span class="text-body-small on-surface-variant">
                            #{{ claim.chunkIndexes.join(", #") }}
                          </span>
                        </template>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>

                <v-expansion-panel value="reverseQuestions">
                  <expansion-panel-title
                    icon="mdi-help-circle-outline"
                    :title="$t('page.aiKnowledge.reverseQuestions')"
                    :subtitle="$t('page.aiKnowledge.reverseQuestionsSubtitle')"
                  >
                    <v-chip size="x-small">
                      {{ dryRunResult.evaluation.reverseQuestions.length }}
                    </v-chip>
                  </expansion-panel-title>
                  <v-expansion-panel-text>
                    <v-list class="bg-transparent">
                      <v-list-item
                        v-for="(question, idx) in dryRunResult.evaluation.reverseQuestions"
                        :key="idx"
                      >
                        <v-list-item-title class="text-body-medium">
                          {{ question }}
                        </v-list-item-title>
                      </v-list-item>
                    </v-list>
                  </v-expansion-panel-text>
                </v-expansion-panel>
              </v-expansion-panels>
            </div>
          </div>

          <!-- 评估被跳过（如 chat 未启用）：仍展示检索分块 -->
          <div v-if="!dryRunResult.evaluation" class="mt-4">
            <div class="text-title-large font-weight-600">
              {{ $t("page.aiKnowledge.retrievedChunks") }}
            </div>
            <v-list class="bg-transparent">
              <v-list-item v-for="(result, idx) in dryRunResult.results" :key="idx">
                <v-list-item-title class="text-body-medium">
                  {{ result.title }}
                </v-list-item-title>
                <v-list-item-subtitle class="text-body-small on-surface-variant">
                  #{{ result.chunkIndex }} · {{ result.documentID }} ·
                  {{ result.score.toFixed(2) }}
                </v-list-item-subtitle>
                <p class="text-body-medium mt-1">{{ result.content }}</p>
              </v-list-item>
            </v-list>
          </div>
        </template>
        <template #footer>
          <v-btn text @click="dryRunDialog = false">
            {{ $t("action.close") }}
          </v-btn>
        </template>
      </app-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: AI Knowledge List
meta:
  title: title.aiKnowledge
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-knowledge-list">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  knowledgeDocumentChunksGet,
  knowledgeDocumentCreate,
  knowledgeDocumentDelete,
  knowledgeDocumentDetailGet,
  knowledgeDocumentDownload,
  knowledgeDocumentListGet,
  knowledgeDocumentListOptions,
  knowledgeDocumentReindex,
  knowledgeDocumentUpdate,
  knowledgeDocumentUpload,
  knowledgeRetrievalDryRunStream,
  knowledgeRetrievalOptions,
  knowledgeSystemDocumentChunksGet,
  knowledgeSystemDocumentListGet,
  knowledgeSystemDocumentListOptions,
} from "@/apis/ai";
import type {
  DiagnosisCategory,
  DiagnosisSuggestion,
  DryRunCallUsage,
  DryRunStageEvent,
  EvalStage,
  EvalStageStatus,
  KnowledgeDocumentDetail,
  KnowledgeDocumentReindexOut,
  KnowledgeDocumentUpdateIn,
  KnowledgeDocumentUploadOut,
  RetrievalDryRunIn,
  RetrievalDryRunOut,
  RetrievalOptionsOut,
} from "@/apis/@types/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import axios from "axios";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import type { ActionButton } from "@/components/base/ActionButtonGroup.vue";
import { useAppStore } from "@/stores/app";
import { knowledgeDocumentHref } from "@/utils/ai";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";
import { formatTokenCount } from "@/utils/token";

// ---- 表单校验 schema（正文为多行文本域，内容中的空白具有语义，不做 trim）----
const createDocSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiDocument.field.title"),
  content: yup.string().nonNullable().required().min(1).label("model.aiDocument.field.content"),
});

const updateDocSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiDocument.field.title"),
  // 正文可选：导入文档正文只读（提交时不携带 content），手工文档的「正文必填」在 updateConfirmDisabled 中校验
  content: yup.string().label("model.aiDocument.field.content"),
});

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const uploadDialogRef = useTemplateRef("uploadDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const reindexConfirmRef = useTemplateRef("reindexConfirmRef");

// ---- 知识范围分栏（当前命名空间可写 / 系统知识库只读共享）----
// scope 为当前激活的分栏键，取值与 tab-view 的 items 键一致；两个分栏各挂一张独立列表卡片，
// 各自的列表 / 选项请求与行内操作在模板中就位绑定，故此处不再需要随视图切换的 computed 转发
const scope = ref("current");

// ---- 表单 ----
const createForm = useForm({ validationSchema: createDocSchema });
const updateForm = useForm({ validationSchema: updateDocSchema });

const [createTitle, createTitleProps] = createForm.defineField("title", vuetifyCfg);
const [createContent, createContentProps] = createForm.defineField("content", vuetifyCfg);

const [editTitle, editTitleProps] = updateForm.defineField("title", vuetifyCfg);
const [editContent, editContentProps] = updateForm.defineField("content", vuetifyCfg);

// ---- 创建对话框 ----
const openCreateDialogAction = (): void => {
  // 复位用原子 API resetForm：完整枚举全部 defineField 字段，值 + 校验状态一次性重置
  createForm.resetForm({ values: { title: "", content: "" } });
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    knowledgeDocumentCreate({
      // 标题为单行文本，提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      title: (values.title as string).trim(),
      content: values.content,
    }),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
});

// ---- 导入文档（multipart 上传原件）----
const uploadFile = ref<File | null>(null);
const uploadTitle = ref("");

const openUploadDialogAction = (): void => {
  uploadFile.value = null;
  uploadTitle.value = "";
  uploadDialogRef.value?.open({});
};

// uploadAction 提交导入：title 可选（留空由后端取文件名去扩展名），file 必须最后 append。
// 返回 Promise 供 temp-editable-dialog 的 v-request 指令展示加载态。
const uploadAction = async (): Promise<void> => {
  const file = uploadFile.value;
  if (!file) {
    return;
  }
  const form = new FormData();
  const title = uploadTitle.value.trim();
  if (title) {
    form.append("title", title);
  }
  // 后端契约：file 必须为最后一个表单部分，故最后 append
  form.append("file", file);
  await getResponse<KnowledgeDocumentUploadOut>(
    knowledgeDocumentUpload(form),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// ---- 编辑对话框 ----
let editDocumentID = 0;
// 当前编辑文档是否为导入文档（详情 originalName 非空）；导入文档正文只读，仅标题可改
const editDocumentImported = ref(false);
// 当前编辑文档的解析失败原因（非 failed 为空），在编辑对话框内提示
const editParseError = ref("");

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<KnowledgeDocumentDetail>(
    knowledgeDocumentDetailGet(item.id),
    async (rsp) => {
      editDocumentID = rsp.data.id;
      // 以详情 originalName 判定导入文档：列表出参不含该字段，详情才是权威来源
      editDocumentImported.value = !!rsp.data.originalName;
      editParseError.value = rsp.data.parseError;
      // 复位与回填同属「准备本次打开的数据」：完整枚举 defineField 字段，原子重置值 + 校验状态
      updateForm.resetForm({
        values: {
          title: rsp.data.title,
          content: rsp.data.content,
        },
      });
      editDialogRef.value?.open(rsp.data);
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// updateConfirmDisabled 确认按钮禁用条件：标题校验未通过，或手工文档正文为空
// （导入文档正文只读，不参与必填校验）
const updateConfirmDisabled = computed(
  () => !updateForm.meta.value.valid || (!editDocumentImported.value && !editContent.value),
);

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    const payload: KnowledgeDocumentUpdateIn = {
      id: editDocumentID,
      title: (values.title as string).trim(),
    };
    // 导入文档正文只读：提交时不携带 content（后端对携带非空 content 的导入文档返回 400）
    if (!editDocumentImported.value) {
      payload.content = values.content;
    }
    await getSimpleResponse(
      knowledgeDocumentUpdate(editDocumentID, payload),
      async (rsp) => {
        await searchTableRef.value?.refresh();
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  })();
};

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效。
// format/status/sizeBytes 为状态徽标渲染与「导入文档判定（下载/重新解析入口）」所依赖，恒定投影；
// namespaceName 供系统视图行内跳转详情页携带来源命名空间，恒定投影
const projectFields = (visible: string[]): string[] => [
  "id",
  "title",
  "namespaceName",
  "format",
  "status",
  "sizeBytes",
  ...visible,
];

// ---- 处理状态展示映射（展示层）----
// color 为 chip 语义色；spinning 表示进行中（展示加载指示）；active 表示状态尚未收敛（触发有界轮询）。
// 取值字面量集中于此展示层映射，模板渲染、行操作可用性与轮询判定均由此派生
interface StatusDisplay {
  color?: string;
  spinning?: boolean;
  active?: boolean;
}
const STATUS_DISPLAY: Record<string, StatusDisplay> = {
  pending: { color: undefined, active: true },
  parsing: { color: "info", spinning: true, active: true },
  ready: { color: "success" },
  failed: { color: "error" },
};

// statusColor 状态徽标语义色（未登记状态回退默认色）
const statusColor = (status: string | undefined): string | undefined =>
  STATUS_DISPLAY[status ?? ""]?.color;

// statusSpinning 是否进行中（展示加载指示）
const statusSpinning = (status: string | undefined): boolean =>
  !!STATUS_DISPLAY[status ?? ""]?.spinning;

// statusActive 状态是否尚未收敛（pending/parsing），用于轮询启停与重新解析入口可用性
const statusActive = (status: string | undefined): boolean =>
  !!STATUS_DISPLAY[status ?? ""]?.active;

// isImportedDocument 列表行是否为导入文档（含 S3 原件）。列表出参不含 objectKey/originalName，
// 仅能据 sizeBytes 推断：手工录入文档固定 size_bytes=0，导入文档原件非空故 sizeBytes>0。
// 局限：导入的空文件（sizeBytes=0）会被误判为手工文档，此时不展示下载/重新解析入口。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const isImportedDocument = (item: Record<string, any>): boolean => Number(item.sizeBytes ?? 0) > 0;

// formatBytes 将字节数格式化为人类可读文本（1024 进制，保留一位小数）；无原件（0 字节）展示为空
const formatBytes = (value: unknown): string => {
  const bytes = Number(value ?? 0);
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  const text = unit === 0 ? String(size) : size.toFixed(1);
  return `${text} ${units[unit]}`;
};

// openDocumentDetail 打开本站知识文档详情页（新标签）：携带行数据里的来源命名空间，
// 详情页据此在与来源一致的接口间切换（自身命名空间 → 可写接口，其他命名空间 → 系统只读接口）。
// 两个分栏共用同一入口，故系统知识库与当前命名空间的「查看详情」行为完全一致
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openDocumentDetail = (item: Record<string, any>): void => {
  window.open(knowledgeDocumentHref(String(item.id), String(item.namespaceName ?? "")), "_blank");
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    detail: () => openDocumentDetail(item),
    edit: () => detailGetAction(item),
    chunks: () => openChunksDialog(item, knowledgeDocumentChunksGet),
  };
  // 导入文档「重新解析」（后端重新下载原件 → 重新解析 → 重建索引）：与手工文档的「重新索引」
  // 共用同一接口，仅按来源切换文案，避免两个入口造成歧义。
  // 进行中（pending/parsing）时后端会拒绝重复投递（400）；共享行操作菜单无禁用态，
  // 故此处以「省略即禁用」处理（与 datasource 启/禁用操作的条件包含范式一致）。
  if (isImportedDocument(item)) {
    if (!statusActive(item.status as string)) {
      actions.reparse = () => openReindexConfirm(item, "reparse");
    }
    actions.download = () => itemDownloadAction(item);
  } else {
    actions.reindex = () => openReindexConfirm(item, "reindex");
  }
  actions.delete = () => {
    batchState.value = null;
    deleteConfirmRef.value?.open(item);
  };
  return actions;
};

// getSystemActions 系统知识库视图的行内操作（只读）：仅查看详情与分块明细，不提供编辑/删除/重新索引
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getSystemActions = (item: Record<string, any>): ItemActions => ({
  // 查看详情：与当前命名空间共用同一入口（新标签打开本站知识文档详情页）
  detail: () => openDocumentDetail(item),
  // 分块明细：系统分栏须走只读系统接口（常规接口的数据范围不含系统命名空间）
  chunks: () => openChunksDialog(item, knowledgeSystemDocumentChunksGet),
});

// ---- 分块明细 ----
// 明细由后端读时派生（不改表），打开对话框时按当前文档 id 现取现算
const chunksDialog = ref(false);
const chunksDocumentID = ref(0);
const chunksDocumentTitle = ref("");
// 分块请求按来源下发：缺省为当前命名空间的常规接口，系统知识库分栏切到只读系统接口
const chunksRequest = ref(knowledgeDocumentChunksGet);

// openChunksDialog 打开分块明细对话框：两个分栏共用同一入口，避免各自维护
// documentID / 标题 / 请求三处状态；request 由调用方按来源传入
const openChunksDialog = (
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  item: Record<string, any>,
  request: typeof knowledgeDocumentChunksGet,
): void => {
  chunksDocumentID.value = item.id as number;
  chunksDocumentTitle.value = item.title as string;
  chunksRequest.value = request;
  chunksDialog.value = true;
};

// ---- 重新索引 / 重新解析 ----
// 当前确认对话框对应的操作键：手工文档为 reindex（重新索引），导入文档为 reparse（重新解析）
const reindexActionKey = ref<"reindex" | "reparse">("reindex");

// openReindexConfirm 打开确认对话框：按操作键切换标题与确认文案（同一接口，仅文案不同）
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openReindexConfirm = (item: Record<string, any>, key: "reindex" | "reparse"): void => {
  batchState.value = null;
  reindexActionKey.value = key;
  reindexConfirmRef.value?.open(item);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemReindexAction = async (proxy: Record<string, any>): Promise<void> => {
  await getResponse<KnowledgeDocumentReindexOut>(
    knowledgeDocumentReindex(proxy.id),
    async (rsp) => {
      // 刷新后由列表行状态驱动有界轮询（导入文档重新解析返回 pending，手工文档同步返回 ready）
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// ---- 下载原件 ----
// fetchDocumentDetail 取文档详情：列表出参不含 originalName/objectKey，详情是导入文档判定的
// 权威来源（originalName 非空即导入文档）。返回 null 表示请求失败（已提示错误）。
const fetchDocumentDetail = async (id: number): Promise<KnowledgeDocumentDetail | null> => {
  let result: KnowledgeDocumentDetail | null = null;
  await getResponse<KnowledgeDocumentDetail>(
    knowledgeDocumentDetailGet(id),
    async (rsp) => {
      result = rsp.data;
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
  return result;
};

// downloadErrorMessage 解析二进制下载失败的错误消息：blob 响应体为统一 JSON 信封
// （{code,summary}），逐层解析失败时回退到通用文案
const downloadErrorMessage = async (err: unknown): Promise<string> => {
  if (axios.isAxiosError(err) && err.response) {
    const data = err.response.data as unknown;
    if (data instanceof Blob) {
      try {
        const text = await data.text();
        const json = JSON.parse(text) as { summary?: string };
        if (json.summary) {
          return json.summary;
        }
      } catch {
        // 非 JSON 响应体：忽略并回退到通用文案
      }
    }
  }
  return i18n.global.t("notice.aiDocument.downloadFailed");
};

// saveBlobAsFile 触发浏览器保存：以对象 URL 生成临时链接触发下载（照 PlaintextDialog 先例）
const saveBlobAsFile = (blob: Blob, filename: string): void => {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
};

// itemDownloadAction 下载导入文档原件：先取详情拿到 originalName（列表无该字段），据其判定
// 是否为导入文档并作为保存文件名（避免解析 Content-Disposition 的 RFC 5987 编码）
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDownloadAction = async (item: Record<string, any>): Promise<void> => {
  const detail = await fetchDocumentDetail(item.id as number);
  if (!detail) {
    return;
  }
  if (!detail.originalName) {
    appStore.notify({
      text: i18n.global.t("notice.aiDocument.notDownloadable"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  try {
    const rsp = await knowledgeDocumentDownload(item.id as number);
    saveBlobAsFile(rsp.data as Blob, detail.originalName);
    appStore.notify({
      text: i18n.global.t("notice.aiDocument.downloadSuccess", { name: detail.originalName }),
      timeout: 3000,
      color: "success",
    });
  } catch (err) {
    appStore.notify({
      text: await downloadErrorMessage(err),
      timeout: 3000,
      color: "error",
    });
  }
};

// ---- 有界轮询 ----
// 列表存在 pending/parsing 行时按固定间隔刷新，使异步解析状态自动收敛，无需人工刷新。
// 停止条件：全部收敛（无进行中行）→ 立即停止并清零计数；达到次数上限 → 停止（后台任务长期
// 不收敛时避免无限刷新，上限 100 次 × 3s ≈ 5 分钟）；组件卸载 → 清理定时器（onUnmounted）。
const POLL_INTERVAL_MS = 3000;
const POLL_MAX_ATTEMPTS = 100;

let pollTimer: ReturnType<typeof setInterval> | null = null;
let pollAttempts = 0;

const stopPolling = (): void => {
  if (pollTimer !== null) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
  pollAttempts = 0;
};

// listHasActiveDocuments 列表当前是否存在进行中（pending/parsing）行
const listHasActiveDocuments = (): boolean =>
  (searchTableRef.value?.items ?? []).some((item) => statusActive(item.status as string));

// syncPolling 按列表状态启停轮询：存在进行中行时启动（已在轮询则保持不变），
// 全部收敛时停止
const syncPolling = (): void => {
  if (!listHasActiveDocuments()) {
    stopPolling();
    return;
  }
  if (pollTimer !== null) {
    return;
  }
  pollAttempts = 0;
  pollTimer = setInterval(async () => {
    if (pollAttempts >= POLL_MAX_ATTEMPTS) {
      stopPolling();
      return;
    }
    pollAttempts += 1;
    await searchTableRef.value?.refresh();
  }, POLL_INTERVAL_MS);
};

// 列表行引用每次刷新后变化，据此启停轮询；卸载时清理定时器
watch(
  () => searchTableRef.value?.items,
  () => syncPolling(),
);
onUnmounted(stopPolling);

// ---- 批量删除 ----
// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时以 error 提示失败数，否则沿用成功摘要
const applyBatchResult = (summary: string, result: BatchResultOut): void => {
  if (result.failed.length > 0) {
    appStore.notify({
      text: i18n.global.t("notice.batchResult", {
        succeeded: result.succeeded,
        failed: result.failed.length,
      }),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  appStore.notify({
    text: summary,
    timeout: 3000,
    color: "success",
  });
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type BatchState = { items: Record<string, any>[]; action: "delete" };
const batchState = ref<BatchState | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    delete: () => {
      batchState.value = { items, action: "delete" };
      deleteConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    knowledgeDocumentDelete(ids),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      searchTableRef.value?.clearSelection?.();
      await searchTableRef.value?.refresh();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDeleteAction = async (proxy: Record<string, any>): Promise<void> => {
  const state = batchState.value;
  if (state) {
    batchState.value = null;
    await batchDeleteAction(state.items);
    return;
  }
  await getResponse<BatchResultOut>(
    knowledgeDocumentDelete([proxy.id]),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      await searchTableRef.value?.refresh();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// ---- 检索试运行 ----
// 单个阶段的前端运行态：status 为后端上报的阶段状态，output 为该阶段模型原始输出增量累计
interface DryRunStageState {
  stage: EvalStage;
  // 该阶段调用的对话模型名（随阶段事件下发，运行中即有值；检索阶段为空）
  model: string;
  status: EvalStageStatus;
  elapsedMs: number;
  usage?: DryRunCallUsage;
  error?: string;
  output: string;
}

const dryRunDialog = ref(false);
const dryRunQuery = ref("");
const dryRunTopK = ref<number | null>(null);
const dryRunCandidates = ref<number | null>(null);
const dryRunFusion = ref<string | null>(null);
const dryRunWeightVector = ref<number | null>(null);
const dryRunWeightKeyword = ref<number | null>(null);
const dryRunRunning = ref(false);
const dryRunResult = ref<RetrievalDryRunOut | null>(null);
// 运行过程区：阶段进度列表与当前展开的阶段（v-expansion-panels 的 v-model）。
// 面板为手风琴式，同一时刻只展开一项，故为单值而非列表；运行中自动跟随最新上报的阶段，
// 空串表示全部收起
const dryRunStages = ref<DryRunStageState[]>([]);
const expandedStage = ref<string>("");
// 各阶段输出容器的 DOM 引用：面板内容仅在展开时渲染、收起后即卸载（Vuetify useLazy 行为），
// 同名模板 ref 在 v-for 中还会退化成数组，无法稳定定位，故按阶段登记到 Map，
// 由模板中的函数 ref 在元素挂载 / 卸载时写入与清除
const stageOutputEls = new Map<EvalStage, HTMLPreElement>();
const bindStageOutputEl = (stage: EvalStage, el: unknown): void => {
  if (el) {
    stageOutputEls.set(stage, el as HTMLPreElement);
  } else {
    stageOutputEls.delete(stage);
  }
};
// 高级参数是否展开（默认折叠隐藏，仅展示查询问题）
const showAdvancedParams = ref(false);
// 后端 [llm].rerank 配置的默认参数，用于折叠提示与展开表单预填
const dryRunDefaults = ref<RetrievalOptionsOut | null>(null);
// 结果明细折叠面板的展开状态（手风琴式，同一时刻仅一项；空串表示全部收起）。
// 与运行过程面板一致默认全部收起，由用户按需展开，避免长明细一上来就撑满对话框
const resultPanel = ref<string>("");
// 当前流式请求的中止函数（为空表示当前没有进行中的流）
const dryRunAbort = ref<(() => void) | null>(null);

// 融合策略选项（rrf / weighted）
const fusionOptions = [
  { title: "RRF", value: "rrf" },
  { title: "Weighted", value: "weighted" },
];

// fusionTitleOf 将融合策略值映射为下拉选项的展示名（如 rrf → RRF），未命中时原样返回
const fusionTitleOf = (value: string): string =>
  fusionOptions.find((option) => option.value === value)?.title ?? value;

// 四项量化指标（0-1 原始值，展示时经 percentOf 转为百分比）
const dryRunMetrics = computed(() => {
  const metrics = dryRunResult.value?.evaluation?.metrics;
  if (!metrics) {
    return [];
  }
  return [
    {
      key: "contextPrecision",
      label: i18n.global.t("page.aiKnowledge.contextPrecision"),
      value: metrics.contextPrecision,
    },
    {
      key: "faithfulness",
      label: i18n.global.t("page.aiKnowledge.faithfulness"),
      value: metrics.faithfulness,
    },
    {
      key: "answerRelevance",
      label: i18n.global.t("page.aiKnowledge.answerRelevance"),
      value: metrics.answerRelevance,
    },
    {
      key: "contextUtilization",
      label: i18n.global.t("page.aiKnowledge.contextUtilization"),
      value: metrics.contextUtilization,
    },
  ];
});

// percentOf 将 0-1 指标值转换为百分比文本（保留一位小数）
const percentOf = (value: number): string => `${(value * 100).toFixed(1)}%`;

// dryRunWarnings 评估警告列表（评估缺失时为空数组，避免模板中的可选链）
const dryRunWarnings = computed<string[]>(() => dryRunResult.value?.evaluation?.warnings ?? []);

// dryRunSuggestions 改进建议列表（评估缺失时为空数组，避免模板中的可选链）
const dryRunSuggestions = computed<DiagnosisSuggestion[]>(
  () => dryRunResult.value?.evaluation?.suggestions ?? [],
);

// 改进建议归类 → 图标：以图标区分「补文档」与「调参数」两类动作（归类取值由后端下发，未命中降级兜底）
const suggestionIcons: Record<string, string> = {
  coverage: "mdi-book-plus-outline",
  ranking: "mdi-sort-variant",
  generation: "mdi-text-box-edit-outline",
  context: "mdi-link-variant",
  other: "mdi-lightbulb-outline",
};

// suggestionIconOf 建议归类图标，未知归类降级为兜底图标
const suggestionIconOf = (category: DiagnosisCategory): string =>
  suggestionIcons[category] ?? suggestionIcons.other;

// suggestionCategoryLabelOf 建议归类展示名（经枚举注册表 diagnosisCategory 按当前语言渲染 label）
const suggestionCategoryLabelOf = (category: DiagnosisCategory): string =>
  enumLabelOf(EnumClass.diagnosisCategory, category);

// stageLabelOf 阶段展示名（经枚举注册表 evalStage 按当前语言渲染 label）
const stageLabelOf = (stage: EvalStage): string => enumLabelOf(EnumClass.evalStage, stage);

// stageIconOf 阶段状态图标：运行中旋转、完成打勾、失败告警；流已停止仍在进行的阶段展示为已中止
const stageIconOf = (stage: DryRunStageState): string => {
  if (stage.status === "done") {
    return "mdi-check-circle-outline";
  }
  if (stage.status === "failed") {
    return "mdi-alert-circle-outline";
  }
  return dryRunRunning.value ? "mdi-loading" : "mdi-stop-circle-outline";
};

// stageColorOf 阶段状态语义色
const stageColorOf = (stage: DryRunStageState): string => {
  if (stage.status === "done") {
    return "success";
  }
  if (stage.status === "failed") {
    return "warning";
  }
  return dryRunRunning.value ? "primary" : "warning";
};

// 当前展开阶段的模型原始输出（阶段未登记或全部收起时为空串）
const expandedStageOutput = computed<string>(
  () => dryRunStages.value.find((item) => item.stage === expandedStage.value)?.output ?? "",
);

// 输出更新后把展开中的输出容器滚动到底部，始终展示最新增量
watch(expandedStageOutput, async () => {
  await nextTick();
  const target = expandedStage.value ? stageOutputEls.get(expandedStage.value) : undefined;
  if (target) {
    target.scrollTop = target.scrollHeight;
  }
});

// applyStageEvent 应用一次阶段状态事件：首次出现时建行，后续仅更新状态字段
const applyStageEvent = (ev: DryRunStageEvent): void => {
  const target = dryRunStages.value.find((item) => item.stage === ev.stage);
  if (target) {
    target.status = ev.status ?? target.status;
    target.model = ev.model ?? target.model;
    target.elapsedMs = ev.elapsedMs ?? target.elapsedMs;
    target.usage = ev.usage ?? target.usage;
    target.error = ev.error ?? target.error;
  } else {
    dryRunStages.value.push({
      stage: ev.stage,
      model: ev.model ?? "",
      status: ev.status ?? "start",
      elapsedMs: ev.elapsedMs ?? 0,
      usage: ev.usage,
      error: ev.error,
      output: "",
    });
  }
  // 自动跟随最新阶段：展开到刚上报的阶段，运行中无需手动切换即可看到当前阶段输出
  expandedStage.value = ev.stage;
};

// appendStageDelta 累计某阶段的模型原始输出增量
const appendStageDelta = (stage: EvalStage, delta: string): void => {
  const target = dryRunStages.value.find((item) => item.stage === stage);
  if (target) {
    target.output += delta;
  }
};

// STAGE_META_TOOLTIP_MAX_WIDTH 阶段元信息 tooltip 的最大宽度（px）：超长失败原因在此宽度内换行，
// 避免 tooltip 被撑到过宽。经 max-width 属性下发（Vuetify 以行内样式落到浮层内容，CSS 类无法覆盖）
const STAGE_META_TOOLTIP_MAX_WIDTH = 360;

// stageMetaLinesOf 阶段元信息明细（逐行一项）：模型、耗时、输入/输出/合计 Token 与失败原因。
// 这里是调用信息的唯一落点（原「调用元信息」列表的字段均已并入）；由标题行的 info 图标以 tooltip
// 按需展示，不在标题行常驻，避免长文本挤占阶段标题。无内容时返回空数组（据此决定是否渲染图标）。
// 模型名随阶段事件下发（stage.model），运行中即可展示，无需等待汇总。
// 每项统一为「标签: 值」，与失败原因的写法保持一致，明细读起来段落清晰、不依赖标点猜测字段边界。
const stageMetaLinesOf = (stage: DryRunStageState): string[] => {
  const lines: string[] = [];
  if (stage.model) {
    lines.push(`${i18n.global.t("page.aiKnowledge.metaModel")}: ${stage.model}`);
  }
  if (stage.elapsedMs > 0) {
    lines.push(`${i18n.global.t("page.aiKnowledge.metaElapsed")}: ${stage.elapsedMs} ms`);
  }
  if (stage.usage) {
    lines.push(
      `${i18n.global.t("page.aiKnowledge.metaPromptTokens")}: ${formatTokenCount(stage.usage.promptTokens)}`,
      `${i18n.global.t("page.aiKnowledge.metaCompletionTokens")}: ${formatTokenCount(stage.usage.completionTokens)}`,
      `${i18n.global.t("page.aiKnowledge.metaTotalTokens")}: ${formatTokenCount(stage.usage.totalTokens)}`,
    );
  }
  if (stage.error) {
    lines.push(`${i18n.global.t("page.aiKnowledge.stageFailed")}: ${stage.error}`);
  }
  return lines;
};

// loadDryRunDefaults 拉取后端 [llm].rerank 配置的默认参数并预填表单：
// 仅填充尚未设置（null/空）的字段，避免覆盖用户修改
const loadDryRunDefaults = async (): Promise<void> => {
  await getResponse<RetrievalOptionsOut>(
    knowledgeRetrievalOptions(),
    async (rsp) => {
      dryRunDefaults.value = rsp.data;
      if (dryRunTopK.value === null) {
        dryRunTopK.value = rsp.data.topK;
      }
      if (dryRunCandidates.value === null) {
        dryRunCandidates.value = rsp.data.candidates;
      }
      if (!dryRunFusion.value) {
        dryRunFusion.value = rsp.data.fusion;
      }
      if (dryRunWeightVector.value === null) {
        dryRunWeightVector.value = rsp.data.weights.vector;
      }
      if (dryRunWeightKeyword.value === null) {
        dryRunWeightKeyword.value = rsp.data.weights.keyword;
      }
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// stopDryRunAction 中止当前流式请求（客户端断开后后端会停止后续大模型调用），
// 并把运行态复位
const stopDryRunAction = (): void => {
  dryRunAbort.value?.();
  dryRunAbort.value = null;
  dryRunRunning.value = false;
};

// 页面卸载时中止仍在后台运行的检索试运行流：试运行状态随组件销毁，继续运行没有意义，
// 且避免请求悬挂与对已卸载组件写状态（关闭对话框不中止，故需在此兜底）
onUnmounted(stopDryRunAction);

// openDryRunDialog 打开检索试运行对话框：状态挂在页面级，关闭对话框不再清空，
// 故重开沿用上次输入与结果（运行中则续看实时进度）；默认值仅在首次打开时拉取
const openDryRunDialog = async (): Promise<void> => {
  dryRunDialog.value = true;
  // 阶段名与建议归类经统一枚举注册表渲染 label，按当前语言预取（缓存到达后模板自动刷新）
  void ensureEnum(EnumClass.evalStage);
  void ensureEnum(EnumClass.diagnosisCategory);
  if (!dryRunDefaults.value) {
    await loadDryRunDefaults();
  }
};

const runDryRunAction = (): void => {
  // 查询为单行文本，提交前去除首尾空白；为空或已在运行则直接返回
  const query = dryRunQuery.value.trim();
  if (!query || dryRunRunning.value) {
    return;
  }
  const payload: RetrievalDryRunIn = { query };
  if (dryRunTopK.value !== null && dryRunTopK.value > 0) {
    payload.topK = dryRunTopK.value;
  }
  if (dryRunCandidates.value !== null && dryRunCandidates.value > 0) {
    payload.candidates = dryRunCandidates.value;
  }
  if (dryRunFusion.value) {
    payload.fusion = dryRunFusion.value;
  }
  // 权重两值任一非 0 才携带；Number 归一化空串/缺省为 0
  const vectorWeight = Number(dryRunWeightVector.value ?? 0);
  const keywordWeight = Number(dryRunWeightKeyword.value ?? 0);
  if (vectorWeight !== 0 || keywordWeight !== 0) {
    payload.weights = { vector: vectorWeight, keyword: keywordWeight };
  }
  // 新一轮运行清空上一轮的过程与结果
  dryRunResult.value = null;
  dryRunStages.value = [];
  expandedStage.value = "";
  stageOutputEls.clear();
  resultPanel.value = "";
  dryRunRunning.value = true;
  dryRunAbort.value = knowledgeRetrievalDryRunStream(payload, {
    onStage: applyStageEvent,
    onDelta: appendStageDelta,
    onDone: (out) => {
      dryRunResult.value = out;
      dryRunRunning.value = false;
      dryRunAbort.value = null;
      // 运行中自动展开最新阶段便于跟读输出，结束后收起：过程与结果都已定格，
      // 折叠成清单让用户按需回看（报错时不收起——此时结果区不存在，过程是唯一内容）
      expandedStage.value = "";
    },
    onError: (msg) => {
      dryRunRunning.value = false;
      dryRunAbort.value = null;
      appStore.notify({
        text: msg,
        timeout: 3000,
        color: "error",
      });
    },
  });
};

// 工具栏操作按钮：导入文档与检索试运行入口（alwaysShow 常驻，收起工具栏时也保持展示）

const toolbarActions = computed<ActionButton[]>(() => [
  {
    icon: "mdi-file-import-outline",
    title: i18n.global.t("page.aiKnowledge.importDocument"),
    // 与「新建」并列常驻，避免另起一行
    alwaysShow: true,
    onClick: openUploadDialogAction,
  },
  {
    // 运行中图标转 mdi-loading 并旋转，表达后台仍在进行（保持可点击）；不得用 VBtn 的 loading
    icon: dryRunRunning.value ? "mdi-loading" : "mdi-flask-outline",
    title: i18n.global.t("page.aiKnowledge.dryRun"),
    // 检索试运行对知识库管理是核心能力，收起工具栏时也保持展示
    alwaysShow: true,
    // 关闭对话框后运行仍在后台继续，据此可看出仍在进行并点击重开续看
    spin: dryRunRunning.value,
    onClick: openDryRunDialog,
  },
]);
</script>

<style scoped>
/* 高级参数区：与上方表单的分隔线走 .zk-hairline-top；容器内边距用于与折叠按钮之间留出间距 */
.advanced-params {
  margin-top: var(--zk-space-1);
  padding: var(--zk-space-2) 0 var(--zk-space-1);
}

/* 检索试运行结果区：分区盒外观（描边 + 圆角）走 .zk-panel */
.dry-run-result {
  margin-top: var(--zk-space-3);
  padding: var(--zk-space-3);
}

/* 检索试运行过程区：与结果区同一分区盒视觉语言（外观走 .zk-panel） */
.dry-run-process {
  margin-top: var(--zk-space-3);
  padding: var(--zk-space-3);
}

/* 阶段元信息 tooltip 内容：逐行左对齐展示（浮层内默认居中，长失败原因换行后需左对齐才成体例） */
.stage-meta-line {
  text-align: left;
  overflow-wrap: anywhere;
}

/* 阶段模型原始输出：等宽走 .font-mono，底色 / 换行 / 内边距走 .zk-code-preview，
   此处仅保留 margin 归零（pre 自带 1em）与本地滚动所需的限高与行距 */
.stage-output {
  margin: 0;
  max-height: calc(var(--zk-editor-min-height) * 1.25);
  overflow: auto;
  line-height: 1.5;
}

.result-block {
  margin-top: 8px;
}

.result-block:first-child {
  margin-top: 0;
}

.result-label {
  margin-bottom: var(--zk-space-1);
}

/* 生成回答：引用块底色 / 换行 / 内边距走 .zk-code-preview，无剩余本地声明 */

.answer-body {
  margin: 0;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: break-word;
}

/* 量化指标：自适应卡片网格 */
.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--zk-space-2);
}

.metric-card {
  padding: var(--zk-space-2) var(--zk-space-3);
}

.metric-value {
  margin: var(--zk-space-1) 0 var(--zk-space-1);
  line-height: 1.4;
}

.metric-footnote {
  margin-top: var(--zk-space-1);
}
</style>

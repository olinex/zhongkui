import i18n from "@/utils/i18n";

// knowledgeDocumentHref 构造知识文档详情页链接（新标签页打开）：携带来源命名空间，供详情页选择读取接口。
export function knowledgeDocumentHref(documentID: string, namespaceName: string): string {
  return `/${i18n.global.locale.value}/ai/document/${documentID}?namespace=${encodeURIComponent(namespaceName)}`;
}

// isSystemKnowledgeSource 判定来源是否系统命名空间：文档命名空间非空且不等于当前活动命名空间即为系统共享来源。
// 系统命名空间由后端常量定义，前端以命名空间比较判定来源，禁止硬编码 "system"。
export function isSystemKnowledgeSource(namespaceName: string, activeNamespace: string): boolean {
  return namespaceName !== "" && namespaceName !== activeNamespace;
}

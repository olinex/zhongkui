/**
 * .eslint.js
 *
 * ESLint 配置文件。
 */

import eslintConfigPrettier from "eslint-config-prettier";
import pluginVue from "eslint-plugin-vue";
import vueTsEslintConfig from "@vue/eslint-config-typescript";

export default [
  {
    name: "app/files-to-lint",
    files: ["**/*.{ts,mts,tsx,vue}"],
  },

  {
    name: "app/files-to-ignore",
    ignores: ["**/dist/**", "**/dist-ssr/**", "**/coverage/**"],
  },

  ...pluginVue.configs["flat/recommended"],
  ...vueTsEslintConfig(),

  {
    name: "app/rules",
    rules: {
      // 禁止显式 `any`；确有必要时使用精确的 eslint-disable 注释。
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/no-unused-expressions": [
        "error",
        {
          allowShortCircuit: true,
          allowTernary: true,
        },
      ],
      "vue/multi-word-component-names": "off",
      // 标识符统一以全大写 `ID` 结尾（如 documentID / recordID）。此类驼峰名经 kebab 化会
      // 被拆成 `document-i-d`（连续大写字母各成一词），可读性极差；故对此类 prop 名豁免
      // 连字符要求，模板内保持 `:documentID=` 形式（Vue 的 camelize 与连字符形式等价）。
      "vue/attribute-hyphenation": ["error", "always", { ignore: ["documentID", "recordID"] }],
    },
  },

  // 必须置于最后：关闭与 Prettier 冲突的格式化规则。
  eslintConfigPrettier,
];

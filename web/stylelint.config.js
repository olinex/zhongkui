/**
 * stylelint.config.js
 *
 * Stylelint configuration for CSS / SCSS / Vue SFC styles.
 */
export default {
  extends: ["stylelint-config-standard", "stylelint-config-recommended-vue"],
  rules: {
    /**
     * 禁止硬编码颜色（兜底防回归）。
     *
     * 背景：组件层颜色必须来自 Vuetify 主题变量（rgb(var(--v-theme-*))）或
     * src/styles/tokens.scss 的 --zk-* 令牌；直接写死颜色会让主题切换失效。
     *
     * 不使用已废弃的 color-no-hex（stylelint 17 已标记 deprecated，会产生告警），
     * 改用 declaration-property-value-disallowed-list，键 `/.+/` 表示匹配所有属性，
     * 值数组中的字符串以 / 包裹即按正则解释：
     *   1. hex 颜色（后须跟合法 hex 位数，避免误伤 SCSS 插值 `#{$var}`）
     *   2. `rgb(` / `rgba(` / `hsl(` / `hsla(` 紧跟数字的字面量颜色
     *      —— 该分支不匹配 `rgb(var(...))` / `rgb(var(--zk-*), 0.12)` 这类
     *      带 CSS 变量的合法写法（`(` 后是字母 v 而非数字）。
     * 两个分支都以「行首或空白/逗号/左括号」为起点，而非锚定 `^`：
     * 历史违规写法多为简写值中的颜色，如 `border: 1px solid rgb(0 0 0 / 24%)`，
     * 锚定行首会漏检。
     */
    "declaration-property-value-disallowed-list": {
      "/.+/": [
        "/(^|[\\s(,])#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\\b/",
        "/(^|[\\s(,])(rgba?|hsla?)\\(\\s*-?[0-9]/",
      ],
    },
  },
  overrides: [
    {
      files: ["**/*.scss"],
      extends: ["stylelint-config-standard-scss"],
    },
    {
      // 令牌层与外壳层豁免「禁止硬编码颜色」：
      // - src/styles/tokens.scss 是 --zk-* 数值/颜色的唯一来源，字面量按设计定义在此，
      //   其中 $zk-shadow-* 的 `rgb(0 0 0 / …)` 是阴影常量（与 Vuetify 自身做法一致），
      //   并非调色板颜色；
      // - src/styles/settings.scss 以字面量覆写 Vuetify 的 SASS 变量（圆角、字体栈），
      //   这些取值本就必须是字面量；
      // - src/styles/main.scss 已全面改用主题变量（html 的 background-color 现为
      //   rgb(var(--v-theme-background))，:root 携带当前主题变量，html 层可用），
      //   故本层豁免不再由该声明引起。
      // 仅豁免样式层自身的 CSS/SCSS 文件（令牌层与外壳层），
      // 不豁免该目录下的 `.vue`（视图组件的 <style> 仍须遵守主题变量约定）。
      files: ["src/styles/**/*.{css,scss}"],
      rules: {
        "declaration-property-value-disallowed-list": null,
      },
    },
  ],
};

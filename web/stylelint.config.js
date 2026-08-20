/**
 * stylelint.config.js
 *
 * Stylelint configuration for CSS / SCSS / Vue SFC styles.
 */
export default {
  extends: ["stylelint-config-standard", "stylelint-config-recommended-vue"],
  overrides: [
    {
      files: ["**/*.scss"],
      extends: ["stylelint-config-standard-scss"],
    },
  ],
};

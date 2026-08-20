/**
 * Ambient declaration for the "vuetify/styles" side-effect import.
 *
 * Vuetify's exports map only maps "./styles" to "lib/styles/main.css"
 * without a "types" condition, so strict side-effect import checks
 * (TS2882) cannot find type declarations for it.
 */
declare module "vuetify/styles";

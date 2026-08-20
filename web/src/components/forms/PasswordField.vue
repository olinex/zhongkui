<template>
  <v-text-field
    :model-value="props.modelValue"
    :label="props.label"
    :error-messages="props.errorMessages"
    :type="showPwd ? 'text' : 'password'"
    :append-inner-icon="showPwd ? 'mdi-eye' : 'mdi-eye-off'"
    @click:append-inner="showPwd = !showPwd"
    @input="emits('update:modelValue', $event.target.value)"
  />
</template>

<script setup lang="ts" name="password-field">
const showPwd = ref(false);

const props = defineProps({
  modelValue: { type: String, default: "" },
  label: { type: String, default: "Password" },
  errorMessages: { type: String, default: "" },
});

const emits = defineEmits(["update:modelValue"]);
</script>

<script setup>
import { levelNames } from '../format.js'
defineProps({ level: { type: String, required: true } })
</script>
<template><span :class="['level', level]"><i :class="['dot', level]"></i>{{ levelNames[level] }}</span></template>

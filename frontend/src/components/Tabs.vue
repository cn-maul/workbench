<script setup lang="ts" generic="T extends string">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { TabItem } from '../types'

const props = defineProps<{ modelValue: T; items: TabItem<T>[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: T] }>()

const list = ref<HTMLElement | null>(null)
const bar = ref({ x: 0, w: 0 })

// HeroUI 的选中背景是独立滑块，位置靠量取，切换时才有缓动位移
function sync() {
  const root = list.value
  const el = root?.querySelector<HTMLElement>('[aria-selected="true"]')
  if (!root || !el) return
  const rr = root.getBoundingClientRect()
  const er = el.getBoundingClientRect()
  bar.value = { x: er.left - rr.left, w: er.width }
}

let ro: ResizeObserver | null = null
onMounted(() => {
  sync()
  ro = new ResizeObserver(sync)
  if (list.value) ro.observe(list.value)
})
onBeforeUnmount(() => ro?.disconnect())
// 必须 post：默认 pre 时 aria-selected 还没改，量到的是上一个标签
watch(() => props.modelValue, sync, { flush: 'post' })
</script>

<template>
  <div ref="list" class="tabs" role="tablist">
    <span class="tab-bar" :style="{ transform: `translateX(${bar.x}px)`, width: bar.w + 'px' }" />
    <button v-for="it in items" :key="it.value" class="tab" role="tab"
      :class="{ active: it.value === modelValue }" :aria-selected="it.value === modelValue"
      @click="emit('update:modelValue', it.value)">{{ it.label }}</button>
  </div>
</template>

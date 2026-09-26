import { ref } from 'vue'

// 树刷新信号：数据发生可能影响项目列表的变化时 bump
export const treeVersion = ref(0)
export function bumpTree() { treeVersion.value++ }

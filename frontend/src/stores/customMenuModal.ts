import { defineStore } from 'pinia'
import { ref } from 'vue'
import { customMenuAPI } from '@/api'
import type { CustomMenuItem, CustomMenuModalContent } from '@/types'

export const useCustomMenuModalStore = defineStore('customMenuModal', () => {
  const isOpen = ref(false)
  const selectedItem = ref<CustomMenuItem | null>(null)
  const content = ref<CustomMenuModalContent | null>(null)
  const loading = ref(false)
  const error = ref(false)

  const cache = new Map<string, CustomMenuModalContent>()
  let requestVersion = 0

  async function load(id: string, force = false) {
    const version = ++requestVersion
    const cached = cache.get(id)
    if (cached && !force) {
      content.value = cached
      loading.value = false
      error.value = false
      return
    }

    loading.value = true
    error.value = false
    content.value = null
    try {
      const result = await customMenuAPI.getModalContent(id)
      cache.set(id, result)
      if (version === requestVersion && selectedItem.value?.id === id) {
        content.value = result
      }
    } catch {
      if (version === requestVersion && selectedItem.value?.id === id) {
        error.value = true
      }
    } finally {
      if (version === requestVersion) {
        loading.value = false
      }
    }
  }

  function open(item: CustomMenuItem) {
    selectedItem.value = item
    isOpen.value = true
    void load(item.id)
  }

  function retry() {
    if (selectedItem.value) {
      void load(selectedItem.value.id, true)
    }
  }

  function close() {
    requestVersion += 1
    isOpen.value = false
    selectedItem.value = null
    content.value = null
    loading.value = false
    error.value = false
  }

  function reset() {
    close()
    cache.clear()
  }

  return {
    isOpen,
    selectedItem,
    content,
    loading,
    error,
    open,
    retry,
    close,
    reset,
  }
})

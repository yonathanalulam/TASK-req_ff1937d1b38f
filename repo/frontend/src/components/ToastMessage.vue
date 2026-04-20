<template>
  <Teleport to="body">
    <div class="toast-container" aria-live="polite">
      <TransitionGroup name="toast">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          class="toast"
          :class="toast.type"
          role="alert"
          :data-testid="`toast-${toast.type}`"
        >
          <span class="toast-icon">{{ icons[toast.type] }}</span>
          <span class="toast-message">{{ toast.message }}</span>
          <button class="toast-close" @click="remove(toast.id)" aria-label="Dismiss">×</button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup>
import { ref } from 'vue'

const toasts = ref([])
let nextId = 0

const icons = {
  success: '✓',
  error: '✕',
  warning: '⚠',
  info: 'ℹ',
}

function add(message, type = 'info', duration = 4000) {
  const id = ++nextId
  toasts.value.push({ id, message, type })
  if (duration > 0) {
    setTimeout(() => remove(id), duration)
  }
}

function remove(id) {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

// Expose so parent can call useToast().show(...)
defineExpose({ add, remove })
</script>

<style scoped>
.toast-container {
  position: fixed;
  bottom: 1.5rem;
  right: 1.5rem;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: .6rem;
  max-width: 360px;
}

.toast {
  display: flex;
  align-items: center;
  gap: .6rem;
  padding: .75rem 1rem;
  border-radius: 8px;
  font-size: .9rem;
  box-shadow: 0 4px 16px rgba(0,0,0,.12);
  background: #fff;
  border-left: 4px solid #6366f1;
}

.toast.success { border-color: #22c55e; background: #f0fdf4; }
.toast.error   { border-color: #ef4444; background: #fef2f2; }
.toast.warning { border-color: #f59e0b; background: #fffbeb; }
.toast.info    { border-color: #6366f1; background: #eef2ff; }

.toast-icon { font-weight: 700; flex-shrink: 0; }
.toast-message { flex: 1; }
.toast-close {
  background: none;
  border: none;
  font-size: 1.1rem;
  cursor: pointer;
  color: #9ca3af;
  padding: 0 .2rem;
  flex-shrink: 0;
}
.toast-close:hover { color: #374151; }

.toast-enter-active,
.toast-leave-active { transition: all .25s ease; }
.toast-enter-from  { opacity: 0; transform: translateX(30px); }
.toast-leave-to    { opacity: 0; transform: translateX(30px); }
</style>

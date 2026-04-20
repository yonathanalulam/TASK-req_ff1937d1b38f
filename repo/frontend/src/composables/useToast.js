import { ref } from 'vue'

// Global toast ref — shared across the app
const toastRef = ref(null)

export function useToast() {
  function show(message, type = 'info', duration = 4000) {
    toastRef.value?.add(message, type, duration)
  }

  function success(message) { show(message, 'success') }
  function error(message)   { show(message, 'error') }
  function warning(message) { show(message, 'warning') }
  function info(message)    { show(message, 'info') }

  return { show, success, error, warning, info, toastRef }
}

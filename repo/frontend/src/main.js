import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import axios from 'axios'

// Global axios defaults
axios.defaults.baseURL = import.meta.env.VITE_API_BASE_URL || ''
axios.defaults.withCredentials = true

const app = createApp(App)

app.use(createPinia())
app.use(router)

app.mount('#app')

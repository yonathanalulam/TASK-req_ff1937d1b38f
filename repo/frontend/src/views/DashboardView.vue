<template>
  <div class="dashboard">
    <h1>Dashboard</h1>
    <p>Welcome, <strong>{{ auth.user?.display_name || auth.user?.username }}</strong>.</p>
    <p class="roles">
      Roles:
      <span v-for="role in auth.user?.roles" :key="role" class="role-badge">{{ role }}</span>
    </p>
  </div>
</template>

<script setup>
import { useAuthStore } from '@/stores/auth'
const auth = useAuthStore()
</script>

<style scoped>
.dashboard { max-width: 600px; }

h1 { margin-bottom: .5rem; }

.roles { margin-top: .5rem; display: flex; align-items: center; gap: .4rem; flex-wrap: wrap; }

.role-badge {
  background: #eef2ff;
  color: #4338ca;
  border-radius: 4px;
  padding: .1rem .5rem;
  font-size: .8rem;
  font-weight: 500;
}
</style>

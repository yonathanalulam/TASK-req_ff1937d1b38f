<template>
  <div class="offering-detail">
    <div v-if="loading" class="loading">Loading…</div>
    <div v-else-if="!offering" class="not-found">Offering not found.</div>
    <template v-else>
      <!-- Header row -->
      <div class="detail-header">
        <button class="btn-back" @click="$router.back()">← Back</button>
        <div class="header-actions">
          <button
            v-if="canEdit"
            class="btn-secondary"
            data-testid="btn-edit-offering"
            @click="$router.push(`/catalog/${offering.id}/edit`)"
          >
            Edit
          </button>
          <button
            v-if="isRegularUser && !isFavorited"
            class="btn-favorite"
            data-testid="btn-favorite"
            @click="addFav"
          >
            ♡ Save
          </button>
          <button
            v-if="isRegularUser && isFavorited"
            class="btn-unfavorite"
            data-testid="btn-unfavorite"
            @click="removeFav"
          >
            ♥ Saved
          </button>
        </div>
      </div>

      <!-- Details -->
      <h1 class="offering-title" data-testid="offering-title">{{ offering.name }}</h1>
      <div class="offering-meta">
        <span class="offering-price" data-testid="offering-price">
          ${{ offering.base_price.toFixed(2) }}
        </span>
        <span class="offering-duration">{{ offering.duration_minutes }} min</span>
        <span v-if="!offering.active_status" class="badge-inactive">Inactive</span>
      </div>
      <p v-if="offering.description" class="offering-description">
        {{ offering.description }}
      </p>

      <!-- Review summary + links -->
      <ReviewSummaryWidget :offering-id="offering.id" />
      <div class="offering-links">
        <router-link :to="`/catalog/${offering.id}/reviews`" class="link" data-testid="link-reviews">
          See all reviews
        </router-link>
        <router-link :to="`/catalog/${offering.id}/qa`" class="link" data-testid="link-qa">
          Questions &amp; Answers
        </router-link>
      </div>

      <!-- Shipping estimate widget -->
      <ShippingEstimateWidget />
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useCatalogStore } from '@/stores/catalog'
import { useAuthStore } from '@/stores/auth'
import { useProfileStore } from '@/stores/profile'
import ShippingEstimateWidget from '@/components/ShippingEstimateWidget.vue'
import ReviewSummaryWidget from '@/components/ReviewSummaryWidget.vue'

const route = useRoute()
const catalog = useCatalogStore()
const auth = useAuthStore()
const profileStore = useProfileStore()

const loading = ref(true)
const offering = ref(null)
const favoritedIds = ref(new Set())

const isRegularUser = computed(() => auth.user?.roles?.includes('regular_user') ?? false)

const canEdit = computed(() => {
  const roles = auth.user?.roles ?? []
  const userId = auth.user?.id
  if (roles.includes('administrator')) return true
  if (roles.includes('service_agent') && offering.value?.agent_id === userId) return true
  return false
})

const isFavorited = computed(() => favoritedIds.value.has(offering.value?.id))

onMounted(async () => {
  const id = Number(route.params.id)
  try {
    offering.value = await catalog.fetchOffering(id)
  } catch {
    offering.value = null
  }

  // Load favorites for regular users to determine saved state
  if (isRegularUser.value) {
    try {
      const page = await profileStore.fetchFavorites()
      favoritedIds.value = new Set(page.items.map((f) => f.offering_id))
    } catch {
      // non-critical
    }
  }

  loading.value = false
})

async function addFav() {
  await profileStore.addFavorite(offering.value.id)
  favoritedIds.value = new Set([...favoritedIds.value, offering.value.id])
}

async function removeFav() {
  await profileStore.removeFavorite(offering.value.id)
  const next = new Set(favoritedIds.value)
  next.delete(offering.value.id)
  favoritedIds.value = next
}
</script>

<style scoped>
.offering-detail { max-width: 720px; }
.loading, .not-found { color: #6b7280; text-align: center; padding: 3rem 0; }

.detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1rem;
}

.header-actions { display: flex; gap: .5rem; }

.btn-back {
  background: none;
  border: none;
  color: #4f46e5;
  font-size: .9rem;
  cursor: pointer;
  padding: 0;
}
.btn-back:hover { text-decoration: underline; }

.offering-title {
  font-size: 1.6rem;
  font-weight: 700;
  color: #111827;
  margin: 0 0 .5rem;
}

.offering-meta {
  display: flex;
  align-items: center;
  gap: .75rem;
  margin-bottom: 1rem;
  font-size: .95rem;
}

.offering-price { font-weight: 700; color: #059669; font-size: 1.1rem; }
.offering-duration { color: #6b7280; }

.badge-inactive {
  background: #fef3c7;
  color: #92400e;
  border-radius: 4px;
  padding: .1rem .4rem;
  font-size: .8rem;
}

.offering-description {
  color: #374151;
  line-height: 1.6;
  margin-bottom: 1rem;
}

.btn-secondary {
  background: #f3f4f6;
  color: #374151;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: .4rem .9rem;
  font-size: .85rem;
  cursor: pointer;
}
.btn-secondary:hover { background: #e5e7eb; }

.btn-favorite {
  background: #fff;
  color: #e11d48;
  border: 1px solid #fca5a5;
  border-radius: 8px;
  padding: .4rem .9rem;
  font-size: .85rem;
  cursor: pointer;
}
.btn-favorite:hover { background: #fff1f2; }

.btn-unfavorite {
  background: #fff1f2;
  color: #be123c;
  border: 1px solid #fca5a5;
  border-radius: 8px;
  padding: .4rem .9rem;
  font-size: .85rem;
  cursor: pointer;
}
.btn-unfavorite:hover { background: #ffe4e6; }

.offering-links {
  display: flex;
  gap: 1.25rem;
  margin: .75rem 0 1rem;
  font-size: .9rem;
}
.link { color: #4f46e5; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>

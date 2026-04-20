<template>
  <div class="catalog-view">
    <div class="catalog-header">
      <h1>Service Catalog</h1>
      <button
        v-if="canCreateOffering"
        class="btn-primary"
        data-testid="btn-new-offering"
        @click="$router.push('/catalog/new')"
      >
        + New Offering
      </button>
    </div>

    <!-- Filters -->
    <div class="filters">
      <label for="filter-category">Category</label>
      <select
        id="filter-category"
        v-model="selectedCategory"
        data-testid="select-category"
        @change="applyFilter"
      >
        <option value="0">All Categories</option>
        <option v-for="cat in catalog.categories" :key="cat.id" :value="cat.id">
          {{ cat.name }}
        </option>
      </select>

      <label for="filter-active">Status</label>
      <select id="filter-active" v-model="selectedActive" @change="applyFilter">
        <option value="-1">All</option>
        <option value="1">Active</option>
        <option value="0">Inactive</option>
      </select>
    </div>

    <!-- Offering grid -->
    <div v-if="loading" class="loading">Loading offerings…</div>
    <div v-else-if="catalog.offerings.length === 0" class="empty" data-testid="empty-catalog">
      No offerings found.
    </div>
    <div v-else class="offerings-grid" data-testid="list-offerings">
      <div
        v-for="offering in catalog.offerings"
        :key="offering.id"
        class="offering-card"
        data-testid="card-offering"
        @click="$router.push(`/catalog/${offering.id}`)"
      >
        <div class="card-body">
          <h3 class="offering-name" data-testid="offering-name">{{ offering.name }}</h3>
          <p class="offering-desc">{{ offering.description }}</p>
          <div class="card-meta">
            <span class="offering-price" data-testid="offering-price">
              ${{ offering.base_price.toFixed(2) }}
            </span>
            <span class="offering-duration">{{ offering.duration_minutes }} min</span>
            <span v-if="!offering.active_status" class="badge-inactive">Inactive</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Load more -->
    <div v-if="catalog.nextCursor > 0" class="load-more">
      <button class="btn-secondary" data-testid="btn-load-more" @click="loadMore">
        Load More
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useCatalogStore } from '@/stores/catalog'
import { useAuthStore } from '@/stores/auth'

const catalog = useCatalogStore()
const auth = useAuthStore()

const loading = ref(false)
const selectedCategory = ref(0)
const selectedActive = ref(-1)

const canCreateOffering = computed(() => {
  const roles = auth.user?.roles ?? []
  return roles.includes('service_agent') || roles.includes('administrator')
})

onMounted(async () => {
  loading.value = true
  await Promise.all([catalog.fetchCategories(), catalog.fetchOfferings()])
  loading.value = false
})

async function applyFilter() {
  loading.value = true
  await catalog.fetchOfferings({
    categoryId: Number(selectedCategory.value),
    active: Number(selectedActive.value),
    cursor: 0,
  })
  loading.value = false
}

async function loadMore() {
  await catalog.fetchOfferings({
    categoryId: Number(selectedCategory.value),
    active: Number(selectedActive.value),
    cursor: catalog.nextCursor,
  })
}
</script>

<style scoped>
.catalog-view { max-width: 960px; }

.catalog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.25rem;
}

.filters {
  display: flex;
  align-items: center;
  gap: .75rem;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}

.filters label { font-size: .85rem; font-weight: 600; color: #374151; }

.filters select {
  padding: .35rem .6rem;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: .9rem;
  background: #fff;
}

.loading, .empty { color: #6b7280; text-align: center; padding: 3rem 0; }

.offerings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 1rem;
}

.offering-card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  cursor: pointer;
  transition: box-shadow .15s, transform .1s;
}

.offering-card:hover {
  box-shadow: 0 4px 12px rgba(0,0,0,.1);
  transform: translateY(-2px);
}

.card-body { padding: 1.1rem; }

.offering-name {
  font-size: 1rem;
  font-weight: 600;
  color: #111827;
  margin: 0 0 .4rem;
}

.offering-desc {
  font-size: .85rem;
  color: #6b7280;
  margin: 0 0 .75rem;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.card-meta {
  display: flex;
  align-items: center;
  gap: .5rem;
  font-size: .85rem;
}

.offering-price { font-weight: 700; color: #059669; }
.offering-duration { color: #9ca3af; }

.badge-inactive {
  background: #fef3c7;
  color: #92400e;
  border-radius: 4px;
  padding: .1rem .4rem;
  font-size: .75rem;
}

.load-more { text-align: center; margin-top: 1.5rem; }

.btn-primary {
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: .5rem 1.1rem;
  font-size: .9rem;
  cursor: pointer;
  transition: background .15s;
}
.btn-primary:hover { background: #4338ca; }

.btn-secondary {
  background: #f3f4f6;
  color: #374151;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  padding: .45rem 1rem;
  font-size: .9rem;
  cursor: pointer;
}
.btn-secondary:hover { background: #e5e7eb; }
</style>

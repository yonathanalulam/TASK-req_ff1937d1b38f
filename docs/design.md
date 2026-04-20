# Frontend Design Document

## Architecture Overview

The Eagle Point Service Portal frontend is a modern Vue.js 3 application built with the Composition API, featuring a component-based architecture with comprehensive state management and responsive design patterns.

## Technology Stack

### Frontend Core
- **Framework**: Vue 3 with Composition API
- **State Management**: Pinia for reactive state
- **Routing**: Vue Router 4 with lazy loading
- **HTTP Client**: Axios with cookie support and interceptors
- **Build Tool**: Vite for fast development and optimized builds
- **Testing**: Playwright for E2E tests, Vitest for unit tests

### UI & Styling
- **CSS Framework**: Custom design system with CSS modules
- **Design Tokens**: CSS custom properties for consistency
- **Responsive Design**: Mobile-first approach with CSS Grid/Flexbox
- **Component Library**: Reusable atomic design components

### Development Tools
- **Package Manager**: npm or pnpm
- **Linting**: ESLint with Vue 3 rules
- **Formatting**: Prettier for consistent code style
- **Type Checking**: TypeScript support (optional)

## Frontend Architecture

### System Architecture
```
Vue.js Application
    |
    | Component Tree
    |
Pinia Stores (State Management)
    |
    | HTTP Requests
    |
Backend API (Go/Gin)
    |
    | Session/Cookie Auth
    |
Database (MySQL)
```

### Project Structure
```
frontend/
src/
  components/        # Reusable Vue components
    common/         # Generic components (Button, Input, Modal)
    forms/          # Form-specific components
    layout/         # Layout components (Header, Sidebar, Footer)
    ui/             # UI-only components (Loading, Toast, Badge)
  composables/       # Vue composition functions
    useApi.js       # API integration logic
    useAuth.js      # Authentication helpers
    useForm.js      # Form handling utilities
  router/           # Route definitions
    index.js        # Main router configuration
    guards.js       # Route guards
  stores/           # Pinia state stores
    auth.js         # Authentication state
    profile.js      # User profile state
    catalog.js      # Service catalog state
    tickets.js      # Ticket management state
    notifications.js # Notification system
    ui.js           # UI state (loading, modals)
  views/            # Page-level components
    auth/           # Authentication pages
    dashboard/      # Main dashboard
    tickets/        # Ticket management
    profile/        # User profile
    admin/          # Admin panel
  assets/           # Static assets
    styles/         # Global styles and design tokens
    images/         # Image assets
    icons/          # Icon components
  utils/            # Utility functions
    api.js          # API configuration
    constants.js    # Application constants
    helpers.js      # Helper functions
  App.vue           # Root component
  main.js           # Application entry point
```

## Component Design Patterns

### 1. Atomic Design Methodology
```
Atoms (Basic Elements)
  - Button, Input, Icon, Badge
  - Typography, Colors, Spacing
  
Molecules (Simple Combinations)
  - FormField, Card, SearchBox
  - Dropdown, Toggle, Checkbox
  
Organisms (Complex Components)
  - Header, Sidebar, DataTable
  - Form, Modal, Navigation
  
Templates (Layout Structure)
  - PageLayout, AuthLayout
  - DashboardLayout, AdminLayout
  
Pages (Complete Views)
  - LoginPage, DashboardView
  - TicketDetailView, ProfileView
```

### 2. Component Composition Patterns

#### Props Down, Events Up Pattern
```vue
<!-- Parent Component -->
<template>
  <ChildComponent 
    :data="parentData"
    @update="handleChildUpdate"
  />
</template>

<!-- Child Component -->
<script setup>
const props = defineProps(['data'])
const emit = defineEmits(['update'])
</script>
```

#### Provide/Inject for Deep Prop Drilling
```vue
<!-- Root Component -->
<script setup>
import { provide } from 'vue'

provide('theme', 'dark')
provide('user', userStore.currentUser)
</script>

<!-- Deep Child Component -->
<script setup>
import { inject } from 'vue'

const theme = inject('theme')
const user = inject('user')
</script>
```

#### Scoped Slots for Flexible Composition
```vue
<!-- Flexible Table Component -->
<template>
  <table class="table">
    <thead>
      <tr>
        <th v-for="column in columns" :key="column.key">
          {{ column.label }}
        </th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="item in items" :key="item.id">
        <slot name="row" :item="item" :columns="columns">
          <td v-for="column in columns" :key="column.key">
            {{ item[column.key] }}
          </td>
        </slot>
      </tr>
    </tbody>
  </table>
</template>
```

### 3. Reusable Component Examples

#### Button Component with Variants
```vue
<!-- components/common/Button/Button.vue -->
<template>
  <button 
    :class="buttonClasses"
    :disabled="disabled || loading"
    @click="handleClick"
  >
    <LoadingSpinner v-if="loading" class="mr-2" />
    <Icon v-if="icon && !loading" :name="icon" class="mr-2" />
    <slot />
  </button>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  variant: {
    type: String,
    default: 'primary',
    validator: (value) => ['primary', 'secondary', 'outline', 'ghost'].includes(value)
  },
  size: {
    type: String,
    default: 'md',
    validator: (value) => ['sm', 'md', 'lg'].includes(value)
  },
  disabled: Boolean,
  loading: Boolean,
  icon: String
})

const emit = defineEmits(['click'])

const buttonClasses = computed(() => [
  'btn',
  `btn--${props.variant}`,
  `btn--${props.size}`,
  {
    'btn--disabled': props.disabled,
    'btn--loading': props.loading
  }
])

const handleClick = (event) => {
  if (!props.disabled && !props.loading) {
    emit('click', event)
  }
}
</script>
```

#### Form Field Component
```vue
<!-- components/forms/FormField/FormField.vue -->
<template>
  <div class="form-field">
    <label 
      :for="fieldId" 
      class="form-field__label"
      :class="{ 'form-field__label--required': required }"
    >
      {{ label }}
    </label>
    <div class="form-field__input-wrapper">
      <slot name="input" :fieldId="fieldId" :value="modelValue" />
      <span v-if="error" class="form-field__error">
        {{ error }}
      </span>
    </div>
    <span v-if="hint" class="form-field__hint">
      {{ hint }}
    </span>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  label: String,
  modelValue: [String, Number],
  error: String,
  hint: String,
  required: Boolean
})

const fieldId = computed(() => `field-${Math.random().toString(36).substr(2, 9)}`)
</script>
```

## State Management Architecture

### 1. Pinia Store Structure

#### Authentication Store
```javascript
// stores/auth.js
import { defineStore } from 'pinia'
import { authApi } from '@/utils/api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null,
    token: null,
    isAuthenticated: false,
    loading: false,
    error: null
  }),

  getters: {
    isAdmin: (state) => state.user?.role === 'admin',
    isModerator: (state) => state.user?.role === 'moderator',
    userDisplayName: (state) => state.user?.displayName || state.user?.username,
    userPermissions: (state) => state.user?.permissions || []
  },

  actions: {
    async login(credentials) {
      this.loading = true
      this.error = null
      
      try {
        const response = await authApi.login(credentials)
        this.user = response.data.user
        this.token = response.data.token
        this.isAuthenticated = true
        
        // Store token in localStorage
        localStorage.setItem('auth_token', this.token)
      } catch (error) {
        this.error = error.response?.data?.message || 'Login failed'
        throw error
      } finally {
        this.loading = false
      }
    },

    async logout() {
      try {
        await authApi.logout()
      } catch (error) {
        console.error('Logout error:', error)
      } finally {
        this.user = null
        this.token = null
        this.isAuthenticated = false
        localStorage.removeItem('auth_token')
      }
    },

    async refreshUser() {
      if (!this.token) return
      
      try {
        const response = await authApi.me()
        this.user = response.data
      } catch (error) {
        this.logout()
      }
    },

    initializeAuth() {
      const token = localStorage.getItem('auth_token')
      if (token) {
        this.token = token
        this.isAuthenticated = true
        this.refreshUser()
      }
    }
  }
})
```

#### UI Store for Global State
```javascript
// stores/ui.js
import { defineStore } from 'pinia'

export const useUIStore = defineStore('ui', {
  state: () => ({
    loading: false,
    sidebarOpen: false,
    theme: 'light',
    toasts: [],
    modals: {},
    notifications: []
  }),

  actions: {
    setLoading(loading) {
      this.loading = loading
    },

    toggleSidebar() {
      this.sidebarOpen = !this.sidebarOpen
    },

    setTheme(theme) {
      this.theme = theme
      document.documentElement.setAttribute('data-theme', theme)
      localStorage.setItem('theme', theme)
    },

    addToast(toast) {
      const id = Date.now()
      this.toasts.push({ ...toast, id })
      
      // Auto-remove after 5 seconds
      setTimeout(() => {
        this.removeToast(id)
      }, 5000)
    },

    removeToast(id) {
      this.toasts = this.toasts.filter(toast => toast.id !== id)
    },

    openModal(modalId, data = {}) {
      this.modals[modalId] = { open: true, data }
    },

    closeModal(modalId) {
      this.modals[modalId] = { open: false, data: null }
    }
  }
})
```

### 2. Reactive State Patterns

#### Computed Properties for Derived State
```javascript
// stores/tickets.js
export const useTicketStore = defineStore('tickets', {
  state: () => ({
    tickets: [],
    filters: {
      status: 'all',
      priority: 'all',
      assignedTo: 'all'
    },
    search: ''
  }),

  getters: {
    filteredTickets: (state) => {
      return state.tickets.filter(ticket => {
        const matchesStatus = state.filters.status === 'all' || ticket.status === state.filters.status
        const matchesPriority = state.filters.priority === 'all' || ticket.priority === state.filters.priority
        const matchesAssigned = state.filters.assignedTo === 'all' || ticket.assignedTo === state.filters.assignedTo
        const matchesSearch = !state.search || 
          ticket.title.toLowerCase().includes(state.search.toLowerCase()) ||
          ticket.description.toLowerCase().includes(state.search.toLowerCase())
        
        return matchesStatus && matchesPriority && matchesAssigned && matchesSearch
      })
    },

    ticketStats: (state) => {
      return {
        total: state.tickets.length,
        open: state.tickets.filter(t => t.status === 'open').length,
        inProgress: state.tickets.filter(t => t.status === 'in_progress').length,
        closed: state.tickets.filter(t => t.status === 'closed').length
      }
    }
  }
})
```

#### Watchers for Side Effects
```javascript
// composables/useLocalStorage.js
import { ref, watch } from 'vue'

export function useLocalStorage(key, defaultValue) {
  const storedValue = localStorage.getItem(key)
  const value = ref(storedValue ? JSON.parse(storedValue) : defaultValue)

  watch(value, (newValue) => {
    localStorage.setItem(key, JSON.stringify(newValue))
  }, { deep: true })

  return value
}
```

## Frontend Architecture

### Project Structure
```
frontend/
src/
  components/        # Reusable Vue components
    common/         # Generic components (Button, Input, Modal)
    forms/          # Form-specific components
    layout/         # Layout components (Header, Sidebar, Footer)
  composables/       # Vue composition functions
  router/           # Route definitions
  stores/           # Pinia state stores
  views/            # Page-level components
  assets/           # Static assets (images, styles)
  utils/            # Utility functions
  App.vue           # Root component
  main.js           # Application entry point
```

### Component Design Patterns

#### 1. Atomic Design
- **Atoms**: Basic UI elements (Button, Input, Icon)
- **Molecules**: Simple component combinations (FormField, Card)
- **Organisms**: Complex components (Header, Sidebar, DataTable)
- **Templates**: Page layouts
- **Pages**: Complete views with data

#### 2. Component Composition
- **Composition API** for reusable logic
- **Props down, events up** pattern
- **Scoped slots** for flexible component composition
- **Provide/inject** for deep prop drilling

#### 3. Reusable Components
```vue
<!-- Example: FormField.vue -->
<template>
  <div class="form-field">
    <label :for="fieldId" class="form-label">{{ label }}</label>
    <input
      :id="fieldId"
      v-model="modelValue"
      :type="type"
      :placeholder="placeholder"
      :disabled="disabled"
      class="form-input"
      @blur="validate"
    />
    <span v-if="error" class="form-error">{{ error }}</span>
  </div>
</template>
```

### State Management Architecture

#### 1. Pinia Store Structure
```javascript
// stores/auth.js
export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null,
    token: null,
    isAuthenticated: false
  }),
  getters: {
    isAdmin: (state) => state.user?.role === 'admin',
    userDisplayName: (state) => state.user?.displayName
  },
  actions: {
    async login(credentials) { /* ... */ },
    async logout() { /* ... */ },
    async refreshUser() { /* ... */ }
  }
})
```

#### 2. Store Domains
- **auth**: Authentication state and user info
- **profile**: User profile and preferences
- **catalog**: Service catalog data
- **tickets**: Support ticket management
- **notifications**: Notification system
- **ui**: UI state (loading, modals, toasts)

#### 3. Reactive State Patterns
- **Computed properties** for derived state
- **Watchers** for side effects
- **Persistent state** using localStorage
- **State hydration** on app load

### Routing Architecture

#### 1. Route Definitions
```javascript
// router/index.js
const routes = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/HomeView.vue')
  },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: () => import('@/views/DashboardView.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/admin',
    name: 'admin',
    component: () => import('@/views/admin/AdminLayout.vue'),
    meta: { requiresAuth: true, requiresRole: 'admin' },
    children: [
      { path: 'users', component: () => import('@/views/admin/UsersView.vue') }
    ]
  }
]
```

#### 2. Route Guards
- **Authentication guard**: Check login status
- **Role-based guard**: Verify user permissions
- **Navigation guards**: Handle redirects
- **Lazy loading**: Code splitting for performance

### UI/UX Design System

#### 1. Design Tokens
```scss
// styles/tokens.scss
:root {
  --color-primary: #3b82f6;
  --color-secondary: #64748b;
  --color-success: #10b981;
  --color-warning: #f59e0b;
  --color-error: #ef4444;
  --spacing-xs: 0.25rem;
  --spacing-sm: 0.5rem;
  --spacing-md: 1rem;
  --spacing-lg: 1.5rem;
  --spacing-xl: 2rem;
  --font-size-sm: 0.875rem;
  --font-size-base: 1rem;
  --font-size-lg: 1.125rem;
  --font-size-xl: 1.25rem;
}
```

#### 2. Component Variants
- **Button variants**: primary, secondary, outline, ghost
- **Input variants**: standard, error, disabled
- **Card variants**: default, elevated, bordered
- **Modal variants**: small, medium, large, fullscreen

#### 3. Responsive Design
- **Mobile-first approach**
- **Breakpoint system**: xs, sm, md, lg, xl
- **Grid layout**: CSS Grid and Flexbox
- **Typography scaling**: clamp() for fluid typography

### Data Flow Patterns

#### 1. API Integration
```javascript
// composables/useApi.js
export function useApi() {
  const { data, loading, error } = useFetch()
  
  const get = async (url) => {
    loading.value = true
    try {
      const response = await api.get(url)
      data.value = response.data
    } catch (err) {
      error.value = err
    } finally {
      loading.value = false
    }
  }
  
  return { get, data, loading, error }
}
```

#### 2. Form Handling
```vue
<!-- Example: Reactive form -->
<script setup>
import { reactive, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()

const form = reactive({
  username: '',
  password: '',
  remember: false
})

const errors = reactive({})

const isValid = computed(() => {
  return form.username && form.password && Object.keys(errors).length === 0
})

const submit = async () => {
  if (!isValid.value) return
  await authStore.login(form)
}
</script>
```

### Performance Optimization

#### 1. Code Splitting
- **Route-based splitting**: Lazy load route components
- **Component-based splitting**: Dynamic imports for heavy components
- **Vendor splitting**: Separate bundle for third-party libraries

#### 2. Caching Strategies
- **HTTP caching**: Cache API responses
- **Component caching**: Keep-alive for frequently used components
- **State persistence**: Store user preferences

#### 3. Bundle Optimization
- **Tree shaking**: Remove unused code
- **Compression**: Gzip/Brotli compression
- **Minification**: Reduce bundle size

### Error Handling & User Experience

#### 1. Error Boundaries
```vue
<!-- ErrorBoundary.vue -->
<template>
  <div v-if="error" class="error-boundary">
    <h2>Something went wrong</h2>
    <p>{{ error.message }}</p>
    <button @click="retry">Retry</button>
  </div>
  <slot v-else />
</template>
```

#### 2. Loading States
- **Skeleton screens**: Improve perceived performance
- **Progress indicators**: Show loading progress
- **Optimistic updates**: Immediate UI feedback

#### 3. Toast Notifications
```javascript
// composables/useToast.js
export function useToast() {
  const toasts = ref([])
  
  const success = (message) => {
    toasts.value.push({ type: 'success', message })
  }
  
  const error = (message) => {
    toasts.value.push({ type: 'error', message })
  }
  
  return { success, error, toasts }
}
```

### Accessibility Considerations

#### 1. Semantic HTML
- **Proper heading hierarchy**: h1, h2, h3, etc.
- **Landmark elements**: header, main, nav, footer
- **Form labels**: Associated with inputs

#### 2. Keyboard Navigation
- **Tab order**: Logical navigation flow
- **Focus indicators**: Visible focus states
- **Keyboard shortcuts**: Power user features

#### 3. Screen Reader Support
- **ARIA labels**: Descriptive element labels
- **Live regions**: Dynamic content announcements
- **Alt text**: Image descriptions

## Database Design

### Key Tables
- **users**: User accounts and authentication
- **sessions**: Active user sessions
- **services**: Service catalog
- **tickets**: Support tickets
- **reviews**: Service reviews and ratings
- **notifications**: User notifications
- **audit_logs**: System audit trail

### Data Integrity
- **Foreign key constraints** for relationships
- **Indexes** for performance optimization
- **Transactions** for data consistency
- **Migrations** for schema versioning

## API Design Principles

### RESTful Design
- **Resource-oriented URLs** (`/tickets`, `/users`, `/services`)
- **HTTP methods** for operations (GET, POST, PUT, DELETE)
- **Status codes** for responses (200, 201, 400, 401, 404, 500)
- **JSON** for request/response payloads

### Error Handling
- **Standardized error format** across all endpoints
- **Error codes** for programmatic handling
- **Detailed messages** for debugging
- **Audit logging** for security events

### Performance Considerations
- **Connection pooling** for database efficiency
- **Rate limiting** to prevent abuse
- **Caching** for frequently accessed data
- **Pagination** for large result sets

## Security Features

### Authentication & Authorization
- **Session-based authentication** with secure cookies
- **CSRF protection** for state-changing operations
- **Rate limiting** per endpoint
- **Role-based access control**

### Data Protection
- **Field-level encryption** for sensitive data
- **HMAC verification** for internal APIs
- **Audit logging** for compliance
- **Data export** for privacy requests

### Infrastructure Security
- **Environment variables** for configuration
- **Docker containers** for isolation
- **Database migrations** for controlled schema changes
- **Health checks** for monitoring

## Development Workflow

### Backend Development
1. Define models in `internal/models/`
2. Implement service logic in `internal/{domain}/`
3. Create HTTP handlers in `internal/{domain}/`
4. Register routes in `internal/router/`
5. Add tests for service and handler layers

### Frontend Development

#### 1. Component Development Workflow
```bash
# Create new component
mkdir src/components/common/NewComponent
touch src/components/common/NewComponent/NewComponent.vue
touch src/components/common/NewComponent/NewComponent.spec.js
```

**Component Development Steps:**
1. **Design API**: Define props, events, and slots
2. **Create component**: Implement Vue 3 Composition API
3. **Add styling**: Use design tokens and CSS modules
4. **Write tests**: Unit tests for component behavior
5. **Add documentation**: Component README with examples

#### 2. State Management Development
```bash
# Create new store
touch src/stores/newFeature.js
```

**Store Development Steps:**
1. **Define state**: Reactive data structure
2. **Add getters**: Computed derived state
3. **Implement actions**: Async operations and mutations
4. **Add persistence**: localStorage sync if needed
5. **Write tests**: Store unit tests

#### 3. Route Development
```bash
# Add new route
# Edit src/router/index.js
```

**Route Development Steps:**
1. **Define route**: Path, component, and metadata
2. **Add guards**: Authentication and role checks
3. **Create view**: Page-level component
4. **Add navigation**: Menu items and links
5. **Test navigation**: Manual and automated tests

#### 4. View Development
```bash
# Create new view
touch src/views/NewFeatureView.vue
```

**View Development Steps:**
1. **Layout structure**: Component composition
2. **Data fetching**: API integration
3. **State management**: Store integration
4. **User interactions**: Event handlers
5. **Error handling**: Loading and error states
6. **Responsive design**: Mobile-first approach

#### 5. Styling Development
```scss
// styles/components/NewComponent.scss
.new-component {
  display: flex;
  gap: var(--spacing-md);
  
  &__header {
    font-size: var(--font-size-lg);
    color: var(--color-primary);
  }
  
  &--disabled {
    opacity: 0.6;
    pointer-events: none;
  }
}
```

**Styling Guidelines:**
- **Design tokens**: Use CSS custom properties
- **BEM methodology**: Block, Element, Modifier
- **Responsive design**: Mobile-first approach
- **Component scoping**: CSS modules or scoped styles

#### 6. Testing Strategy
```javascript
// tests/e2e/new-feature.spec.js
import { test, expect } from '@playwright/test'

test.describe('New Feature', () => {
  test('should load and display correctly', async ({ page }) => {
    await page.goto('/new-feature')
    await expect(page.locator('h1')).toHaveText('New Feature')
  })
  
  test('should handle user interactions', async ({ page }) => {
    await page.goto('/new-feature')
    await page.click('[data-testid="action-button"]')
    await expect(page.locator('[data-testid="result"]')).toBeVisible()
  })
})
```

**Testing Pyramid:**
1. **Unit tests**: Component logic and utilities
2. **Integration tests**: Component interactions
3. **E2E tests**: User workflows and critical paths

#### 7. Performance Optimization
```javascript
// Lazy loading example
const HeavyComponent = defineAsyncComponent(() => 
  import('@/components/HeavyComponent.vue')
)
```

**Optimization Techniques:**
- **Code splitting**: Route and component-based
- **Image optimization**: WebP format and lazy loading
- **Bundle analysis**: Identify large dependencies
- **Caching**: HTTP and browser caching strategies

### Testing Strategy
- **Unit tests** for business logic
- **Integration tests** for API endpoints
- **E2E tests** for user workflows
- **Security tests** for authentication flows

## Deployment Architecture

### Containerization
- **Docker** for application packaging
- **Docker Compose** for local development
- **Environment-specific configurations**
- **Health checks** for monitoring

### Data Management
- **Database migrations** for schema changes
- **Backup strategies** for data protection
- **Log aggregation** for monitoring
- **Performance metrics** for optimization

## Future Considerations

### Scalability
- **Horizontal scaling** with load balancers
- **Database sharding** for large datasets
- **Caching layers** for performance
- **Microservices** decomposition

### Monitoring & Observability
- **Structured logging** for analysis
- **Metrics collection** for performance
- **Distributed tracing** for debugging
- **Health monitoring** for reliability

### Security Enhancements
- **OAuth2/OIDC** integration
- **Multi-factor authentication**
- **API key management**
- **Advanced threat detection**

import { createApp } from 'vue'

import App from './App.vue'
import { router } from './router'
import 'ant-design-vue/dist/reset.css'
import './platform/tokens.css'
import './platform/archetypes.css'
import './platform/sources.css'
import './platform/shell.css'
import './platform/components.css'

router.afterEach((to) => {
  const pageTitle = typeof to.meta.title === 'string' ? to.meta.title : ''
  document.title = pageTitle ? `OB Data Orch · ${pageTitle}` : 'OB Data Orch'
})

createApp(App).use(router).mount('#app')

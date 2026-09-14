import { createApp } from 'vue'

import App from './App.vue'
import { router } from './router'
import './style.css'
import './styles/enterprise-tokens.css'
import './workbench/workbench.css'
import './styles/enterprise-workbench.css'

router.afterEach((to) => {
  const pageTitle = typeof to.meta.title === 'string' ? to.meta.title : ''
  document.title = pageTitle ? `OB Data Orch · ${pageTitle}` : 'OB Data Orch'
})

createApp(App).use(router).mount('#app')

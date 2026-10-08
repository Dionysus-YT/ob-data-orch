import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import vueParser from 'vue-eslint-parser'

export default tseslint.config(
  { ignores: ['dist/', 'coverage/'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  { files: ['scripts/**/*.mjs'], languageOptions: { globals: globals.node } },
  { files: ['tests/fixtures/zoom-extension/*.js'], languageOptions: { globals: { chrome: 'readonly' } } },
  {
    files: ['**/*.{ts,vue}'],
    languageOptions: {
      globals: globals.browser,
    },
    rules: {
      'vue/multi-word-component-names': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/html-self-closing': 'off',
    },
  },
  {
    files: ['src/views/*WizardView.vue', 'src/workbench/{export,import}/**/steps/*.vue'],
    rules: {
      'no-restricted-globals': ['error',
        { name: 'fetch', message: '向导网络请求经 api 边界与业务异步管理器处理。' },
        { name: 'XMLHttpRequest', message: '向导网络请求经 api 边界与业务异步管理器处理。' },
      ],
    },
  },
  {
    files: ['**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },
)

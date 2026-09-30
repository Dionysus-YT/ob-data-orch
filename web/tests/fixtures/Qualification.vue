<script setup lang="ts">
import { ref, useId } from 'vue'
import { Button, Checkbox, ConfigProvider, Drawer, Form, FormItem, Input, Radio, Select, Switch, Table, Tabs, TabPane, Textarea } from 'ant-design-vue'
import { platformTheme } from '../../src/platform/theme'
import OrchDangerConfirm from '../../src/components/OrchDangerConfirm.vue'
import { useAntDrawerDialog } from '../../src/composables/useAntDrawerDialog'

const value = ref('合成配置')
const checked = ref(false)
const choice = ref('one')
const drawer = ref(false)
const modal = ref(false)
const saves = ref(0)
const drawerTitleId = useId()
const { heading: drawerHeading, afterOpenChange: afterDrawerOpenChange, captureInvoker: captureDrawerInvoker } = useAntDrawerDialog(drawerTitleId, () => { drawer.value = false })
void drawerHeading
function openDrawer() {
  captureDrawerInvoker()
  drawer.value = true
}
</script>

<template>
  <ConfigProvider :theme="platformTheme" :auto-insert-space-in-button="false">
    <main style="padding: 24px">
      <h1>平台兼容性验证</h1>
      <Form layout="vertical" :model="{ name: value }" @finish="saves++">
        <FormItem label="名称" name="name"><Input v-model:value="value" aria-label="名称" /></FormItem>
        <FormItem label="选择"><Select v-model:value="choice" aria-label="选择" :options="[{value:'one', label:'一'}, {value:'two', label:'二'}]" /></FormItem>
        <Textarea v-model:value="value" aria-label="多行输入" />
        <Checkbox v-model:checked="checked">复选框</Checkbox>
        <Radio :checked="checked" @change="checked = true">单选框</Radio>
        <Switch v-model:checked="checked" aria-label="可用状态" />
        <Button html-type="submit" type="primary">保存</Button>
      </Form>
      <p role="status">已保存 {{ saves }} 次</p>
      <Tabs><TabPane key="facts" tab="事实"><Table :columns="[{ title: '名称', dataIndex: 'name' }]" :data-source="[{ key: 'synthetic', name: value }]" :pagination="false" /></TabPane></Tabs>
      <Button @click="openDrawer">打开抽屉</Button><Button @click="modal = true">打开确认</Button>
      <Drawer :open="drawer" @close="drawer = false" @after-open-change="afterDrawerOpenChange">
        <template #title><h2 :id="drawerTitleId" ref="drawerHeading" class="ant-drawer-title" tabindex="-1">编辑合成配置</h2></template>
        <Input aria-label="抽屉输入" />
      </Drawer>
      <OrchDangerConfirm :open="modal" title="确认合成动作" @cancel="modal = false" @confirm="modal = false">仅验证框架交互，不执行业务请求。</OrchDangerConfirm>
    </main>
  </ConfigProvider>
</template>

<script setup lang="ts" generic="T extends object">
import { ConfigProvider, Table } from 'ant-design-vue'
import { managementTheme } from '@/platform/theme'
import type { TableColumnsType } from 'ant-design-vue'
import type { ColumnType } from 'ant-design-vue/es/table'

// 领域层提供已授权的一页事实；产品表格禁止客户端分页、排序或过滤来重解释服务端范围。
defineProps<{ rows: readonly T[]; columns: TableColumnsType<T>; label: string; rowKey: string; selectedKey?: string | null; loading?: boolean; minimumWidth?: number }>()
defineSlots<{ bodyCell(props: { record: T; column: ColumnType<T>; index: number }): unknown; empty(): unknown }>()
</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <div class="orch-operational-table" role="region" :aria-label="label" tabindex="0">
      <Table :columns="columns" :data-source="[...rows]" :row-key="rowKey" :pagination="false" :loading="loading" :scroll="minimumWidth ? { x: minimumWidth } : undefined" :row-class-name="(row: T) => (row as Record<string, unknown>)[rowKey] === selectedKey ? 'is-selected' : ''" :table-layout="columns.every(column => 'width' in column && column.width) ? 'fixed' : 'auto'">
        <template #title><span class="orch-sr-only">{{ label }}</span></template>
        <template #bodyCell="cell"><slot name="bodyCell" :record="cell.record as T" :column="cell.column as ColumnType<T>" :index="cell.index" /></template>
        <template #emptyText><slot name="empty">当前范围内没有记录</slot></template>
      </Table>
    </div>
  </ConfigProvider>
</template>

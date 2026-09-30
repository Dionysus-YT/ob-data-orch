<script setup lang="ts">
import { ExclamationCircleOutlined } from '@ant-design/icons-vue'
import { Checkbox as ACheckbox, Radio as ARadio, Input as AInput, Button as AButton } from 'ant-design-vue'
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import WizardFrame from '@/components/WizardFrame.vue'

const route = useRoute()
const activeStep = computed(() => {
  const value = Number(route.query.step ?? '1')
  return Number.isInteger(value) && value >= 1 && value <= 6 ? value : 1
})
const titles = ['适用条件', '连接与版本', '单表文件', '表与格式', '执行参数', '预检查与命令']
</script>

<template>
  <section class="page-heading">
    <div>
      <h1>新建旁路导入任务</h1>
      <p>OBLOADER 旁路导入是独立流程：只承载单目标表的数据批量写入，SQL 与 RPC 分别校验，且不支持检查点继续。</p>
    </div>
    <span class="draft-status">草稿尚未创建</span>
  </section>
  <WizardFrame kind="direct" :active-step="activeStep">
    <template #default>
      <section v-if="activeStep === 1" class="form-section">
        <h2>确认旁路导入适用条件</h2>
        <p>旁路导入不是普通导入的高级开关。只有明确适合大批量、唯一目标表写入的任务，才进入后续配置。</p>
        <div class="option-grid three form-line">
          <label class="option-card selected"><ACheckbox checked />大批量单目标表<span>本任务只写入一个目标表，不承载对象定义或多表导入。</span></label>
          <label class="option-card selected"><ACheckbox checked />同表文件分片<span>多个输入文件只能是同一目标表的数据分片。</span></label>
          <label class="option-card selected"><ACheckbox checked />接受整体提交<span>表级整体提交；失败后只能从头重新执行。</span></label>
        </div>
        <section class="context-note form-line"><span><ExclamationCircleOutlined class="product-icon" aria-hidden="true" /></span><span>旁路导入通过 RPC 传输数据，并受目标表索引和结构限制约束；不提供断点续传、错误阈值或任务级重试。</span></section>
      </section>

      <section v-else-if="activeStep === 2" class="form-section">
        <h2>数据源、连接场景与版本</h2>
        <div class="section-block">
          <h3>连接场景</h3>
          <div class="option-grid three">
            <label class="option-card selected"><ARadio checked name="scenario" />OceanBase Cloud + Cloud ODP<span>SQL 连接与云上旁路端口分别确认。</span></label>
            <label class="option-card"><ARadio name="scenario" />私有 OceanBase + 私有 ODP<span>ODP SQL 端口与 RPC 监听端口分开校验。</span></label>
            <label class="option-card"><ARadio name="scenario" />私有 OceanBase + OBServer<span>直连时须额外确认跨机事务风险。</span></label>
          </div>
        </div>
        <div class="section-block">
          <h3>SQL 与 RPC 连接</h3>
          <div class="parameter-list">
            <div><strong>SQL 连接</strong><span>从已有数据源只读获取；不在向导中编辑凭据。</span><AInput disabled placeholder="功能接入后显示数据源摘要" /></div>
            <div><strong>RPC 地址与端口 <b>*</b></strong><span>来源必须可追溯：数据源、节点、安全查询或授权输入。</span><AInput placeholder="功能接入后按来源填写" /></div>
          </div>
          <p class="section-hint">RPC 地址和端口必须单独验证，不能直接沿用 SQL 地址或端口。</p>
        </div>
        <div class="section-block">
          <h3>版本矩阵</h3>
          <div class="placeholder-control short"><span>预检查将核对 OBLOADER、OBServer 与 ODP 的实际版本组合；未执行前不展示“已通过”。</span></div>
        </div>
      </section>

      <section v-else-if="activeStep === 3" class="form-section">
        <h2>单表数据文件</h2>
        <div class="section-block">
          <h3>输入位置</h3>
          <div class="radio-row"><label><ARadio checked name="file-source" />执行节点本地绝对路径</label><label><ARadio disabled name="file-source" />远程存储（待官方参数映射确认）</label></div>
          <label class="field-label wide-label">文件或目录路径 <b>*</b><AInput aria-label="文件或目录路径 *" placeholder="由执行节点读取的完整绝对路径" /></label>
          <p class="section-hint">不提供浏览器上传、任意文件浏览、平台文件管理或跨节点复制。</p>
        </div>
        <div class="section-block">
          <h3>同表分片发现</h3>
          <div class="placeholder-control short"><span>功能接入后由执行节点确认路径可读、文件集合、后缀和总量；不读取业务内容进行字段推断。</span></div>
          <p class="section-hint">目录、文件后缀和第三方文件模式仅在对应官方条件满足时显示。</p>
        </div>
      </section>

      <section v-else-if="activeStep === 4" class="form-section">
        <h2>唯一目标表、格式与结构</h2>
        <div class="section-block">
          <h3>目标表</h3>
          <label class="field-label wide-label">数据库与唯一目标表 <b>*</b><AInput aria-label="数据库与唯一目标表 *" placeholder="功能接入后从已授权数据源读取并选择" /></label>
          <p class="section-hint">目标表是否为空将派生全量或增量旁路模式；不提供手工切换。DDL、MIX、全部对象和多表均不进入旁路导入。</p>
        </div>
        <div class="section-block">
          <h3>文件格式</h3>
          <div class="format-grid">
            <label class="format-card selected"><ARadio checked name="format" />CSV<span>三种已确认连接场景均可作为旁路格式。</span></label>
            <label class="format-card pending"><ARadio disabled name="format" />Parquet<span>待官方参数映射确认</span></label>
            <label class="format-card pending"><ARadio disabled name="format" />ORC<span>待官方参数映射确认</span></label>
          </div>
          <p class="section-hint">格式可用性由“格式 + 连接场景”共同决定，普通导入支持不自动等于旁路支持。</p>
        </div>
        <div class="section-block">
          <h3>结构限制</h3>
          <div class="placeholder-control short"><span>预检查将核对列结构、唯一索引冲突和当前格式的旁路适用证据；不在此页面假设结构可用。</span></div>
        </div>
      </section>

      <section v-else-if="activeStep === 5" class="form-section">
        <h2>执行参数与 Direct Load 配置</h2>
        <div class="parameter-list">
          <div><strong>旁路标识</strong><span><code>--direct</code> 由旁路流程固定派生，不能关闭。</span><AInput disabled value="系统固定生成" /></div>
          <div><strong>客户端并发 thread</strong><span>客户端连接服务端连接池；与 parallel 分开配置。</span><AInput disabled placeholder="沿用官方默认" /></div>
          <div><strong>服务端并发 parallel</strong><span>OBServer 写入与排序线程；不自动与 thread 联动。</span><AInput disabled placeholder="沿用官方默认" /></div>
          <div><strong>执行节点配置</strong><span>session.config.json 与节点工具事实仅只读展示。</span><AInput disabled placeholder="尚未选择执行节点" /></div>
        </div>
        <section class="context-note form-line"><span><ExclamationCircleOutlined class="product-icon" aria-hidden="true" /></span><span>并发值由预检查结合实际节点与目标表事实评估；页面不会给出伪精确推荐，也不会自动改写任一参数。</span></section>
        <details class="parameter-fold"><summary>高级配置 <span>已确认的文件解析、日志与并发参数按条件显示</span></summary><p>例如 CSV 列分隔符、文件后缀、第三方文件模式和日志目录；未确认旁路适用性的参数保持“待官方参数映射确认”。</p></details>
        <details class="parameter-fold"><summary>专家配置 <span>103 个 OBLOADER 参数均有适用状态，不适用项不生成</span></summary><p>普通导入专属的 <code>--retry</code>、<code>--max-errors</code>、<code>--max-discards</code>、<code>--ddl</code>、<code>--all</code> 不在旁路任务中出现。</p></details>
      </section>

      <section v-else class="form-section">
        <h2>预检查、完整命令与风险确认</h2>
        <section class="precheck-list">
          <h3>预检查结果</h3>
          <div v-for="item in ['适用条件与整体提交', 'SQL 与 RPC 连接', '版本矩阵与节点工具', '文件集合与唯一目标表', '格式与结构限制', 'Direct Load 参数约束', '最终风险确认']" :key="item" class="precheck-item"><span class="status-dot neutral" /><strong>{{ item }}</strong><span>尚未执行；当前页面不会把未知事实显示为通过。</span></div>
        </section>
        <section class="command-empty">
          <div><h3>旁路导入执行命令（只读）</h3><AButton disabled>复制脱敏命令</AButton></div>
          <pre><code>完成旁路配置和 Agent 预检查后，由控制面生成只读、脱敏的 OBLOADER 命令。</code></pre>
          <p>命令只包含已确认的旁路适用参数，并绑定当前预检查和不可变任务快照。</p>
        </section>
        <section class="context-note form-line"><span><ExclamationCircleOutlined class="product-icon" aria-hidden="true" /></span><span>旁路失败不提供“从检查点继续”或“从失败点继续”；后续只能基于原配置新建任务并从头执行。</span></section>
      </section>
    </template>

    <template #summary>
      <h2>配置总览</h2>
      <dl class="summary-definition">
        <div><dt>当前步骤</dt><dd>{{ titles[activeStep - 1] }}</dd></div>
        <div><dt>适用条件</dt><dd>尚未确认</dd></div>
        <div><dt>数据源 / RPC</dt><dd>尚未配置</dd></div>
        <div><dt>文件集合</dt><dd>尚未配置</dd></div>
        <div><dt>唯一目标表</dt><dd>尚未选择</dd></div>
        <div><dt>格式与执行参数</dt><dd>尚未配置</dd></div>
        <div><dt>预检查</dt><dd>尚未执行</dd></div>
      </dl>
      <p class="aside-note">旁路导入使用 SQL 与 RPC 双连接，表级整体提交。失败后仅支持从头执行或基于原配置新建。</p>
    </template>
  </WizardFrame>
</template>

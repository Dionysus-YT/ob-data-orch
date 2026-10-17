import type { UnwrapNestedRefs } from 'vue'
import type { NormalImportWizard } from './useNormalImportWizard'

type Model = UnwrapNestedRefs<NormalImportWizard>
// 端口引用同一表单，不产生字段副本；各步骤不能取得 API 客户端或完整事务管理器。
export type NormalImportSourceModel = Pick<Model, 'form' | 'fieldPrefix' | 'sourceKeyword' | 'sourceEnvironment' | 'sourceFailure' | 'loadingSources' | 'sources' | 'visibleSources' | 'eligibleSources' | 'selectedSource' | 'loadSources' | 'selectSource' | 'fieldError'>
export type NormalImportFileSettingsModel = Pick<Model, 'form' | 'fieldPrefix' | 'nodes' | 'selectedNode' | 'nodeFailure' | 'loadingNodes' | 'loadNodes' | 'fieldError'>
export type NormalImportFormatSettingsModel = Pick<Model, 'form' | 'fieldPrefix' | 'traits' | 'currentFormat' | 'selectedSource' | 'fieldError'>
export type NormalImportInputModel = NormalImportFileSettingsModel & NormalImportFormatSettingsModel & Pick<Model, 'selectContent' | 'inputAdvancedCount'>
export type NormalImportObjectsModel = Pick<Model, 'form' | 'fieldPrefix' | 'traits' | 'currentFormat' | 'availableObjectTypes' | 'objectSummary' | 'selectScope' | 'fieldError'>
export type NormalImportExecutionModel = Pick<Model, 'form' | 'fieldPrefix' | 'traits' | 'selectedSource' | 'executionFields' | 'executionAdvancedCount' | 'riskDescriptions' | 'riskAcknowledged' | 'acknowledgeRisk' | 'fieldError'>
export type NormalImportReviewModel = Pick<Model, 'fieldPrefix' | 'issues' | 'moveToStep' | 'activePrecheck' | 'discoveredFiles' | 'checkExpired' | 'commandPreview' | 'previewingCommand' | 'copyingCommand' | 'copyCommand'>
export type NormalImportSubmitModel = Pick<Model, 'submitConfirmationOpen' | 'submitting' | 'taskNameInput' | 'taskNameError' | 'submitTask' | 'fieldPrefix' | 'reviewFailure' | 'riskDescriptions' | 'selectedSource'>
export type NormalImportSummaryModel = Pick<Model, 'form' | 'configuration' | 'selectedSource' | 'selectedNode' | 'contentLabel' | 'formatLabel' | 'objectSummary' | 'riskDescriptions' | 'currentDraft' | 'draftDirty' | 'draftNeedsRebind' | 'loadingDraft' | 'activePrecheck' | 'checkExpired'>

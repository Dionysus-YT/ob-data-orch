import type { ThemeConfig } from 'ant-design-vue/es/config-provider/context'
import { component, foundation, product, semantic } from './tokens'

// 环境是业务上下文，Tag 使用冻结环境色，不借用成功或失败状态色。
export function environmentTagTheme(environment: string): ThemeConfig {
  const colors = product.environment[environment as keyof typeof product.environment]
  return { components: { Tag: { colorText: colors?.color ?? semantic.secondary, colorFillTertiary: colors?.background ?? semantic.subtle } } }
}

// P0 冻结的作用域差异通过框架主题继承表达，不在页面重写 Ant 选择器。
export const managementTheme: ThemeConfig = { token: { borderRadius: component.control.managementRadius } }
export const overlayTheme: ThemeConfig = { token: { borderRadius: component.control.radius } }

// 框架主题只消费 Canonical Token；页面不得再覆盖组件 token 或使用默认 small 密度。
export const platformTheme: ThemeConfig = {
  token: {
    colorPrimary: semantic.primary,
    colorPrimaryHover: semantic.primaryHover,
    colorPrimaryActive: semantic.primaryActive,
    colorLink: semantic.link,
    colorLinkHover: semantic.primaryHover,
    colorLinkActive: semantic.primaryActive,
    colorSuccess: semantic.success,
    colorWarning: semantic.warning,
    colorError: semantic.danger,
    colorInfo: semantic.link,
    colorText: semantic.formText,
    colorTextSecondary: semantic.formSecondary,
    colorTextDisabled: semantic.disabled,
    colorBgContainer: semantic.surface,
    colorBgLayout: semantic.page,
    colorBgElevated: semantic.surface,
    colorBgMask: foundation.mask,
    colorBorder: semantic.border,
    colorBorderSecondary: foundation.rowBorder,
    controlItemBgHover: foundation.rowHover,
    controlItemBgActive: semantic.selected,
    controlItemBgActiveHover: semantic.primarySoft,
    fontFamily: foundation.fontUI,
    fontSize: component.control.fontSize,
    lineHeight: component.control.lineHeight / component.control.fontSize,
    controlHeight: component.control.height,
    borderRadius: component.control.radius,
    borderRadiusLG: component.overlay.radius,
    boxShadow: foundation.overlayShadow,
    boxShadowSecondary: foundation.overlayShadow,
    motionDurationFast: `${component.motion.feedbackMs / 1000}s`,
    motionDurationMid: `${component.motion.feedbackMs / 1000}s`,
    motionDurationSlow: `${component.motion.feedbackMs / 1000}s`,
    controlOutlineWidth: component.focus.width,
  },
  components: {
    Button: { controlOutline: 'transparent', controlTmpOutline: 'transparent', colorErrorOutline: 'transparent' },
    Table: {
      colorFillAlter: foundation.rowHover,
      colorTextHeading: semantic.secondary,
      colorText: semantic.text,
      controlItemBgActive: semantic.selected,
      colorBorderSecondary: foundation.rowBorder,
      padding: component.table.paddingBlock,
      borderRadiusLG: component.table.radius,
    },
    // Vue 4.2.6 根据 fontSize × lineHeight 推导滑轨高度和滑块尺寸；不是 React trackHeight API。
    Switch: {
      fontSize: component.control.fontSize,
      lineHeight: component.availability.height / component.control.fontSize,
      colorTextQuaternary: semantic.muted,
    },
    Form: { fontSize: component.field.labelSize, colorTextHeading: semantic.formSecondary },
    Drawer: { colorBgElevated: semantic.surface },
    Modal: { fontSizeHeading5: component.overlay.titleSize, lineHeightHeading5: component.overlay.titleLineHeight / component.overlay.titleSize },
  },
}

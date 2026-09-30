# Design Tokens

> 本目录描述设计 token taxonomy。实现唯一值源仍是 `web/src/platform/tokens.ts`；`web/src/platform/tokens.css` 由 `npm run tokens:generate` 生成，禁止手改。

## 1. Taxonomy

```text
Foundation Token
  ↓
Semantic Token
  ↓
Ant Theme Token + Product Layout Token
```

本项目不建立 Button/Input/Select/Form 等通用组件状态 token。Generic interaction state 归 Ant Design Vue；Product token 只处理产品布局、密度、证据面和 shell 几何。

## 2. Foundation

| Token | Value | 用途 |
| --- | --- | --- |
| `white` | `#FFFFFF` | Surface |
| `ink` | `#132039` | 强文本 |
| `formInk` | `#132039` | Ant 基础正文 |
| `secondary` | `#5F6B7A` | 次级文本 |
| `blue` | `#0B6BFF` | Action / link / focus |
| `blueHover` | `#3385FF` | hover |
| `blueActive` | `#0057D9` | active / selected text |
| `blueSoft` | `#EAF3FF` | selected navigation / active surface |
| `blueSubtle` | `#F4F8FF` | hover surface |
| `page` | `#F3F6FC` | 页面底色 |
| `navigation` | `#F7F9FD` | navigation / header background |
| `subtle` | `#F8FAFD` | muted surface |
| `border` | `#E5EAF2` | 默认边界 |
| `danger` | `#DC2626` | destructive semantic |
| `fontUI` | Noto Sans SC + system CJK fallback | 中文 UI |
| `fontMono` | JetBrains Mono + system mono fallback | 技术 identifier |
| `space` | `4 / 8 / 12 / 16 / 24 / 32px` | 4px spacing scale |

Success / Warning / Error 通过 Ant semantic token 消费，页面不得另建状态色系统。

## 3. Semantic

| Semantic | Foundation |
| --- | --- |
| `surface` | `white` |
| `page` | `page` |
| `subtle` | `subtle` |
| `text` | `ink` |
| `secondary` | `secondary` |
| `formText` | `formInk` |
| `formSecondary` | `secondary` |
| `muted` | `muted` |
| `disabled` | `disabled` |
| `border` | `border` |
| `primary` | `blue` |
| `primaryHover` | `blueHover` |
| `primaryActive` | `blueActive` |
| `primarySoft` | `blueSoft` |
| `primarySubtle` | `blueSubtle` |
| `link` | `linkBlue` |
| `selected` | `selected` |
| `danger` | `danger` |

业务页面优先消费 semantic token 或生成后的 `--ob-color-*` CSS variable，不直接写 raw hex。

## 4. Component / Layout

这些 token 是产品布局和密度，不是通用组件状态：

| Token | Value |
| --- | --- |
| `component.control.height` | `36px` |
| `component.control.radius` | `4px` |
| `component.control.managementRadius` | `6px` |
| `component.table.headerHeight` | `36px` |
| `component.table.rowHeight` | `42px` |
| `component.table.radius` | `8px` |
| `component.action.iconSize` | `32px` |
| `component.focus.width / offset` | `2px / 2px` |
| `product.shell.navigationWidth` | `200px` |
| `product.shell.collapsedWidth` | `48px` |
| `product.shell.headerHeight` | `48px` |
| `product.shell.workspacePadding` | `24px` |
| `product.task.railWidth` | `208px` |
| `product.task.compactRailWidth` | `176px` |
| `product.source.editorWidth` | `520px` |
| `product.inspector.width` | `640px` |

## 5. Ant Theme Mapping

`web/src/platform/theme.ts` 固定消费 token：

| Ant Token | Source |
| --- | --- |
| `colorPrimary` | `semantic.primary` |
| `colorPrimaryHover` | `semantic.primaryHover` |
| `colorLink` | `semantic.link` |
| `colorSuccess` | `semantic.success` |
| `colorWarning` | `semantic.warning` |
| `colorError` | `semantic.danger` |
| `colorText` | `semantic.formText` |
| `colorTextSecondary` | `semantic.formSecondary` |
| `colorBgLayout` | `semantic.page` |
| `colorBgContainer` | `semantic.surface` |
| `colorBorder` | `semantic.border` |
| `fontFamily` | `foundation.fontUI` |
| `fontSize` | `component.control.fontSize` |
| `controlHeight` | `component.control.height` |
| `borderRadius` | `component.control.radius` |

Ant component token 只用于已知产品差异，例如管理页 radius、Table 几何和 Switch 业务状态尺寸。不得通过 theme 或 CSS 新增第二套 Button/Input/Form 状态。

## 6. Generated CSS Contract

生成变量前缀：

| Prefix | Meaning |
| --- | --- |
| `--ob-foundation-*` | 原始值 |
| `--ob-color-*` | semantic color |
| `--ob-component-*` | 产品密度、表格、焦点、浮层几何 |
| `--ob-product-*` | shell、navigation、task rail、source editor、inspector |

检查：

```powershell
cd web
npm run tokens:generate
npm run tokens:check
```

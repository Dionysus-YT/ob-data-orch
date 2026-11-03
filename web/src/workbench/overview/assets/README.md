# 首页立体指标插画

`task-metrics-isometric.png` 是本项目随首页代码一起构建的透明 2×2 图集，依次为左上运行、右上等待、左下成功、右下失败。`OverviewMetrics.vue` 使用四象限背景定位，文字、数量、焦点和点击仍由真实 UI 承担，插画对无障碍树隐藏。

初始生成日期：2026-10-28；2026-10-31 用户要求贴合产品主色调，使用内置 ImageGen 编辑原图，将紫/绿/红材质统一为蓝白，保留四象限、四种造型与符号。当前交付为 1254×1254 透明 RGBA PNG，随 Vite 构建；旧图仅留在本轮外部验收基线，不再被页面引用。仅首页四卡允许该装饰与既有 surfaceShadow，不扩展全平台颜色或控件规则，不改变业务与图表语义色。

最终生成提示词：

```text
Use case: precise-object-edit. EDIT TARGET: the attached existing transparent 2x2 sprite atlas for an enterprise database operations application. Recolor ONLY the four existing miniature isometric illustrations to one coherent restrained enterprise blue-and-white product palette, anchored at #0B6BFF, light powder blue #DCEAFF, porcelain white, and neutral blue-gray shadows. Replace every purple, green, emerald, red and coral material with corresponding blue tones. Retain all four DIFFERENT original objects and symbols exactly: top-left interconnected data cubes and circular flow arrows, top-right layered queue with clock, bottom-left cubes with check, bottom-right cubes with X. Preserve their exact quadrant order, camera angle, geometry, relative size, padding, platforms, light direction and readable symbols. Keep white and pale neutral surfaces; reduce excessive candy-like saturation. The four artworks should feel like one product family and use the SAME blue material family, not four semantic colors. No extra objects, text, letters, numbers, logos, borders, colored cards or background. Deliver ONE square production PNG sprite atlas with genuine transparent alpha, a perfectly equal 2-by-2 grid. Each object must be fully contained and centered in its original quadrant with transparent space between all quadrants, nothing crossing midlines. This replaces the existing asset, so preserve the sprite layout faithfully. Transparent background.
```

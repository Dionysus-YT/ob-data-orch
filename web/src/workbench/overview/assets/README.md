# 首页立体指标插画

`task-metrics-isometric.png` 是本项目随首页代码一起构建的透明 2×2 图集，依次为左上运行、右上等待、左下成功、右下失败。`OverviewMetrics.vue` 使用四象限背景定位，文字、数量、焦点和点击仍由真实 UI 承担，插画对无障碍树隐藏。

生成日期：2026-10-28；工具：内置 ImageGen。用户要求卡片有 OCP 式立体感与特色；这是按描述新生成的插画，不包含业务数据或秘密。仅首页四卡允许该装饰与既有 surfaceShadow，不扩展全平台颜色或控件规则。

最终生成提示词：

```text
Use case: stylized-concept. Asset type: production UI illustration sprite atlas for a Chinese enterprise database orchestration console. Create ONE square PNG sprite atlas with a transparent background, a perfectly even 2 by 2 grid of four separated small isometric 3D icons. Each icon is centered inside its own quadrant with equal scale and generous transparent padding; nothing crosses quadrant boundaries. No drawn grid, no text, no letters, no digits, no logos, no watermark. Style: refined OceanBase OCP-inspired geometric infrastructure miniatures, orthographic isometric camera, crisp broad bevels, matte porcelain with subtle glass accents, soft top-left lighting, delicate contact shadows, restrained enterprise palette, high quality at 80px display size. TOP LEFT: cobalt and pale-blue interlocking cubic data pipeline with a small raised circular flow arrow, representing running tasks. TOP RIGHT: violet layered queue blocks with a compact clock element, representing waiting tasks. BOTTOM LEFT: mint and emerald completed data cube on a plinth with a clear inset check mark, representing successful tasks. BOTTOM RIGHT: warm coral and ruby data cube with a small raised warning diamond and subtle cross, representing failed tasks. All four are structurally related, purposeful 3D objects rather than flat icons in colored tiles. Keep each object fully visible, centered, at the same visual scale, with the same plinth and camera angle. Genuine transparent background outside the objects and their soft shadows. Target 1024 by 1024 pixels.
```

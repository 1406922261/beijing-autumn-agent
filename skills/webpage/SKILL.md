---
name: webpage
description: 当场写一个网页。用户说"写个网页/做个页面/生成一个 HTML/给我个网页"时使用，产出一个可直接双击打开的单文件 HTML，保存到 output/ 目录
---

# 技能：当场写一个网页

## 目标
产出一个**单文件、可离线打开**的 HTML 页面，保存到项目目录 `output/` 下。

## 执行步骤（必须按顺序）
1. 确定主题：用户没说就默认「北京赏秋」。确认标题与一句 slogan。
2. 取真实数据：景点/花期/门票等信息优先调用 `get_autumn_spots` / `get_autumn_index`，不要凭空编数字。
3. 一次性生成完整 HTML，用 `write_file` 写入 `output/<英文短名>.html`（如 `output/beijing-autumn.html`）。
4. 写完后用一句话告诉用户：文件路径 + 双击即可用浏览器打开。

## 页面要求
- 结构完整：`<!DOCTYPE html>` + `<html lang="zh-CN">` + `<head>` + `<body>`，含 `<meta charset="utf-8">` 与 viewport
- 样式全部内联在 `<style>` 里，**禁止引用任何外部 CSS / JS / 字体 / 图片 CDN**（保证断网可打开）
- 视觉：北京秋天配色——红墙 `#8C1F28`、金叶 `#D8A24A`、暖白 `#FBF6EC`、墨色 `#2B2622`
- 布局：首屏大标题 + slogan；下面 3~5 个卡片区块（景点/花期/交通/避坑等）；底部一行小字说明数据来源
- 响应式：`max-width` 容器 + flex/grid，手机上也好看
- 文案用抖音口吻（短句、少量 emoji），但**不要整页都是 emoji**

## 禁止
- 不要输出 Markdown 代码块当作结果——必须真正调用 `write_file` 落盘
- 不要只写半截 HTML（缺 `</html>` 等）
- 不要引用外链资源、不要用需要联网的框架

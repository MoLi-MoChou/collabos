# DS Bot (Fei2411/DeepSeek-Bot) 视觉语言笔记

来源：仓库 docs/images/*.png（官方截图，1400x860@2x）+ ds-bot/src、app/ui/src 中的 CSS 变量（--bt-*）。
许可：Apache-2.0（LICENSE + NOTICE）。仅借鉴视觉语言与数值，CollabOS 代码全部重写，未拷贝源码。
未在本机实际运行：DS Bot 依赖 DeepSeek Harness (DSH) 宿主 + 模型配置，参考图用官方截图。

## 布局
- 三栏：左侧栏 ~244px（--bt-sidebar #f7f7f7）、主区（--bt-main #fcfcfc）、可选右侧详情面板 ~370px（同主区底色，左侧 1px 细线）。
- 顶部 48px 带（--dsb-band），标题是**居中的悬浮胶囊**（Bot 头像 + 名称），右上角圆形图标按钮。
- 消息列宽 min(960px,100%)，居中。

## 颜色（light / dark）
- ink #141414 / #f2f2f2；ink-2 60%；ink-3 36% 透明度。
- 线 rgba(ink,.08)、强线 .15；hover rgba(119,119,119,.09)，active .17。
- 他人气泡 #eeeeee / #262626；**自己气泡 #070707 黑底白字**（dark 反转为浅底黑字）。
- 卡片 #fff / #1f1f1f；强调色 DeepSeek 蓝 #4d6bfe；绿 #00c972；危险 #e02135；星标 #ff9800。
- tag：bg rgba(ink,.06)，字 rgba(ink,.56)。

## 字体
- 系统栈 -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei"…；mono ui-monospace。
- 字号以 12/13/14px 为主，11px 用于标签，17/22px 用于标题。

## 圆角 / 阴影
- 999px 胶囊（按钮、输入栏、标题胶囊）、50% 圆形头像；卡片 12px，气泡 ~16px，小控件 6–8px。
- 几乎不用投影，用 `inset 0 0 0 1px var(--line)` 描边代替边框。

## 组件
- 侧栏：Logo+名称，右侧圆形搜索/新建；会话项为头像+名称+一行摘要，选中项灰底 12px 圆角。
- 消息：他人左侧，头像贴底，名称（Bot 名用其主题色）在气泡上方，旁边灰色小 tag（如“管理员”）；@提及为蓝色文字。
- 系统事件：居中灰色小字一行（“已创建 写手”）。
- 输入栏：底部白色大圆角胶囊，左侧圆形“+”，右侧圆形蓝色发送/语音按钮，占位“给 X 发消息”。
- 右面板：大头像居中、名称、灰色角色；分段胶囊 tab（详情|浏览器）；字段为白色描边卡片。

## 动效
- spring 曲线 cubic-bezier(.22,1,.36,1)，消息/面板淡入上浮；按钮按下轻微缩放。

## 暗色
- main #141414，sidebar #1b1b1b，card #1f1f1f，bubble #262626，线为白 8%。

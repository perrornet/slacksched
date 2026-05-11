# 首轮会话说明（可选）

此文件由配置项 `scheduler.first_turn_prompt_md_path` 引用。开启后，**仅在该 Slack 线程第一次成功发往 Provider 的提示词里**，本文会出现在 Slack 用户内容**之前**（`---` 分隔）。

ACP `session/prompt` 当前只发送用户侧文本块，没有单独的 *system role*；需要模型优先遵守的短约束可写在这里。

将本文件复制到任意可跟踪路径并写入 `first_turn_prompt_md_path`，或直接修改仓库内本示例路径。

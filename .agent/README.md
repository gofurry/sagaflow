# Agent Onboarding

`.agent/` 保存面向后续 Agent 的持久项目上下文。根目录 [`AGENTS.md`](../AGENTS.md) 是强制契约，本目录提供执行任务时需要的展开说明。

## 建议阅读顺序

1. [`../AGENTS.md`](../AGENTS.md)：不可违反的产品、数据、验证和 Git 约束。
2. [`project.md`](project.md)：产品目标、非目标、运行形态和用户心智模型。
3. [`architecture.md`](architecture.md)：进程、数据流、模块边界和关键入口。
4. [`playbooks.md`](playbooks.md)：常见任务的文件落点与实施顺序。
5. [`quality-gates.md`](quality-gates.md)：测试、构建、提交和交付检查表。

## 快速接手清单

```powershell
git status --short
git branch --show-current
git log -5 --oneline
go test ./...
```

然后按任务范围读取代码。不要一开始扫描 `data/`、`bin/`、`dist/` 或用户本地数据库；这些目录包含运行产物或可能的敏感信息，不是理解源码所必需的。

## 信息优先级

发生冲突时按以下顺序判断：

1. 用户当前明确要求；
2. 根目录 `AGENTS.md`；
3. 可执行代码和测试；
4. `.agent/` 文档；
5. `docs/` 与 `README.md`；
6. 历史提交或注释。

如果代码与文档不一致，先通过测试和调用链确定真实行为，再在同一任务中修正文档。

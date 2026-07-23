# 模型目录与更新包

SagaFlow 的模型目录分成三层，避免模型平台的一次参数调整迫使用户升级整个二进制：

1. 编译期目录：仓库中的 `internal/modelcatalog/model-catalog.json` 随二进制发布，提供核心模型和开箱即用的服务连接；同一个文件可直接作为 GitHub Release 资产发布。
2. 在线发现：Ollama 与硅基流动等支持发现的连接可读取当前账号实际可见的模型；新发现的模型以“动态兼容”级别导入。
3. 目录更新包：`data/catalog/model-catalog.json` 会覆盖或补充编译期目录，可由 GitHub Release 单独发布。

## 支持级别

- `verified`：当前 SagaFlow 版本已经完成真实请求验证。
- `compatible`：使用已知兼容协议和通用参数映射，平台变更时可由目录更新包修正。
- `experimental`：已经识别，但能力或参数还没有完整验证。

支持级别描述的是 SagaFlow 对调用链路的验证程度，不代表模型质量。

## 命令

```bash
sagaflow catalog status
sagaflow catalog update
sagaflow catalog install --file model-catalog.json
```

`catalog update` 默认下载：

```text
https://github.com/gofurry/sagaflow/releases/latest/download/model-catalog.json
```

安装会先校验 JSON、Schema 版本、模型能力、参数 Schema 与支持级别，再原子替换本地文件。更新后的目录会在下次启动时同步到 SQLite；用户对模型“启用/停用”的选择不会被覆盖。

## Release 资产格式

更新包使用 `schema_version: 1`。公共参数可以放在 `profiles` 中，模型通过 `profile` 继承，再按需覆盖：

```json
{
  "schema_version": 1,
  "catalog_version": "2026.07.23.1",
  "published_at": "2026-07-23T00:00:00+08:00",
  "profiles": {
    "example-chat": {
      "task": "chat",
      "capability": "text",
      "input_modalities": ["text"],
      "features": ["chat"],
      "parameter_schema": {
        "type": "object",
        "properties": {
          "temperature": {
            "type": "number",
            "title": "温度",
            "minimum": 0,
            "maximum": 2
          }
        }
      },
      "default_parameters": {
        "temperature": 0.7
      },
      "support_status": "compatible"
    }
  },
  "models": [
    {
      "profile": "example-chat",
      "provider_code": "example",
      "model_id": "vendor/model-name",
      "display_name": "Model Name"
    }
  ]
}
```

同一个 `provider_code + model_id + capability` 会稳定映射到同一个目录项。更新包可以调整显示名、输入类型、特性、参数 Schema、默认参数、文档地址和支持级别，但不能在后台改变用户的启用状态。

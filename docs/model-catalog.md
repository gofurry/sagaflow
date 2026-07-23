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

## 更新入口

在“模型 → 模型目录”右侧工具栏打开“更新模型目录”，可以粘贴更新 JSON 或选择本地 JSON 文件。模态框提供默认更新文件的跳转：

```text
https://github.com/gofurry/sagaflow/releases/latest/download/model-catalog.json
```

导入会先校验 JSON、Schema 版本、模型能力、参数 Schema、支持级别、生命周期以及当前二进制是否包含对应服务商，再原子替换本地文件并立即同步到 SQLite。用户对模型“启用/停用”的选择不会被覆盖。

## Release 资产格式

更新包使用 `schema_version: 2`。仍兼容读取旧的 `schema_version: 1`，旧目录中的模型按 `active` 处理。公共参数可以放在 `profiles` 中，模型通过 `profile` 继承，再按需覆盖：

```json
{
  "schema_version": 2,
  "catalog_version": "2026.07.23.2",
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
      "display_name": "Model Name",
      "lifecycle_status": "active"
    }
  ]
}
```

同一个 `provider_code + model_id + capability` 会稳定映射到同一个目录项。更新包可以调整显示名、输入类型、特性、参数 Schema、默认参数、文档地址和支持级别，但不能在后台改变用户的启用状态。

## 生命周期

- `active`：正常使用。
- `deprecated`：仍可调用，但模型页会提示迁移，不建议用于新项目。
- `retired`：停止发起新任务，历史调用、参数预设和项目引用继续保留。

弃用应当显式写入目录，不能把“没有出现在更新包中”理解为弃用，因为更新包可以只覆盖部分模型：

```json
{
  "provider_code": "example",
  "model_id": "example/legacy",
  "display_name": "Legacy",
  "capability": "text",
  "input_modalities": ["text"],
  "features": ["chat"],
  "parameter_schema": {"type": "object", "properties": {}},
  "default_parameters": {},
  "support_status": "compatible",
  "lifecycle_status": "deprecated",
  "deprecated_at": "2026-07-01T00:00:00Z",
  "sunset_at": "2026-10-01T00:00:00Z",
  "replacement": {
    "provider_code": "example",
    "model_id": "example/current",
    "capability": "text"
  },
  "lifecycle_message": "请迁移到当前型号"
}
```

更新包从清单中删除一个型号不会删除 SQLite 中的目录项。只有显式的 `retired` 会把模型设为不可用于新任务；这样不会破坏历史记录和参数预设。

## 智谱目录

内置目录包含 GLM 文本/视觉理解、GLM Image、GLM TTS/ASR、CogVideoX 与 Vidu 2 系列。`glm-tts` 同时作为音色管理入口；复刻时 SagaFlow 会使用智谱的 `glm-tts-clone` 接口，复刻结果仍由 `glm-tts` 合成。CogVideoX 可以直接读取本机图片数据，Vidu 2 的参考生视频只接受公网 URL，因此使用该型号前需要在资产页手动发布参考图到 S3。

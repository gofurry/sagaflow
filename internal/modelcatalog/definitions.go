package modelcatalog

import (
	"encoding/json"

	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

const manifestVersion = 5

func Builtins() []Definition {
	return []Definition{
		model("20000000-0000-0000-0000-000000000001", "deepseek", "deepseek-v4-flash", "DeepSeek V4 Flash", "text",
			[]string{"text"}, []string{"thinking", "json_output", "1m_context", "fast"}, deepSeekV4Schema(), values("max_tokens", 8192, "thinking", "disabled", "reasoning_effort", "high"),
			"https://api-docs.deepseek.com/quick_start/pricing/"),
		model("20000000-0000-0000-0000-000000000005", "deepseek", "deepseek-v4-pro", "DeepSeek V4 Pro", "text",
			[]string{"text"}, []string{"thinking", "json_output", "1m_context"}, deepSeekV4Schema(), values("max_tokens", 8192, "thinking", "enabled", "reasoning_effort", "high"),
			"https://api-docs.deepseek.com/quick_start/pricing/"),

		model("20000000-0000-0000-0000-000000000007", "minimax", "MiniMax-M2.7", "MiniMax M2.7", "text",
			[]string{"text"}, []string{"reasoning", "chat"}, openAITextSchema(204800), values("max_tokens", 4096, "temperature", 1),
			"https://platform.minimaxi.com/docs/api-reference/text-openai-api"),
		model("20000000-0000-0000-0000-000000000008", "minimax", "MiniMax-M2.7-highspeed", "MiniMax M2.7 Highspeed", "text",
			[]string{"text"}, []string{"reasoning", "chat", "fast"}, openAITextSchema(204800), values("max_tokens", 4096, "temperature", 1),
			"https://platform.minimaxi.com/docs/api-reference/text-openai-api"),
		model("20000000-0000-0000-0000-000000000004", "minimax", "speech-2.8-hd", "MiniMax Speech 2.8 HD", "audio",
			[]string{"text", "audio"}, []string{"speech_generation", "voice_clone"}, miniMaxSpeechSchema(), miniMaxSpeechDefaults(),
			"https://platform.minimaxi.com/docs/api-reference/api-overview"),
		model("20000000-0000-0000-0000-000000000009", "minimax", "speech-2.8-turbo", "MiniMax Speech 2.8 Turbo", "audio",
			[]string{"text", "audio"}, []string{"speech_generation", "voice_clone", "fast"}, miniMaxSpeechSchema(), miniMaxSpeechDefaults(),
			"https://platform.minimaxi.com/docs/api-reference/api-overview"),
		model("20000000-0000-0000-0000-000000000010", "minimax", "image-01", "MiniMax Image 01", "image",
			[]string{"text", "image"}, []string{"image_generation", "character_reference"}, miniMaxImageSchema(false), values("aspect_ratio", "16:9", "response_format", "url", "n", 1),
			"https://platform.minimaxi.com/docs/guides/image-generation"),
		model("20000000-0000-0000-0000-000000000011", "minimax", "image-01-live", "MiniMax Image 01 Live", "image",
			[]string{"text", "image"}, []string{"image_generation", "character_reference", "illustration_styles"}, miniMaxImageSchema(true), values("aspect_ratio", "16:9", "response_format", "url", "n", 1),
			"https://platform.minimaxi.com/docs/guides/image-generation"),
		model("20000000-0000-0000-0000-000000000012", "minimax", "MiniMax-Hailuo-2.3", "MiniMax Hailuo 2.3", "video",
			[]string{"text", "image"}, []string{"video_generation", "first_last_frame", "subject_reference"}, miniMaxVideoSchema(), values("duration", 6, "resolution", "1080P", "reference_mode", "first_frame"),
			"https://platform.minimaxi.com/docs/guides/video-generation"),
		model("20000000-0000-0000-0000-000000000013", "minimax", "MiniMax-Hailuo-2.3-Fast", "MiniMax Hailuo 2.3 Fast", "video",
			[]string{"text", "image"}, []string{"video_generation", "image_to_video", "fast"}, miniMaxVideoSchema(), values("duration", 6, "resolution", "768P", "reference_mode", "first_frame"),
			"https://platform.minimaxi.com/docs/guides/video-generation"),

		model("20000000-0000-0000-0000-000000000014", "volcengine", "doubao-seed-2-0-lite-260215", "Doubao Seed 2.0 Lite", "text",
			[]string{"text", "image"}, []string{"reasoning", "vision", "responses_api"}, arkTextSchema(), values("max_output_tokens", 4096, "thinking", "disabled"),
			"https://www.volcengine.com/docs/82379/1795150"),
		model("20000000-0000-0000-0000-000000000028", "volcengine", "doubao-seed-evolving", "Doubao Seed Evolving", "text",
			[]string{"text"}, []string{"reasoning", "coding", "agent", "rolling_release"}, arkEvolvingSchema(), values("max_tokens", 4096, "temperature", 0.7),
			"https://www.volcengine.com/product/doubao"),
		model("20000000-0000-0000-0000-000000000029", "volcengine", "doubao-seed-2-1-pro-260628", "Doubao Seed 2.1 Pro", "text",
			[]string{"text", "image"}, []string{"reasoning", "vision", "responses_api", "agent"}, arkTextSchema(), values("max_output_tokens", 4096, "thinking", "disabled"),
			"https://www.volcengine.com/product/doubao"),
		model("20000000-0000-0000-0000-000000000030", "volcengine", "doubao-seed-2-1-turbo-260628", "Doubao Seed 2.1 Turbo", "text",
			[]string{"text", "image"}, []string{"reasoning", "vision", "responses_api", "fast"}, arkTextSchema(), values("max_output_tokens", 4096, "thinking", "disabled"),
			"https://www.volcengine.com/product/doubao"),
		model("20000000-0000-0000-0000-000000000031", "volcengine", "doubao-seed-character-260628", "Doubao Seed Character", "text",
			[]string{"text"}, []string{"roleplay", "dialogue", "long_context"}, arkTextSchema(), values("max_output_tokens", 4096, "thinking", "disabled"),
			"https://www.volcengine.com/product/doubao"),
		model("20000000-0000-0000-0000-000000000002", "volcengine", "doubao-seedream-5-0-260128", "Seedream 5.0", "image",
			[]string{"text", "image"}, []string{"image_generation", "image_edit", "multi_reference", "sequential_images"}, seedreamSchema(), values("size", "2K", "seed", -1, "max_images", 1, "watermark", false),
			"https://www.volcengine.com/docs/82379/1795150"),
		model("20000000-0000-0000-0000-000000000032", "volcengine", "doubao-seedream-5-0-lite-260128", "Seedream 5.0 Lite", "image",
			[]string{"text", "image"}, []string{"image_generation", "image_edit", "multi_reference", "sequential_images", "knowledge_grounding"}, seedreamSchema(), values("size", "2K", "seed", -1, "max_images", 1, "watermark", false),
			"https://www.volcengine.com/docs/82379/1829186"),
		model("20000000-0000-0000-0000-000000000033", "volcengine", "doubao-seedream-4-5-251128", "Seedream 4.5", "image",
			[]string{"text", "image"}, []string{"image_generation", "image_edit", "multi_reference", "sequential_images", "4k"}, seedreamSchema(), values("size", "2K", "seed", -1, "max_images", 1, "watermark", false),
			"https://www.volcengine.com/docs/82379/1829186"),
		model("20000000-0000-0000-0000-000000000015", "volcengine", "doubao-seedream-4-0-250828", "Seedream 4.0", "image",
			[]string{"text", "image"}, []string{"image_generation", "image_edit", "multi_reference", "sequential_images"}, seedreamSchema(), values("size", "2K", "seed", -1, "max_images", 1, "watermark", false),
			"https://api.volcengine.com/api-docs/view?action=ImageGenerations&serviceCode=ark&version=2024-01-01"),
		model("20000000-0000-0000-0000-000000000003", "volcengine", "doubao-seedance-2-0-mini-260615", "Seedance 2.0 Mini", "video",
			[]string{"text", "image", "video", "audio"}, []string{"video_generation", "multi_reference", "audio_generation"}, seedanceSchema(), values("ratio", "16:9", "duration", 5, "generate_audio", true, "watermark", false),
			"https://www.volcengine.com/docs/82379/2222480"),
		model("20000000-0000-0000-0000-000000000034", "volcengine", "doubao-seedance-2-0-260128", "Seedance 2.0", "video",
			[]string{"text", "image", "video", "audio"}, []string{"video_generation", "multi_reference", "audio_generation", "1080p"}, seedanceSchema(), values("ratio", "16:9", "duration", 5, "resolution", "720p", "generate_audio", true, "watermark", false),
			"https://www.volcengine.com/docs/82379/2222480"),
		model("20000000-0000-0000-0000-000000000035", "volcengine", "doubao-seedance-2-0-fast-260128", "Seedance 2.0 Fast", "video",
			[]string{"text", "image", "video", "audio"}, []string{"video_generation", "multi_reference", "audio_generation", "fast"}, seedanceSchema(), values("ratio", "16:9", "duration", 5, "resolution", "720p", "generate_audio", true, "watermark", false),
			"https://www.volcengine.com/docs/82379/2222480"),

		model("20000000-0000-0000-0000-000000000016", "aliyun_bailian", "qwen3.7-plus", "Qwen3.7 Plus", "text",
			[]string{"text", "image"}, []string{"reasoning", "vision", "tools", "structured_output", "1m_context"}, bailianTextSchema(), values("max_tokens", 8192, "enable_thinking", true, "temperature", 0.7),
			"https://help.aliyun.com/zh/model-studio/models"),
		model("20000000-0000-0000-0000-000000000017", "aliyun_bailian", "qwen3.6-flash", "Qwen3.6 Flash", "text",
			[]string{"text", "image"}, []string{"reasoning", "vision", "fast"}, bailianTextSchema(), values("max_tokens", 4096, "enable_thinking", false, "temperature", 0.7),
			"https://help.aliyun.com/zh/model-studio/models"),
		model("20000000-0000-0000-0000-000000000018", "aliyun_bailian", "qwen3.7-max", "Qwen3.7 Max", "text",
			[]string{"text", "image"}, []string{"reasoning", "vision", "tools", "structured_output"}, bailianTextSchema(), values("max_tokens", 8192, "enable_thinking", true, "temperature", 0.7),
			"https://help.aliyun.com/zh/model-studio/models"),
		model("20000000-0000-0000-0000-000000000019", "aliyun_bailian", "wan2.7-image-pro", "Wan 2.7 Image Pro", "image",
			[]string{"text", "image"}, []string{"image_generation", "image_edit", "multi_reference", "4k"}, bailianImageSchema(), values("size", "2K", "n", 1, "watermark", false, "thinking_mode", true),
			"https://help.aliyun.com/zh/model-studio/wan-image-generation-api-reference"),
		model("20000000-0000-0000-0000-000000000020", "aliyun_bailian", "wan2.7-image", "Wan 2.7 Image", "image",
			[]string{"text", "image"}, []string{"image_generation", "image_edit", "multi_reference", "fast"}, bailianImageSchema(), values("size", "2K", "n", 1, "watermark", false, "thinking_mode", true),
			"https://help.aliyun.com/zh/model-studio/wan-image-generation-api-reference"),
		model("20000000-0000-0000-0000-000000000021", "aliyun_bailian", "qwen-audio-3.0-tts-plus", "Qwen Audio 3.0 TTS Plus", "audio",
			[]string{"text", "audio"}, []string{"speech_generation", "instruction_control", "multilingual"}, bailianSpeechSchema(), bailianSpeechDefaults("longanlingxin"),
			"https://help.aliyun.com/zh/model-studio/qwen-tts-api"),
		model("20000000-0000-0000-0000-000000000022", "aliyun_bailian", "cosyvoice-v3.5-plus", "CosyVoice 3.5 Plus", "audio",
			[]string{"text", "audio"}, []string{"speech_generation", "voice_clone", "voice_design", "instruction_control", "multilingual"}, bailianSpeechSchema(), bailianSpeechDefaults(""),
			"https://help.aliyun.com/zh/model-studio/cosyvoice-api"),
		model("20000000-0000-0000-0000-000000000023", "aliyun_bailian", "happyhorse-1.1-t2v", "HappyHorse 1.1 Text to Video", "video",
			[]string{"text"}, []string{"video_generation", "text_to_video", "native_audio"}, bailianVideoSchema(), values("resolution", "720P", "ratio", "16:9", "duration", 5, "watermark", false, "prompt_extend", true),
			"https://help.aliyun.com/zh/model-studio/happyhorse-api"),
		model("20000000-0000-0000-0000-000000000024", "aliyun_bailian", "happyhorse-1.1-i2v", "HappyHorse 1.1 Image to Video", "video",
			[]string{"text", "image"}, []string{"video_generation", "first_frame", "native_audio"}, bailianVideoSchema(), values("resolution", "720P", "ratio", "16:9", "duration", 5, "watermark", false, "prompt_extend", true),
			"https://help.aliyun.com/zh/model-studio/happyhorse-api"),
		model("20000000-0000-0000-0000-000000000025", "aliyun_bailian", "happyhorse-1.1-r2v", "HappyHorse 1.1 Reference to Video", "video",
			[]string{"text", "image"}, []string{"video_generation", "multi_reference", "native_audio"}, bailianVideoSchema(), values("resolution", "720P", "ratio", "16:9", "duration", 5, "watermark", false, "prompt_extend", true),
			"https://help.aliyun.com/zh/model-studio/happyhorse-api"),
		model("20000000-0000-0000-0000-000000000026", "aliyun_bailian", "wan2.7-i2v-2026-04-25", "Wan 2.7 Image to Video", "video",
			[]string{"text", "image", "video", "audio"}, []string{"video_generation", "first_last_frame", "video_continuation", "audio_driven"}, bailianVideoSchema(), values("resolution", "720P", "ratio", "16:9", "duration", 5, "watermark", false, "prompt_extend", true),
			"https://help.aliyun.com/zh/model-studio/wan-video-generation-api-reference"),
		model("20000000-0000-0000-0000-000000000027", "aliyun_bailian", "wan2.7-r2v-2026-06-12", "Wan 2.7 Reference to Video", "video",
			[]string{"text", "image", "video", "audio"}, []string{"video_generation", "multi_reference", "audio_driven"}, bailianVideoSchema(), values("resolution", "720P", "ratio", "16:9", "duration", 5, "watermark", false, "prompt_extend", true),
			"https://help.aliyun.com/zh/model-studio/wan-video-generation-api-reference"),
	}
}

func model(id, provider, modelID, displayName, capability string, inputs, features []string, schema, defaults json.RawMessage, docs string) Definition {
	return Definition{
		ProviderCode: provider,
		Model: db.Model{
			ID: uuid.MustParse(id), ModelID: modelID, DisplayName: displayName, Capability: capability,
			InputModalities: inputs, Features: features, ParameterSchema: schema, DefaultParameters: defaults,
			Enabled: true, Available: true,
			Metadata: db.JSON(map[string]any{
				"source": "builtin", "support_status": "verified", "manifest_version": manifestVersion, "documentation_url": docs,
			}),
		},
	}
}

func values(pairs ...any) json.RawMessage {
	value := make(map[string]any, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		key, _ := pairs[index].(string)
		value[key] = pairs[index+1]
	}
	return db.JSON(value)
}

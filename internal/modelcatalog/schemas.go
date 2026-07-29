package modelcatalog

import (
	"encoding/json"

	"github.com/gofurry/sagaflow/internal/store/db"
)

func schema(properties map[string]any) json.RawMessage {
	return db.JSON(map[string]any{"type": "object", "properties": properties})
}

func number(title string, minimum, maximum float64) map[string]any {
	return map[string]any{"type": "number", "title": title, "minimum": minimum, "maximum": maximum}
}

func integer(title string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "title": title, "minimum": minimum, "maximum": maximum}
}

func choice(title string, choices ...string) map[string]any {
	return map[string]any{"type": "string", "title": title, "enum": choices}
}

func integerChoice(title string, choices ...int) map[string]any {
	return map[string]any{"type": "integer", "title": title, "enum": choices}
}

func boolean(title string) map[string]any {
	return map[string]any{"type": "boolean", "title": title}
}

func deepSeekV4Schema() json.RawMessage {
	return schema(map[string]any{
		"max_tokens":        integer("最大输出 Token", 1, 393216),
		"thinking":          choice("思考模式", "enabled", "disabled"),
		"reasoning_effort":  choice("思考强度", "high", "max"),
		"temperature":       number("温度", 0, 2),
		"top_p":             number("Top P", 0, 1),
		"frequency_penalty": number("频率惩罚", -2, 2),
		"presence_penalty":  number("存在惩罚", -2, 2),
		"response_format":   choice("输出格式", "text", "json_object"),
		"stop":              map[string]any{"type": "array", "title": "停止序列", "items": map[string]any{"type": "string"}},
	})
}

func openAITextSchema(contextLength int) json.RawMessage {
	return schema(map[string]any{
		"max_tokens":      integer("最大输出 Token", 1, 32768),
		"temperature":     number("温度", 0, 2),
		"top_p":           number("Top P", 0, 1),
		"reasoning_split": boolean("分离思考内容"),
		"context_length":  map[string]any{"type": "integer", "title": "模型上下文", "readOnly": true, "default": contextLength},
	})
}

func miniMaxSpeechSchema() json.RawMessage {
	return schema(map[string]any{
		"voice_id":              map[string]any{"type": "string", "title": "音色 ID"},
		"speed":                 number("语速", .5, 2),
		"volume":                number("音量", 0, 10),
		"pitch":                 integer("音高", -12, 12),
		"emotion":               choice("情绪", "calm", "happy", "sad", "angry", "fearful", "disgusted", "surprised"),
		"language_boost":        map[string]any{"type": "string", "title": "语言增强"},
		"sample_rate":           integerChoice("采样率", 16000, 24000, 32000, 44100),
		"bitrate":               integerChoice("比特率", 64000, 128000, 256000),
		"format":                choice("音频格式", "mp3", "wav", "flac", "pcm"),
		"channel":               integer("声道数", 1, 2),
		"subtitle_enable":       boolean("生成字幕"),
		"aigc_watermark":        boolean("音频水印"),
		"english_normalization": boolean("英文规范化"),
	})
}

func miniMaxSpeechDefaults() json.RawMessage {
	return values(
		"voice_id", "Chinese (Mandarin)_Lyrical_Voice", "speed", 1, "volume", 1, "pitch", 0,
		"emotion", "calm", "language_boost", "auto", "sample_rate", 32000, "bitrate", 128000,
		"format", "mp3", "channel", 1, "subtitle_enable", false, "aigc_watermark", false,
	)
}

func miniMaxImageSchema(live bool) json.RawMessage {
	properties := map[string]any{
		"aspect_ratio":    choice("画面比例", "1:1", "16:9", "4:3", "3:2", "2:3", "3:4", "9:16", "21:9"),
		"width":           integer("宽度", 512, 2048),
		"height":          integer("高度", 512, 2048),
		"n":               integer("生成数量", 1, 4),
		"response_format": choice("返回格式", "url", "base64"),
	}
	if live {
		properties["style"] = map[string]any{"type": "string", "title": "画面风格"}
	}
	return schema(properties)
}

func miniMaxVideoSchema() json.RawMessage {
	return schema(map[string]any{
		"duration":         integerChoice("时长", 6, 10),
		"resolution":       choice("分辨率", "768P", "1080P"),
		"reference_mode":   choice("参考方式", "first_frame", "first_last_frame", "subject"),
		"prompt_optimizer": boolean("Prompt 优化"),
	})
}

func arkTextSchema() json.RawMessage {
	return schema(map[string]any{
		"max_output_tokens": integer("最大输出 Token", 1, 32768),
		"temperature":       number("温度", 0, 2),
		"top_p":             number("Top P", 0, 1),
		"thinking":          choice("深度思考", "enabled", "disabled", "auto"),
	})
}

func arkEvolvingSchema() json.RawMessage {
	return schema(map[string]any{
		"max_tokens":        integer("最大输出 Token", 1, 32768),
		"temperature":       number("温度", 0, 2),
		"top_p":             number("Top P", 0, 1),
		"frequency_penalty": number("频率惩罚", -2, 2),
		"presence_penalty":  number("存在惩罚", -2, 2),
		"response_format":   choice("输出格式", "text", "json_object"),
	})
}

func seedreamSchema() json.RawMessage {
	return schema(map[string]any{
		"size":                        map[string]any{"type": "string", "title": "图像尺寸"},
		"seed":                        integer("随机种子", -1, 2147483647),
		"guidance_scale":              number("Prompt 一致程度", 1, 10),
		"watermark":                   boolean("添加水印"),
		"response_format":             choice("返回格式", "url", "b64_json"),
		"sequential_image_generation": choice("组图生成", "disabled", "auto"),
		"max_images":                  integer("组图最大数量", 1, 15),
	})
}

func seedanceSchema() json.RawMessage {
	return schema(map[string]any{
		"ratio":          choice("画面比例", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"),
		"duration":       integer("时长（秒）", 2, 15),
		"resolution":     choice("分辨率", "480p", "720p", "1080p"),
		"generate_audio": boolean("生成声音"),
		"watermark":      boolean("添加水印"),
	})
}

func bailianTextSchema() json.RawMessage {
	return schema(map[string]any{
		"max_tokens":        integer("最大输出 Token", 1, 65536),
		"temperature":       number("温度", 0, 2),
		"top_p":             number("Top P", 0, 1),
		"enable_thinking":   boolean("深度思考"),
		"thinking_budget":   integer("思考 Token 上限", 0, 65536),
		"frequency_penalty": number("频率惩罚", -2, 2),
		"presence_penalty":  number("存在惩罚", -2, 2),
		"response_format":   choice("输出格式", "text", "json_object"),
		"seed":              integer("随机种子", 0, 2147483647),
	})
}

func bailianImageSchema() json.RawMessage {
	return schema(map[string]any{
		"size":            choice("图像尺寸", "1K", "2K", "4K"),
		"n":               integer("生成数量", 1, 4),
		"seed":            integer("随机种子", 0, 2147483647),
		"watermark":       boolean("添加水印"),
		"thinking_mode":   boolean("智能构图"),
		"prompt_extend":   boolean("Prompt 优化"),
		"negative_prompt": map[string]any{"type": "string", "title": "负面 Prompt"},
	})
}

func bailianImageEditSchema() json.RawMessage {
	return schema(map[string]any{
		"n":         integer("生成数量", 1, 4),
		"seed":      integer("随机种子", 0, 2147483647),
		"watermark": boolean("添加水印"),
	})
}

func bailianSpeechSchema() json.RawMessage {
	return schema(map[string]any{
		"voice":       map[string]any{"type": "string", "title": "音色 ID"},
		"instruction": map[string]any{"type": "string", "title": "语气指令"},
		"format":      choice("音频格式", "mp3", "wav", "pcm", "opus"),
		"sample_rate": integerChoice("采样率", 8000, 16000, 22050, 24000, 44100, 48000),
		"volume":      number("音量", 0, 100),
		"rate":        number("语速", .5, 2),
		"pitch":       number("音高", .5, 2),
	})
}

func bailianSpeechDefaults(voice string) json.RawMessage {
	defaults := map[string]any{"format": "mp3", "sample_rate": 24000, "volume": 50, "rate": 1, "pitch": 1}
	if voice != "" {
		defaults["voice"] = voice
	}
	return db.JSON(defaults)
}

func bailianVideoSchema() json.RawMessage {
	return schema(map[string]any{
		"resolution":      choice("分辨率", "720P", "1080P"),
		"ratio":           choice("画面比例", "16:9", "9:16", "1:1", "4:3", "3:4"),
		"duration":        integer("时长（秒）", 3, 15),
		"seed":            integer("随机种子", 0, 2147483647),
		"watermark":       boolean("添加水印"),
		"prompt_extend":   boolean("Prompt 优化"),
		"negative_prompt": map[string]any{"type": "string", "title": "负面 Prompt"},
	})
}

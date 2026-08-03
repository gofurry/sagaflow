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

func describedChoice(title, description string, choices ...string) map[string]any {
	property := choice(title, choices...)
	property["description"] = description
	return property
}

func stringSuggestions(title, description, pattern string, examples ...string) map[string]any {
	property := map[string]any{"type": "string", "title": title, "description": description, "examples": examples}
	if pattern != "" {
		property["pattern"] = pattern
	}
	return property
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
		"aspect_ratio":     choice("画面比例", "1:1", "16:9", "4:3", "3:2", "2:3", "3:4", "9:16"),
		"n":                integer("生成数量", 1, 9),
		"response_format":  choice("返回格式", "url", "base64"),
		"seed":             integer("随机种子", 0, 2147483647),
		"prompt_optimizer": boolean("Prompt 优化"),
		"aigc_watermark":   boolean("添加水印"),
	}
	if live {
		properties["style"] = map[string]any{"type": "object", "title": "画面风格", "description": "Image 01 Live 的风格对象，按 MiniMax 文档填写 JSON。"}
	} else {
		properties["aspect_ratio"] = choice("画面比例", "1:1", "16:9", "4:3", "3:2", "2:3", "3:4", "9:16", "21:9")
		properties["width"] = map[string]any{"type": "integer", "title": "宽度", "minimum": 512, "maximum": 2048, "multipleOf": 8, "description": "自定义尺寸仅 Image 01 支持；宽高需同时填写，且会被画面比例覆盖。"}
		properties["height"] = map[string]any{"type": "integer", "title": "高度", "minimum": 512, "maximum": 2048, "multipleOf": 8, "description": "512–2048，必须为 8 的倍数。"}
	}
	return schema(properties)
}

func miniMaxVideoSchema(fast bool) json.RawMessage {
	referenceModes := describedChoice("参考方式", "1080P 仅支持 6 秒；768P 支持 6 或 10 秒。", "first_frame", "first_last_frame", "subject")
	if fast {
		referenceModes = describedChoice("参考方式", "Fast 版仅支持首帧图生视频。", "first_frame")
	}
	return schema(map[string]any{
		"duration":         integerChoice("时长", 6, 10),
		"resolution":       describedChoice("分辨率", "1080P 仅支持 6 秒；10 秒请选择 768P。", "768P", "1080P"),
		"reference_mode":   referenceModes,
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
		"size":                        stringSuggestions("图像尺寸", "可选择 1K / 2K / 4K，也可输入模型支持的 宽x高；具体边长和总像素限制取决于 Seedream 版本。", `^(1K|2K|4K|[1-9][0-9]{2,4}x[1-9][0-9]{2,4})$`, "1K", "2K", "4K", "2048x2048", "2560x1440", "1440x2560"),
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
		"resolution":     describedChoice("分辨率", "火山方舟视频生成接口当前直出 480p / 720p / 1080p；2K、4K 并非该接口的可选直出档位。", "480p", "720p", "1080p"),
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

func bailianImageSchema(pro bool) json.RawMessage {
	examples := []string{"1K", "2K", "2048*2048", "2560*1440", "1440*2560"}
	description := "可选择 1K / 2K，也可输入模型支持的 宽*高 自定义尺寸。"
	pattern := `^(1K|2K|[1-9][0-9]{2,4}\*[1-9][0-9]{2,4})$`
	if pro {
		examples = append([]string{"1K", "2K", "4K"}, examples[2:]...)
		description = "可选择 1K / 2K / 4K，也可输入模型支持的 宽*高 自定义尺寸；编辑场景最高通常为 2K。"
		pattern = `^(1K|2K|4K|[1-9][0-9]{2,4}\*[1-9][0-9]{2,4})$`
	}
	return schema(map[string]any{
		"size":          stringSuggestions("图像尺寸", description, pattern, examples...),
		"n":             integer("生成数量", 1, 4),
		"seed":          integer("随机种子", 0, 2147483647),
		"watermark":     boolean("添加水印"),
		"thinking_mode": boolean("智能构图"),
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

func bailianVideoSchema(mode string) json.RawMessage {
	properties := map[string]any{
		"resolution": choice("分辨率", "720P", "1080P"),
		"seed":       integer("随机种子", 0, 2147483647),
		"watermark":  boolean("添加水印"),
	}
	switch mode {
	case "happyhorse-t2v":
		properties["ratio"] = choice("画面比例", "16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9")
		properties["duration"] = integer("时长（秒）", 3, 15)
	case "happyhorse-i2v", "happyhorse-r2v":
		properties["duration"] = integer("时长（秒）", 3, 15)
	case "wan-i2v":
		properties["duration"] = integer("时长（秒）", 2, 15)
		properties["prompt_extend"] = boolean("Prompt 优化")
		properties["negative_prompt"] = map[string]any{"type": "string", "title": "负面 Prompt"}
	case "wan-r2v":
		properties["ratio"] = describedChoice("画面比例", "未提供首帧时生效；提供首帧后会沿用参考素材比例。", "16:9", "9:16", "1:1", "4:3", "3:4")
		properties["duration"] = map[string]any{"type": "integer", "title": "时长（秒）", "minimum": 2, "maximum": 15, "description": "含视频参考时厂商上限为 10 秒，否则最多 15 秒。"}
		properties["prompt_extend"] = boolean("Prompt 优化")
		properties["negative_prompt"] = map[string]any{"type": "string", "title": "负面 Prompt"}
	}
	return schema(properties)
}

// Package providercatalog keeps the official help and credential entry points
// for cloud providers supported by SagaFlow.
package providercatalog

type Provider struct {
	Code          string
	DisplayName   string
	CredentialURL string
	DocsURL       string
}

var providers = []Provider{
	{
		Code: "deepseek", DisplayName: "DeepSeek",
		CredentialURL: "https://platform.deepseek.com/api_keys",
		DocsURL:       "https://api-docs.deepseek.com/",
	},
	{
		Code: "volcengine", DisplayName: "火山方舟",
		CredentialURL: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey",
		DocsURL:       "https://www.volcengine.com/docs/82379",
	},
	{
		Code: "minimax", DisplayName: "MiniMax",
		CredentialURL: "https://platform.minimaxi.com/console/access?tab=api-keys",
		DocsURL:       "https://platform.minimaxi.com/docs/api-reference/api-overview",
	},
	{
		Code: "aliyun_bailian", DisplayName: "阿里云百炼",
		CredentialURL: "https://bailian.console.aliyun.com/?apiKey=1#/api-key",
		DocsURL:       "https://help.aliyun.com/zh/model-studio/get-api-key",
	},
	{
		Code: "siliconflow", DisplayName: "硅基流动",
		CredentialURL: "https://cloud.siliconflow.cn/account/ak",
		DocsURL:       "https://docs.siliconflow.cn/",
	},
	{
		Code: "zhipu", DisplayName: "智谱开放平台",
		CredentialURL: "https://bigmodel.cn/usercenter/proj-mgmt/apikeys",
		DocsURL:       "https://docs.bigmodel.cn/",
	},
	{
		Code: "tencent_tokenhub", DisplayName: "腾讯云 TokenHub",
		CredentialURL: "https://console.cloud.tencent.com/tokenhub/apikey?regionId=1",
		DocsURL:       "https://cloud.tencent.com/document/product/1823/130090",
	},
	{
		Code: "moonshot", DisplayName: "Kimi · Moonshot",
		CredentialURL: "https://platform.kimi.com/console/api-keys",
		DocsURL:       "https://platform.kimi.com/docs/",
	},
}

func All() []Provider {
	result := make([]Provider, len(providers))
	copy(result, providers)
	return result
}

func SupportedCodes() map[string]struct{} {
	result := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		result[provider.Code] = struct{}{}
	}
	return result
}

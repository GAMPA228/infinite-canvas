package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func PublicSettings() (model.PublicSetting, error) {
	return PublicSettingsForUser(model.AuthUser{})
}

func PublicSettingsForUser(user model.AuthUser) (model.PublicSetting, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.PublicSetting{}, err
	}
	public := normalizePublicSetting(settings.Public)
	if user.ID == "" || user.Role != model.UserRoleVIP {
		return public, nil
	}
	savedUser, ok, err := repository.GetUserByID(user.ID)
	if err != nil {
		return model.PublicSetting{}, err
	}
	if !ok || strings.TrimSpace(savedUser.ChannelName) == "" {
		return public, nil
	}
	for _, channel := range normalizePrivateSetting(settings.Private).Channels {
		if channel.Enabled && channel.Name == savedUser.ChannelName {
			applyUserChannelModels(&public.ModelChannel, channel.Models)
			return public, nil
		}
	}
	return public, nil
}

func AdminSettings() (model.Settings, error) {
	settings, err := repository.GetSettings()
	return hidePrivateAPIKeys(normalizeSettings(settings)), err
}

func SaveSettings(settings model.Settings) (model.Settings, error) {
	saved, err := repository.GetSettings()
	if err != nil {
		return model.Settings{}, err
	}
	settings = normalizeSettings(settings)
	keepPrivateAPIKeys(&settings, normalizeSettings(saved))
	result, err := repository.SaveSettings(settings, now())
	if err == nil {
		RefreshPromptSyncScheduler()
	}
	return hidePrivateAPIKeys(result), err
}

func AdminChannelModels(index *int, channel model.ModelChannel) ([]string, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return nil, err
	}
	return fetchAdminChannelModels(resolved)
}

func AdminTestChannelModel(index *int, channel model.ModelChannel, modelName string) (string, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return "", err
	}
	if isArkAgentPlanChannel(resolved) || isSeedanceModelName(modelName) {
		return testArkSeedanceChannelModel(resolved, modelName)
	}
	return testAdminChannelModel(resolved, modelName)
}

func normalizeSettings(settings model.Settings) model.Settings {
	settings.Public = normalizePublicSetting(settings.Public)
	settings.Private = normalizePrivateSetting(settings.Private)
	return settings
}

func normalizePublicSetting(setting model.PublicSetting) model.PublicSetting {
	if setting.ModelChannel.AvailableModels == nil {
		setting.ModelChannel.AvailableModels = []string{}
	}
	if setting.ModelChannel.ModelCosts == nil {
		setting.ModelChannel.ModelCosts = []model.ModelCost{}
	}
	for i := range setting.ModelChannel.ModelCosts {
		setting.ModelChannel.ModelCosts[i].Model = strings.TrimSpace(setting.ModelChannel.ModelCosts[i].Model)
		if setting.ModelChannel.ModelCosts[i].Credits < 0 {
			setting.ModelChannel.ModelCosts[i].Credits = 0
		}
		if setting.ModelChannel.ModelCosts[i].ImageCredits1K < 0 {
			setting.ModelChannel.ModelCosts[i].ImageCredits1K = 0
		}
		if setting.ModelChannel.ModelCosts[i].ImageCredits2K < 0 {
			setting.ModelChannel.ModelCosts[i].ImageCredits2K = 0
		}
		if setting.ModelChannel.ModelCosts[i].ImageCredits4K < 0 {
			setting.ModelChannel.ModelCosts[i].ImageCredits4K = 0
		}
		if setting.ModelChannel.ModelCosts[i].Credits > 0 {
			if setting.ModelChannel.ModelCosts[i].ImageCredits1K <= 0 {
				setting.ModelChannel.ModelCosts[i].ImageCredits1K = setting.ModelChannel.ModelCosts[i].Credits
			}
			if setting.ModelChannel.ModelCosts[i].ImageCredits2K <= 0 {
				setting.ModelChannel.ModelCosts[i].ImageCredits2K = setting.ModelChannel.ModelCosts[i].Credits
			}
			if setting.ModelChannel.ModelCosts[i].ImageCredits4K <= 0 {
				setting.ModelChannel.ModelCosts[i].ImageCredits4K = setting.ModelChannel.ModelCosts[i].Credits
			}
		}
	}
	if setting.ModelChannel.AllowCustomChannel == nil {
		enabled := true
		setting.ModelChannel.AllowCustomChannel = &enabled
	}
	return setting
}

func ModelCost(modelName string) (int, error) {
	return ModelCostByTier(modelName, "")
}

func ModelCostByTier(modelName string, tier string) (int, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return 0, err
	}
	modelName = strings.TrimSpace(modelName)
	for _, item := range normalizePublicSetting(settings.Public).ModelChannel.ModelCosts {
		if item.Model == modelName {
			switch strings.ToLower(strings.TrimSpace(tier)) {
			case "1k":
				return modelCostByTier(item.ImageCredits1K, 1), nil
			case "2k":
				return modelCostByTier(item.ImageCredits2K, 2), nil
			case "4k":
				return modelCostByTier(item.ImageCredits4K, 3), nil
			}
			return item.Credits, nil
		}
	}
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "1k":
		return 1, nil
	case "2k":
		return 2, nil
	case "4k":
		return 3, nil
	}
	return 0, nil
}

func modelCostByTier(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func normalizePrivateSetting(setting model.PrivateSetting) model.PrivateSetting {
	if setting.Channels == nil {
		setting.Channels = []model.ModelChannel{}
	}
	setting.PromptSync = normalizePromptSyncSetting(setting.PromptSync)
	for i := range setting.Channels {
		if setting.Channels[i].Protocol == "" {
			setting.Channels[i].Protocol = "openai"
		}
		if setting.Channels[i].Models == nil {
			setting.Channels[i].Models = []string{}
		}
		if setting.Channels[i].Mode == "" {
			setting.Channels[i].Mode = "openai"
		}
		if setting.Channels[i].SizeStrategy == "" {
			setting.Channels[i].SizeStrategy = "exact"
		}
		if setting.Channels[i].Weight <= 0 {
			setting.Channels[i].Weight = 1
		}
	}
	return setting
}

func hidePrivateAPIKeys(settings model.Settings) model.Settings {
	for i := range settings.Private.Channels {
		settings.Private.Channels[i].APIKey = ""
	}
	return settings
}

func keepPrivateAPIKeys(settings *model.Settings, saved model.Settings) {
	for i := range settings.Private.Channels {
		if strings.TrimSpace(settings.Private.Channels[i].APIKey) != "" {
			continue
		}
		if channel, ok := findSavedChannel(settings.Private.Channels[i], saved.Private.Channels, i); ok {
			settings.Private.Channels[i].APIKey = channel.APIKey
		}
	}
}

func findSavedChannel(channel model.ModelChannel, saved []model.ModelChannel, index int) (model.ModelChannel, bool) {
	for _, item := range saved {
		if item.Name == channel.Name && item.BaseURL == channel.BaseURL {
			return item, true
		}
	}
	if index < len(saved) {
		return saved[index], true
	}
	return model.ModelChannel{}, false
}

func SelectModelChannel(modelName string) (model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelChannel{}, err
	}
	channels := modelChannelsForModel(normalizePrivateSetting(settings.Private).Channels, modelName)
	if len(channels) == 0 {
		return model.ModelChannel{}, errors.New("没有可用模型渠道")
	}
	total := 0
	for _, channel := range channels {
		total += channel.Weight
	}
	hit := rand.Intn(total)
	for _, channel := range channels {
		hit -= channel.Weight
		if hit < 0 {
			return channel, nil
		}
	}
	return channels[0], nil
}

func applyUserChannelModels(setting *model.PublicModelChannelSetting, models []string) {
	nextModels := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || seen[modelName] {
			continue
		}
		nextModels = append(nextModels, modelName)
		seen[modelName] = true
	}
	if len(nextModels) == 0 {
		return
	}
	setting.AvailableModels = nextModels
	if !stringInSlice(setting.DefaultModel, nextModels) {
		setting.DefaultModel = nextModels[0]
	}
	if !stringInSlice(setting.DefaultImageModel, nextModels) {
		setting.DefaultImageModel = firstModelByKeyword(nextModels, []string{"image", "img", "gpt-image"}, setting.DefaultModel)
	}
	if !stringInSlice(setting.DefaultVideoModel, nextModels) {
		setting.DefaultVideoModel = firstModelByKeyword(nextModels, []string{"sora", "video"}, setting.DefaultModel)
	}
	if !stringInSlice(setting.DefaultTextModel, nextModels) {
		setting.DefaultTextModel = firstModelByKeyword(nextModels, []string{"gpt", "claude", "gemini", "codex"}, setting.DefaultModel)
	}
}

func firstModelByKeyword(models []string, keywords []string, fallback string) string {
	for _, keyword := range keywords {
		for _, modelName := range models {
			if strings.Contains(strings.ToLower(modelName), keyword) {
				return modelName
			}
		}
	}
	if stringInSlice(fallback, models) {
		return fallback
	}
	return models[0]
}

func stringInSlice(value string, items []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func SelectUserModelChannel(user model.AuthUser, modelName string) (model.ModelChannel, error) {
	if user.ID == "" || user.Role != model.UserRoleVIP {
		return SelectModelChannel(modelName)
	}
	savedUser, ok, err := repository.GetUserByID(user.ID)
	if err != nil {
		return model.ModelChannel{}, err
	}
	if !ok || strings.TrimSpace(savedUser.ChannelName) == "" {
		return SelectModelChannel(modelName)
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelChannel{}, err
	}
	for _, channel := range normalizePrivateSetting(settings.Private).Channels {
		if channel.Enabled && channel.Name == savedUser.ChannelName {
			if !channelSupportsModel(channel, modelName) {
				return model.ModelChannel{}, settingsSafeMessageError{message: "专属渠道不支持当前模型"}
			}
			return channel, nil
		}
	}
	return model.ModelChannel{}, settingsSafeMessageError{message: "专属渠道不可用"}
}

func BuildModelChannelURL(channel model.ModelChannel, path string) string {
	baseURL := normalizeModelChannelBaseURL(channel.BaseURL)
	lowerBaseURL := strings.ToLower(baseURL)
	if !strings.HasSuffix(lowerBaseURL, "/v1") && !strings.HasSuffix(lowerBaseURL, "/api/v3") && !strings.HasSuffix(lowerBaseURL, "/api/plan/v3") {
		baseURL += "/v1"
	}
	return baseURL + path
}

func normalizeModelChannelBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		path := strings.TrimRight(parsed.Path, "/")
		lowerPath := strings.ToLower(path)
		if index := strings.Index(lowerPath, "/api/plan/v3"); index >= 0 {
			end := index + len("/api/plan/v3")
			if len(lowerPath) == end || lowerPath[end] == '/' {
				parsed.Path = path[:end]
				parsed.RawPath = ""
				parsed.RawQuery = ""
				parsed.Fragment = ""
				return strings.TrimRight(parsed.String(), "/")
			}
		}
	}
	return baseURL
}

func isArkAgentPlanChannel(channel model.ModelChannel) bool {
	baseURL := strings.ToLower(normalizeModelChannelBaseURL(channel.BaseURL))
	return strings.HasSuffix(baseURL, "/api/plan/v3")
}

func isSeedanceModelName(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(modelName, "seedance") || strings.Contains(modelName, "doubao-seedance")
}

func normalizeModelChannel(channel model.ModelChannel) model.ModelChannel {
	if channel.Protocol == "" {
		channel.Protocol = "openai"
	}
	if channel.Models == nil {
		channel.Models = []string{}
	}
	if channel.Mode == "" {
		channel.Mode = "openai"
	}
	if channel.SizeStrategy == "" {
		channel.SizeStrategy = "exact"
	}
	if channel.Weight <= 0 {
		channel.Weight = 1
	}
	return channel
}

func resolveAdminChannel(index *int, channel model.ModelChannel) (model.ModelChannel, error) {
	resolved := normalizeModelChannel(channel)
	if strings.TrimSpace(resolved.APIKey) == "" {
		settings, err := repository.GetSettings()
		if err != nil {
			return model.ModelChannel{}, err
		}
		saved := normalizePrivateSetting(settings.Private).Channels
		if index != nil && *index >= 0 && *index < len(saved) {
			if resolved.APIKey == "" {
				resolved.APIKey = saved[*index].APIKey
			}
			if resolved.BaseURL == "" {
				resolved.BaseURL = saved[*index].BaseURL
			}
			if resolved.Name == "" {
				resolved.Name = saved[*index].Name
			}
		}
		if resolved.APIKey == "" {
			if savedChannel, ok := findSavedChannel(resolved, saved, -1); ok {
				resolved.APIKey = savedChannel.APIKey
			}
		}
	}
	if strings.TrimSpace(resolved.BaseURL) == "" {
		return model.ModelChannel{}, settingsSafeMessageError{message: "缺少接口地址"}
	}
	if strings.TrimSpace(resolved.APIKey) == "" {
		return model.ModelChannel{}, settingsSafeMessageError{message: "缺少 API Key"}
	}
	return resolved, nil
}

func fetchAdminChannelModels(channel model.ModelChannel) ([]string, error) {
	request, err := http.NewRequest(http.MethodGet, BuildModelChannelURL(channel, "/models"), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		return nil, readAdminChannelError(body, response.StatusCode, "读取模型失败")
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &payload)
	result := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) != "" {
			result = append(result, item.ID)
		}
	}
	sort.Strings(result)
	return result, nil
}

func testAdminChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	body, _ := json.Marshal(map[string]any{
		"model": modelName,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "hi",
		}},
	})
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		return "", readAdminChannelError(responseBody, response.StatusCode, "测试失败")
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	if len(payload.Choices) > 0 && strings.TrimSpace(payload.Choices[0].Message.Content) != "" {
		return payload.Choices[0].Message.Content, nil
	}
	return "ok", nil
}

func testArkSeedanceChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	if strings.TrimSpace(channel.BaseURL) == "" {
		return "", settingsSafeMessageError{message: "缺少接口地址"}
	}
	if strings.TrimSpace(channel.APIKey) == "" {
		return "", settingsSafeMessageError{message: "缺少 API Key"}
	}
	if !isArkAgentPlanChannel(channel) {
		return "Seedance 视频模型不会发送 /chat/completions 文本测试。已检查 Base URL、API Key 和模型名非空；未调用视频生成接口，因此未验证套餐额度或模型权限。", nil
	}
	return "Agent Plan / Seedance 视频模型配置格式已通过。后台测试不会调用视频生成接口，因此未验证 API Key、套餐额度或模型权限；请在画布中使用视频生成验证。", nil
}

func readAdminChannelError(body []byte, statusCode int, fallback string) error {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Msg string `json:"msg"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return settingsSafeMessageError{message: payload.Error.Message}
		}
		if strings.TrimSpace(payload.Msg) != "" {
			return settingsSafeMessageError{message: payload.Msg}
		}
	}
	if statusCode == http.StatusUnauthorized {
		return settingsSafeMessageError{message: "上游接口认证失败（401），请检查 API Key"}
	}
	if statusCode > 0 {
		return settingsSafeMessageError{message: fmt.Sprintf("%s：%d", fallback, statusCode)}
	}
	return settingsSafeMessageError{message: fallback}
}

type settingsSafeMessageError struct {
	message string
}

func (err settingsSafeMessageError) Error() string {
	return err.message
}

func (err settingsSafeMessageError) SafeMessage() string {
	return err.message
}

func modelChannelsForModel(channels []model.ModelChannel, modelName string) []model.ModelChannel {
	result := []model.ModelChannel{}
	for _, channel := range channels {
		if !channel.Enabled || channel.BaseURL == "" || channel.APIKey == "" {
			continue
		}
		if channelSupportsModel(channel, modelName) {
			result = append(result, channel)
		}
	}
	return result
}

func channelSupportsModel(channel model.ModelChannel, modelName string) bool {
	for _, item := range channel.Models {
		if strings.TrimSpace(item) == modelName {
			return true
		}
	}
	return false
}

package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/service"
)

func AIImagesGenerations(w http.ResponseWriter, r *http.Request) {
	if !canUseImageQuality(r) {
		Fail(w, "请升级用户套餐")
		return
	}
	proxyAIRequest(w, r, "/images/generations")
}

func AIImagesEdits(w http.ResponseWriter, r *http.Request) {
	if !canUseImageQuality(r) {
		Fail(w, "请升级用户套餐")
		return
	}
	proxyAIRequest(w, r, "/images/edits")
}

func AIChatCompletions(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/chat/completions")
}

func AIResponses(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/responses")
}

func AIVideos(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/videos")
}

func AIVideo(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id)
}

func AIVideoContent(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id+"/content")
}

func proxyAIGetRequest(w http.ResponseWriter, r *http.Request, path string) {
	modelName := r.URL.Query().Get("model")
	if strings.TrimSpace(modelName) == "" {
		modelName = "sora-2"
	}
	user, _ := service.UserFromContext(r.Context())
	channel, err := service.SelectUserModelChannel(user, modelName)
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		FailError(w, err)
		return
	}
	request, err := http.NewRequest(http.MethodGet, service.BuildModelChannelURL(channel, path), nil)
	if err != nil {
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	copyAIResponse(w, request, nil)
}

func proxyAIRequest(w http.ResponseWriter, r *http.Request, path string) {
	body, contentType, modelName, err := readAIRequest(r)
	if err != nil {
		log.Printf("AI proxy request read failed: %v", err)
		Fail(w, "AI 接口请求失败")
		return
	}
	user, _ := service.UserFromContext(r.Context())
	credits, err := service.ModelCostByTier(modelName, aiCostTier(path, body, contentType))
	if err != nil {
		log.Printf("AI proxy read model cost failed: model=%s err=%v", modelName, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	credits *= readAIRequestCount(body, contentType)
	if credits > 0 && user.ID == "" {
		Fail(w, "未登录或权限不足")
		return
	}
	channel, err := service.SelectUserModelChannel(user, modelName)
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		FailError(w, err)
		return
	}
	body, contentType = normalizeAIProxyBody(path, channel.Mode, channel.SizeStrategy, body, contentType)
	if path == "/images/generations" || path == "/images/edits" {
		logAIImageRequest(channel.Name, channel.Mode, channel.SizeStrategy, modelName, body, contentType)
	}
	targetURL := service.BuildModelChannelURL(channel, path)
	isCodexImage := channel.Mode == "codex" && (path == "/images/generations" || path == "/images/edits")
	request, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("AI proxy build request failed: url=%s err=%v", targetURL, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	refund := func() {}
	if credits > 0 {
		if err := service.ConsumeUserCredits(user.ID, modelName, credits, path); err != nil {
			FailError(w, err)
			return
		}
		refund = func() {
			if err := service.RefundUserCredits(user.ID, modelName, credits, path); err != nil {
				log.Printf("AI proxy refund credits failed: user=%s model=%s credits=%d err=%v", user.ID, modelName, credits, err)
			}
		}
	}
	if isCodexImage {
		copyAICodexFetchStreamResponse(w, targetURL, channel.APIKey, body, contentType, refund)
		return
	}
	request.Header.Set("Accept", "application/json")
	if path == "/images/generations" || path == "/images/edits" {
		copyAIImageResponse(w, request, body, contentType, refund)
		return
	}
	copyAIResponse(w, request, refund)
}

func copyAIImageResponse(w http.ResponseWriter, request *http.Request, body []byte, contentType string, onFailure func()) {
	response, err := aiProxyHTTPClient.Do(request)
	if err == nil {
		defer response.Body.Close()
		if shouldRetryImageStreamFallback(response.StatusCode) {
			payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			log.Printf("AI image upstream retry stream fallback: url=%s status=%d body=%s", request.URL.String(), response.StatusCode, strings.TrimSpace(string(payload)))
			retryAIImageStreamFallback(w, request, body, contentType, onFailure)
			return
		}
		copyAIHTTPResponse(w, response, request.URL.String(), onFailure)
		return
	}
	log.Printf("AI image proxy request failed, retry stream fallback: url=%s err=%v", request.URL.String(), err)
	retryAIImageStreamFallback(w, request, body, contentType, onFailure)
}

func retryAIImageStreamFallback(w http.ResponseWriter, request *http.Request, body []byte, contentType string, onFailure func()) {
	streamBody, streamContentType, ok := withImageStreamFallbackBody(body, contentType)
	if !ok {
		if onFailure != nil {
			onFailure()
		}
		Fail(w, "AI 接口请求失败")
		return
	}
	apiKey := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
	if apiKey == "" {
		if onFailure != nil {
			onFailure()
		}
		Fail(w, "AI 接口请求失败")
		return
	}
	copyAICodexFetchStreamResponse(w, request.URL.String(), apiKey, streamBody, streamContentType, onFailure)
}

func shouldRetryImageStreamFallback(statusCode int) bool {
	return statusCode == http.StatusBadGateway ||
		statusCode == http.StatusGatewayTimeout ||
		(statusCode >= 520 && statusCode <= 524)
}

func copyAIResponse(w http.ResponseWriter, request *http.Request, onFailure func()) {
	response, err := aiProxyHTTPClient.Do(request)
	if err != nil {
		log.Printf("AI proxy request failed: url=%s err=%v", request.URL.String(), err)
		if onFailure != nil {
			onFailure()
		}
		Fail(w, "AI 接口请求失败")
		return
	}
	defer response.Body.Close()
	copyAIHTTPResponse(w, response, request.URL.String(), onFailure)
}

func copyAIHTTPResponse(w http.ResponseWriter, response *http.Response, targetURL string, onFailure func()) {
	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		log.Printf("AI upstream error: url=%s status=%d body=%s", targetURL, response.StatusCode, strings.TrimSpace(string(payload)))
		if onFailure != nil {
			onFailure()
		}
		Fail(w, "AI 接口请求失败")
		return
	}

	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

var aiProxyHTTPClient = &http.Client{
	Timeout:   30 * time.Minute,
	Transport: newAIProxyTransport(),
}

func newAIProxyTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return transport
}

func copyAICodexStreamResponse(w http.ResponseWriter, request *http.Request) {
	response, err := aiProxyHTTPClient.Do(request)
	if err != nil {
		log.Printf("AI codex stream request failed: url=%s err=%v", request.URL.String(), err)
		Fail(w, "AI 接口请求失败")
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		log.Printf("AI codex stream upstream error: url=%s status=%d body=%s", request.URL.String(), response.StatusCode, strings.TrimSpace(string(payload)))
		Fail(w, "AI 接口请求失败")
		return
	}

	payload, err := readCodexStreamPayload(response.Body)
	if err != nil {
		log.Printf("AI codex stream parse failed: url=%s err=%v", request.URL.String(), err)
		Fail(w, "AI 接口请求失败")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func copyAICodexFetchStreamResponse(w http.ResponseWriter, targetURL string, apiKey string, body []byte, contentType string, onFailure func()) {
	payload, err := runCodexCurlStream(targetURL, apiKey, body, contentType)
	if err != nil {
		log.Printf("AI codex fetch stream failed, retry non-stream fallback: url=%s err=%v", targetURL, err)
		if fallbackBody, fallbackContentType, ok := withoutImageStreamBody(body, contentType); ok {
			if fallbackPayload, fallbackErr := runCodexCurlJSON(targetURL, apiKey, fallbackBody, fallbackContentType); fallbackErr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(fallbackPayload)
				return
			} else {
				log.Printf("AI codex non-stream fallback failed: url=%s err=%v", targetURL, fallbackErr)
			}
		}
		if onFailure != nil {
			onFailure()
		}
		Fail(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func runCodexCurlStream(targetURL string, apiKey string, body []byte, contentType string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	args := []string{
		"-k",
		"-N",
		"-sS",
		"--http1.1",
		"--no-buffer",
		"--connect-timeout", "30",
		"--max-time", "1800",
		"--retry", "0",
		"-X", http.MethodPost,
		targetURL,
		"-H", "Authorization: Bearer " + apiKey,
		"-H", "Accept: text/event-stream",
	}
	if contentType != "" {
		args = append(args, "-H", "Content-Type: "+contentType)
	}
	args = append(args, "--data-binary", "@-")
	cmd := exec.CommandContext(ctx, "curl", args...)
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	payload, err := readCodexCurlStreamPayload(stdout)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if err == nil {
		return payload, nil
	}
	if message := strings.TrimSpace(stderr.String()); message != "" {
		log.Printf("AI codex curl stream stderr: url=%s stderr=%s", targetURL, strings.TrimSpace(string(limitBytes([]byte(message), 4096))))
		return nil, &aiError{message}
	}
	return nil, err
}

func runCodexCurlJSON(targetURL string, apiKey string, body []byte, contentType string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	args := []string{
		"-k",
		"-sS",
		"--http1.1",
		"--connect-timeout", "30",
		"--max-time", "1800",
		"--retry", "0",
		"-X", http.MethodPost,
		targetURL,
		"-H", "Authorization: Bearer " + apiKey,
		"-H", "Accept: application/json",
	}
	if contentType != "" {
		args = append(args, "-H", "Content-Type: "+contentType)
	}
	args = append(args, "--data-binary", "@-")
	cmd := exec.CommandContext(ctx, "curl", args...)
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	rawBody := strings.TrimSpace(string(limitBytes(output, 4096)))
	stderrBody := strings.TrimSpace(string(limitBytes(stderr.Bytes(), 4096)))
	if payload, ok := normalizeCodexImagePayload(bytes.TrimSpace(output)); ok {
		return payload, nil
	}
	if payload, ok := codexImagePayloadFromString(string(bytes.TrimSpace(output))); ok {
		return payload, nil
	}
	if message := parseAIErrorPayload(output); message != "" {
		log.Printf("AI codex json fallback upstream error: url=%s err=%v stderr=%s body=%s", targetURL, err, stderrBody, rawBody)
		return nil, &aiError{message}
	}
	if err != nil {
		log.Printf("AI codex json fallback curl failed: url=%s err=%v stderr=%s body=%s", targetURL, err, stderrBody, rawBody)
		if stderrBody != "" {
			return nil, &aiError{stderrBody}
		}
		if rawBody != "" {
			return nil, &aiError{rawBody}
		}
		return nil, &aiError{"Upstream request failed"}
	}
	log.Printf("AI codex json fallback unrecognized response: url=%s stderr=%s body=%s", targetURL, stderrBody, rawBody)
	return nil, &aiError{"接口没有返回图片"}
}

func readCodexCurlStreamPayload(reader io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 128*1024*1024)
	var dataLines []string
	var rawLines []string
	var lastPartial []map[string]any
	var streamErr error
	var eventName string
	flushEvent := func() ([]byte, bool) {
		if len(dataLines) == 0 {
			if eventName == "error" {
				streamErr = &aiError{"上游返回流式错误事件"}
				eventName = ""
			}
			return nil, false
		}
		eventBody := strings.Join(dataLines, "\n")
		dataLines = nil
		eventName = ""
		var event map[string]any
		if json.Unmarshal([]byte(eventBody), &event) != nil {
			if payload, ok := codexImagePayloadFromString(eventBody); ok {
				return payload, true
			}
			return nil, false
		}
		if message := codexStreamErrorMessage(event); message != "" {
			streamErr = &aiError{message}
			return nil, false
		}
		if payload, ok := normalizeCodexImagePayload([]byte(eventBody)); ok {
			return payload, true
		}
		object, _ := event["object"].(string)
		eventType, _ := event["type"].(string)
		if object == "image.generation.result" || object == "image.edit.result" || eventType == "image_generation.completed" || eventType == "image_edit.completed" {
			if payload, ok := normalizeCodexImagePayload([]byte(eventBody)); ok {
				return payload, true
			}
		}
		if eventType == "image_generation.partial_image" || eventType == "image_edit.partial_image" {
			if item := codexImageItem(event); item != nil {
				lastPartial = []map[string]any{item}
				payload, err := json.Marshal(map[string]any{"data": lastPartial})
				return payload, err == nil
			}
		}
		return nil, false
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if payload, ok := flushEvent(); ok {
				return payload, nil
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			rawLines = append(rawLines, line)
			if payload, ok := codexImagePayloadFromString(line); ok {
				return payload, nil
			}
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		dataLines = append(dataLines, data)
	}
	if payload, ok := flushEvent(); ok {
		return payload, nil
	}
	if len(lastPartial) > 0 {
		payload, err := json.Marshal(map[string]any{"data": lastPartial})
		return payload, err
	}
	if len(rawLines) > 0 {
		rawBody := strings.Join(rawLines, "\n")
		if payload, err := parseCodexStreamPayloadBody([]byte(rawBody)); err == nil {
			return payload, nil
		}
		if payload, ok := codexImagePayloadFromString(rawBody); ok {
			return payload, nil
		}
		log.Printf("AI codex stream unrecognized response: body=%s", strings.TrimSpace(string(limitBytes([]byte(rawBody), 4096))))
	}
	if scannerErr := scanner.Err(); scannerErr != nil {
		return nil, scannerErr
	}
	if streamErr != nil {
		return nil, streamErr
	}
	return nil, &aiError{"Codex 流式接口未返回最终图片数据"}
}

func withoutImageStreamBody(body []byte, contentType string) ([]byte, string, bool) {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return withoutImageStreamMultipartBody(body, contentType)
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil, "", false
	}
	delete(payload, "stream")
	delete(payload, "partial_images")
	nextBody, err := json.Marshal(payload)
	return nextBody, contentType, err == nil
}

func withoutImageStreamMultipartBody(body []byte, contentType string) ([]byte, string, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", false
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return nil, "", false
	}
	defer form.RemoveAll()

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, values := range form.Value {
		if key == "stream" || key == "partial_images" {
			continue
		}
		for _, value := range values {
			_ = writer.WriteField(key, value)
		}
	}
	for key, files := range form.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				continue
			}
			part, err := writer.CreateFormFile(key, fileHeader.Filename)
			if err == nil {
				_, _ = io.Copy(part, file)
			}
			_ = file.Close()
		}
	}
	if writer.Close() != nil {
		return nil, "", false
	}
	return buffer.Bytes(), writer.FormDataContentType(), true
}

func readCodexStreamPayload(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(reader)
	payload, parseErr := parseCodexStreamPayloadBody(body)
	if parseErr == nil {
		return payload, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, parseErr
}

func parseCodexStreamPayloadBody(body []byte) ([]byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, &aiError{"Codex 流式接口未返回最终图片数据"}
	}
	if payload, ok := normalizeCodexImagePayload(bytes.TrimSpace(body)); ok {
		return payload, nil
	}
	if payload, ok := codexImagePayloadFromString(string(bytes.TrimSpace(body))); ok {
		return payload, nil
	}
	events := splitServerSentEvents(string(body))
	completedItems := make([]map[string]any, 0)
	partialItems := make([]map[string]any, 0)
	var resultPayload []byte
	var streamErr error
	for _, eventBody := range events {
		var event map[string]any
		if json.Unmarshal([]byte(eventBody), &event) != nil {
			continue
		}
		if message := codexStreamErrorMessage(event); message != "" {
			streamErr = &aiError{message}
			continue
		}
		if payload, ok := normalizeCodexImagePayload([]byte(eventBody)); ok {
			resultPayload = payload
			continue
		}
		object, _ := event["object"].(string)
		eventType, _ := event["type"].(string)
		if object == "image.generation.result" || object == "image.edit.result" {
			if payload, ok := normalizeCodexImagePayload([]byte(eventBody)); ok {
				resultPayload = payload
			}
			continue
		}
		if eventType == "image_generation.completed" || eventType == "image_edit.completed" {
			if item := codexImageItem(event); item != nil {
				completedItems = append(completedItems, item)
			}
		}
		if eventType == "image_generation.partial_image" || eventType == "image_edit.partial_image" {
			if item := codexImageItem(event); item != nil {
				partialItems = append(partialItems, item)
			}
		}
	}
	if len(resultPayload) > 0 {
		return resultPayload, nil
	}
	if len(completedItems) > 0 {
		payload, err := json.Marshal(map[string]any{"data": completedItems})
		return payload, err
	}
	if len(partialItems) > 0 {
		payload, err := json.Marshal(map[string]any{"data": []map[string]any{partialItems[len(partialItems)-1]}})
		return payload, err
	}
	if streamErr != nil {
		return nil, streamErr
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		if payload, ok := codexImagePayloadFromString(line); ok {
			return payload, nil
		}
	}
	return nil, &aiError{"Codex 流式接口未返回最终图片数据"}
}

func parseAIErrorPayload(body []byte) string {
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return ""
	}
	var event map[string]any
	if json.Unmarshal(body, &event) != nil {
		return ""
	}
	return codexStreamErrorMessage(event)
}

func splitServerSentEvents(body string) []string {
	blocks := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n")
	events := make([]string, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		dataLines := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data != "" && data != "[DONE]" {
					dataLines = append(dataLines, data)
				}
			}
		}
		if len(dataLines) > 0 {
			events = append(events, strings.Join(dataLines, "\n"))
		}
	}
	return events
}

func codexStreamErrorMessage(event map[string]any) string {
	if value, ok := event["error"].(string); ok {
		return strings.TrimSpace(value)
	}
	if errorValue, ok := event["error"].(map[string]any); ok {
		if message, ok := errorValue["message"].(string); ok {
			return strings.TrimSpace(message)
		}
	}
	if eventType, _ := event["type"].(string); strings.HasSuffix(eventType, ".failed") {
		if message, ok := event["message"].(string); ok && strings.TrimSpace(message) != "" {
			return strings.TrimSpace(message)
		}
		return "流式请求失败"
	}
	return ""
}

func codexImageItem(event map[string]any) map[string]any {
	for _, key := range []string{"b64_json", "base64_json", "base64", "image_base64", "imageBase64", "b64", "image", "data", "content", "text"} {
		if b64, ok := event[key].(string); ok && looksLikeImagePayload(b64) {
			return map[string]any{"b64_json": stripDataURLPrefix(b64)}
		}
	}
	for _, key := range []string{"url", "image_url", "imageUrl", "output_url", "outputUrl", "asset_url", "assetUrl", "download_url", "downloadUrl"} {
		if url, ok := event[key].(string); ok && looksLikeHTTPURL(url) {
			return map[string]any{"url": strings.TrimSpace(url)}
		}
	}
	return nil
}

func looksLikeImagePayload(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "data:image/") || looksLikeBase64ImagePayload(value)
}

func looksLikeBase64ImagePayload(value string) bool {
	if len(value) <= 1024 || strings.ContainsAny(value, " \t\r\n") || strings.Contains(value, "://") {
		return false
	}
	valid := 0
	for _, item := range value {
		if (item >= 'A' && item <= 'Z') || (item >= 'a' && item <= 'z') || (item >= '0' && item <= '9') || item == '+' || item == '/' || item == '=' || item == '-' || item == '_' {
			valid++
		}
	}
	return valid*100/len(value) > 95
}

func looksLikeImageURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if !looksLikeHTTPURL(value) {
		return false
	}
	return strings.Contains(value, ".png") ||
		strings.Contains(value, ".jpg") ||
		strings.Contains(value, ".jpeg") ||
		strings.Contains(value, ".webp") ||
		strings.Contains(value, ".gif") ||
		strings.Contains(value, "/image") ||
		strings.Contains(value, "/images")
}

func looksLikeHTTPURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func stripDataURLPrefix(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, ","); strings.HasPrefix(value, "data:image/") && index >= 0 {
		return value[index+1:]
	}
	return value
}

func normalizeCodexImagePayload(data []byte) ([]byte, bool) {
	if !json.Valid(data) {
		return nil, false
	}
	var payload any
	if json.Unmarshal(data, &payload) != nil {
		return nil, false
	}
	items := collectCodexImageItems(payload)
	if len(items) == 0 {
		return nil, false
	}
	nextPayload, err := json.Marshal(map[string]any{"data": items})
	return nextPayload, err == nil
}

func codexImagePayloadFromString(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if json.Valid([]byte(value)) {
		if payload, ok := normalizeCodexImagePayload([]byte(value)); ok {
			return payload, true
		}
		var text string
		if json.Unmarshal([]byte(value), &text) == nil {
			return codexImagePayloadFromString(text)
		}
		return nil, false
	}
	if looksLikeImagePayload(value) {
		payload, err := json.Marshal(map[string]any{"data": []map[string]any{{"b64_json": stripDataURLPrefix(value)}}})
		return payload, err == nil
	}
	if looksLikeImageURL(value) {
		payload, err := json.Marshal(map[string]any{"data": []map[string]any{{"url": strings.TrimSpace(value)}}})
		return payload, err == nil
	}
	return nil, false
}

func collectCodexImageItems(value any) []map[string]any {
	var items []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			if item := codexImageItem(typed); item != nil {
				items = append(items, item)
				return
			}
			for _, key := range []string{"data", "image", "images", "output", "outputs", "result", "results", "choices", "message", "content", "delta", "artifacts", "attachments", "file", "files"} {
				if next, ok := typed[key]; ok {
					walk(next)
				}
			}
		}
	}
	walk(value)
	return items
}

func limitBytes(data []byte, size int) []byte {
	if len(data) <= size {
		return data
	}
	return data[:size]
}

func readAIRequest(r *http.Request) ([]byte, string, string, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", "", err
	}
	modelName := ""
	if strings.HasPrefix(contentType, "multipart/form-data") {
		modelName = readMultipartModel(body, contentType)
	} else {
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &payload)
		modelName = payload.Model
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, "", "", errMissingModel
	}
	return body, contentType, modelName, nil
}

func readMultipartModel(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return ""
	}
	defer form.RemoveAll()
	if values := form.Value["model"]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func readAIRequestCount(body []byte, contentType string) int {
	count := 1
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return count
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return count
		}
		defer form.RemoveAll()
		if values := form.Value["n"]; len(values) > 0 {
			_, _ = fmt.Sscan(values[0], &count)
		}
	} else {
		var payload struct {
			N int `json:"n"`
		}
		_ = json.Unmarshal(body, &payload)
		count = payload.N
	}
	if count < 1 {
		return 1
	}
	return count
}

func logAIImageRequest(channelName string, mode string, strategy string, modelName string, body []byte, contentType string) {
	size := readAIStringField(body, contentType, "size")
	quality := readAIStringField(body, contentType, "quality")
	stream := readAIStringField(body, contentType, "stream")
	partialImages := readAIStringField(body, contentType, "partial_images")
	tier := imageQualityTier(quality)
	if quality == "" {
		tier = imageTierFromPixelSize(size)
	}
	log.Printf(
		"AI image request normalized: channel=%s mode=%s model=%s size=%s tier=%s strategy=%s stream=%s partial_images=%s",
		channelName,
		mode,
		modelName,
		size,
		tier,
		normalizeSizeStrategy(strategy),
		stream,
		partialImages,
	)
}

func imageTierFromPixelSize(size string) string {
	width, height, ok := parsePixelImageSize(size)
	if !ok {
		return "1k"
	}
	pixels := width * height
	if pixels > 4194304 {
		return "4k"
	}
	if pixels > 1572864 {
		return "2k"
	}
	return "1k"
}

func readAIStringField(body []byte, contentType string, key string) string {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return ""
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return ""
		}
		defer form.RemoveAll()
		if values := form.Value[key]; len(values) > 0 {
			return values[0]
		}
		return ""
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	value := payload[key]
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func aiCostTier(path string, body []byte, contentType string) string {
	if path != "/images/generations" && path != "/images/edits" {
		return ""
	}
	return imageQualityTier(readAIQuality(body, contentType))
}

func normalizeAIProxyBody(path string, mode string, sizeStrategy string, body []byte, contentType string) ([]byte, string) {
	isImageRequest := path == "/images/generations" || path == "/images/edits"
	if isImageRequest {
		body, contentType = normalizeImageSizeBody(body, contentType, sizeStrategy)
	}
	if mode != "codex" || !isImageRequest {
		return body, contentType
	}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		nextBody, nextContentType, ok := normalizeCodexMultipartBody(body, contentType)
		if ok {
			return nextBody, nextContentType
		}
		return body, contentType
	}
	nextBody, ok := normalizeCodexJSONBody(body)
	if ok {
		return nextBody, contentType
	}
	return body, contentType
}

func normalizeImageSizeBody(body []byte, contentType string, strategy string) ([]byte, string) {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		nextBody, nextContentType, ok := normalizeImageSizeMultipartBody(body, contentType, strategy)
		if ok {
			return nextBody, nextContentType
		}
		return body, contentType
	}
	nextBody, ok := normalizeImageSizeJSONBody(body, strategy)
	if ok {
		return nextBody, contentType
	}
	return body, contentType
}

func normalizeImageSizeJSONBody(body []byte, strategy string) ([]byte, bool) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil, false
	}
	size, _ := payload["size"].(string)
	quality, _ := payload["quality"].(string)
	payload["size"] = normalizeImageSizeByQuality(size, quality, strategy)
	nextBody, err := json.Marshal(payload)
	return nextBody, err == nil
}

func normalizeImageSizeMultipartBody(body []byte, contentType string, strategy string) ([]byte, string, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", false
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return nil, "", false
	}
	defer form.RemoveAll()

	quality := ""
	if values := form.Value["quality"]; len(values) > 0 {
		quality = values[0]
	}
	hasSize := false
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, values := range form.Value {
		for _, value := range values {
			if key == "size" {
				value = normalizeImageSizeByQuality(value, quality, strategy)
				hasSize = true
			}
			_ = writer.WriteField(key, value)
		}
	}
	if !hasSize {
		_ = writer.WriteField("size", normalizeImageSizeByQuality("", quality, strategy))
	}
	for key, files := range form.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				continue
			}
			part, err := writer.CreateFormFile(key, fileHeader.Filename)
			if err == nil {
				_, _ = io.Copy(part, file)
			}
			_ = file.Close()
		}
	}
	if writer.Close() != nil {
		return nil, "", false
	}
	return buffer.Bytes(), writer.FormDataContentType(), true
}

func withImageStreamFallbackBody(body []byte, contentType string) ([]byte, string, bool) {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return withImageStreamFallbackMultipartBody(body, contentType)
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil, "", false
	}
	payload["stream"] = true
	payload["partial_images"] = 1
	nextBody, err := json.Marshal(payload)
	return nextBody, contentType, err == nil
}

func withImageStreamFallbackMultipartBody(body []byte, contentType string) ([]byte, string, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", false
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return nil, "", false
	}
	defer form.RemoveAll()

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, values := range form.Value {
		if key == "stream" || key == "partial_images" {
			continue
		}
		for _, value := range values {
			_ = writer.WriteField(key, value)
		}
	}
	_ = writer.WriteField("stream", "true")
	_ = writer.WriteField("partial_images", "1")
	for key, files := range form.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				continue
			}
			part, err := writer.CreateFormFile(key, fileHeader.Filename)
			if err == nil {
				_, _ = io.Copy(part, file)
			}
			_ = file.Close()
		}
	}
	if writer.Close() != nil {
		return nil, "", false
	}
	return buffer.Bytes(), writer.FormDataContentType(), true
}

func normalizeCodexJSONBody(body []byte) ([]byte, bool) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil, false
	}
	if prompt, ok := payload["prompt"].(string); ok {
		payload["prompt"] = withCodexPromptGuard(prompt)
	}
	delete(payload, "quality")
	delete(payload, "response_format")
	payload["output_format"] = "png"
	payload["stream"] = true
	payload["partial_images"] = 1
	if _, ok := payload["moderation"]; !ok {
		payload["moderation"] = "auto"
	}
	if size, ok := payload["size"].(string); ok {
		payload["size"] = normalizeCodexImageSize(size)
	}
	nextBody, err := json.Marshal(payload)
	return nextBody, err == nil
}

func normalizeCodexMultipartBody(body []byte, contentType string) ([]byte, string, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", false
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return nil, "", false
	}
	defer form.RemoveAll()

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, values := range form.Value {
		if key == "quality" || key == "response_format" {
			continue
		}
		for _, value := range values {
			if key == "size" {
				value = normalizeCodexImageSize(value)
			} else if key == "prompt" {
				value = withCodexPromptGuard(value)
			}
			_ = writer.WriteField(key, value)
		}
	}
	_ = writer.WriteField("output_format", "png")
	_ = writer.WriteField("stream", "true")
	_ = writer.WriteField("partial_images", "1")
	if _, ok := form.Value["moderation"]; !ok {
		_ = writer.WriteField("moderation", "auto")
	}
	for key, files := range form.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				continue
			}
			part, err := writer.CreateFormFile(key, fileHeader.Filename)
			if err == nil {
				_, _ = io.Copy(part, file)
			}
			_ = file.Close()
		}
	}
	if writer.Close() != nil {
		return nil, "", false
	}
	return buffer.Bytes(), writer.FormDataContentType(), true
}

func normalizeCodexImageSize(size string) string {
	return normalizeImageSizeByQuality(size, "", "exact")
}

func normalizeImageSizeByQuality(size string, quality string, strategy string) string {
	value := strings.ToLower(strings.TrimSpace(size))
	if isPixelImageSize(value) {
		return value
	}
	aspect := value
	if aspect == "" || aspect == "auto" {
		aspect = "1:1"
	}
	aspect, _, _ = strings.Cut(aspect, "-")
	tier := imageQualityTier(quality)
	switch normalizeSizeStrategy(strategy) {
	case "safe":
		return safeImageSizeByTier(aspect, tier)
	case "compatible":
		if size := compatibleImageSizeByTier(aspect, tier); size != "" {
			return size
		}
	}

	switch tier {
	case "4k":
		switch aspect {
		case "1:1":
			return "4096x4096"
		case "3:2":
			return "3840x2560"
		case "2:3":
			return "2560x3840"
		case "4:3":
			return "3840x2880"
		case "3:4":
			return "2880x3840"
		case "16:9":
			return "3840x2160"
		case "9:16":
			return "2160x3840"
		}
	case "2k":
		switch aspect {
		case "1:1":
			return "2048x2048"
		case "3:2":
			return "2048x1365"
		case "2:3":
			return "1365x2048"
		case "4:3":
			return "2048x1536"
		case "3:4":
			return "1536x2048"
		case "16:9":
			return "2048x1152"
		case "9:16":
			return "1152x2048"
		}
	default:
		switch aspect {
		case "1:1":
			return "1024x1024"
		case "3:2":
			return "1536x1024"
		case "2:3":
			return "1024x1536"
		case "4:3":
			return "1344x1024"
		case "3:4":
			return "1024x1344"
		case "16:9":
			return "1536x864"
		case "9:16":
			return "864x1536"
		}
	}
	return size
}

func normalizeSizeStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "safe", "compatible":
		return strings.ToLower(strings.TrimSpace(strategy))
	default:
		return "exact"
	}
}

func safeImageSizeByTier(aspect string, tier string) string {
	orientation := imageOrientation(aspect)
	switch tier {
	case "4k":
		switch orientation {
		case "landscape":
			return "3840x2560"
		case "portrait":
			return "2560x3840"
		default:
			return "4096x4096"
		}
	case "2k":
		switch orientation {
		case "landscape":
			return "2048x1365"
		case "portrait":
			return "1365x2048"
		default:
			return "2048x2048"
		}
	default:
		switch orientation {
		case "landscape":
			return "1536x1024"
		case "portrait":
			return "1024x1536"
		default:
			return "1024x1024"
		}
	}
}

func compatibleImageSizeByTier(aspect string, tier string) string {
	ratioWidth, ratioHeight, ok := parseImageAspect(aspect)
	if !ok {
		return ""
	}
	targetRatio := float64(ratioWidth) / float64(ratioHeight)
	pixelBudget := 1572864
	if tier == "2k" {
		pixelBudget = 4194304
	} else if tier == "4k" {
		pixelBudget = 8294400
	}
	bestWidth, bestHeight, bestPixels := 0, 0, 0
	for width := 16; width <= 3840; width += 16 {
		idealHeight := float64(width) / targetRatio
		for _, height := range []int{floorToMultiple(idealHeight, 16), ceilToMultiple(idealHeight, 16)} {
			if height < 16 || height > 3840 {
				continue
			}
			pixels := width * height
			if pixels > pixelBudget || pixels < 655360 {
				continue
			}
			if maxFloat(float64(width)/float64(height), float64(height)/float64(width)) > 3 {
				continue
			}
			actualRatio := float64(width) / float64(height)
			if absFloat(actualRatio-targetRatio)/targetRatio > 0.01 {
				continue
			}
			if pixels > bestPixels {
				bestWidth, bestHeight, bestPixels = width, height, pixels
			}
		}
	}
	if bestPixels == 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", bestWidth, bestHeight)
}

func imageOrientation(aspect string) string {
	width, height, ok := parseImageAspect(aspect)
	if !ok || width == height {
		return "square"
	}
	if width > height {
		return "landscape"
	}
	return "portrait"
}

func parseImageAspect(aspect string) (int, int, bool) {
	parts := strings.Split(aspect, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	var width, height int
	if _, err := fmt.Sscan(parts[0], &width); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscan(parts[1], &height); err != nil {
		return 0, 0, false
	}
	return width, height, width > 0 && height > 0
}

func floorToMultiple(value float64, multiple int) int {
	return int(value/float64(multiple)) * multiple
}

func ceilToMultiple(value float64, multiple int) int {
	result := int(value/float64(multiple)) * multiple
	if float64(result) < value {
		result += multiple
	}
	return result
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func imageQualityTier(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "medium":
		return "2k"
	case "high":
		return "4k"
	default:
		return "1k"
	}
}

func isPixelImageSize(size string) bool {
	_, _, ok := parsePixelImageSize(size)
	return ok
}

func parsePixelImageSize(size string) (int, int, bool) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 || !isPositiveInteger(parts[0]) || !isPositiveInteger(parts[1]) {
		return 0, 0, false
	}
	var width, height int
	_, widthErr := fmt.Sscan(parts[0], &width)
	_, heightErr := fmt.Sscan(parts[1], &height)
	return width, height, widthErr == nil && heightErr == nil && width > 0 && height > 0
}

func isPositiveInteger(value string) bool {
	if value == "" {
		return false
	}
	for _, item := range value {
		if item < '0' || item > '9' {
			return false
		}
	}
	return true
}

func withCodexPromptGuard(prompt string) string {
	return "Use the following text as the complete prompt. Do not rewrite it:\n" + prompt
}

func canUseImageQuality(r *http.Request) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	quality := strings.TrimSpace(readAIQuality(body, r.Header.Get("Content-Type")))
	if quality != "medium" && quality != "high" {
		return true
	}
	user, ok := service.UserFromContext(r.Context())
	return ok && (user.Role == "vip" || user.Role == "admin")
}

func readAIQuality(body []byte, contentType string) string {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return ""
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		form, err := reader.ReadForm(32 << 20)
		if err != nil {
			return ""
		}
		defer form.RemoveAll()
		if values := form.Value["quality"]; len(values) > 0 {
			return values[0]
		}
		return ""
	}
	var payload struct {
		Quality string `json:"quality"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.Quality
}

var errMissingModel = &aiError{"缺少模型名称"}

type aiError struct {
	message string
}

func (err *aiError) Error() string {
	return err.message
}



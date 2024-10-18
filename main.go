package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/owu-one/gotosocial-sdk/client/accounts"
	"github.com/owu-one/gotosocial-sdk/client/notifications"
	"github.com/owu-one/gotosocial-sdk/client/statuses"
	"github.com/owu-one/gotosocial-sdk/models"
)

func main() {
	checkConnections()

	for {
		log.Printf("<%s> Polling for notifications...", time.Now().Format("2006-01-02 15:04:05"))
		processNotifications()
		time.Sleep(20 * time.Second)
	}
}

func checkConnections() {
	_, err := gts.Client.Accounts.AccountVerify(accounts.NewAccountVerifyParams(), gts.Auth)
	if err != nil {
		log.Fatalf("GoToSocial Connection Error: %v", err)
		os.Exit(1)
	}
	log.Println("GoToSocial Connection: OK")

	err = pingGPTService()
	if err != nil {
		log.Fatalf("GPT Connection Error: %v", err)
		os.Exit(1)
	}
	log.Println("GPT Connection: OK")
}

func pingGPTService() error {
	url := fmt.Sprintf("%s/chat/completions", config.OpenAIAPIURL)
	payload := strings.NewReader(`{"model": "` + config.OpenAIModel + `", "messages": [{"role": "user", "content": "Ping"}]}`)

	req, _ := http.NewRequest("POST", url, payload)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+config.OpenAIAPIKey)

	res, err := openAI.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("GPT service returned non-200 status code: %d", res.StatusCode)
	}

	return nil
}

func processNotifications() {
	notifs, err := gts.Client.Notifications.Notifications(notifications.NewNotificationsParams(), gts.Auth)
	if err != nil {
		log.Printf("Failed to fetch notifications: %v", err)
		return
	}

	for _, notif := range notifs.Payload {
		if notif.Type != "mention" {
			continue
		}

		processNotification(notif)
	}

	_, err = gts.Client.Notifications.ClearNotifications(notifications.NewClearNotificationsParams(), gts.Auth)
	if err != nil {
		log.Printf("Failed to clear notifications: %v", err)
	}
}

func processNotification(notif *models.Notification) {
	stack := buildConversationStack(notif.Status)
	chatHistory := buildChatHistory(stack)
	printChatHistory(chatHistory)

	response := callGPT(chatHistory)
	if response == "" {
		log.Println("Empty response from GPT service")
		return
	}

	replyToStatus(notif.Status, response)
}

func buildConversationStack(status *models.Status) []*models.Status {
	stack := []*models.Status{status}
	currentStatus := status

	for len(stack) < config.MaxHistoryCount && currentStatus.InReplyToID != "" {
		params := statuses.NewStatusGetParams().WithID(currentStatus.InReplyToID)
		resp, err := gts.Client.Statuses.StatusGet(params, gts.Auth)
		if err != nil {
			log.Printf("Failed to get status: %v", err)
			break
		}
		stack = append(stack, resp.Payload)
		currentStatus = resp.Payload
	}

	return trimStackToMaxChar(stack)
}

func trimStackToMaxChar(stack []*models.Status) []*models.Status {
	totalChars := 0
	for i := len(stack) - 1; i >= 0; i-- {
		totalChars += len(stack[i].Content)
		if totalChars > config.MaxHistoryChar {
			return stack[i+1:]
		}
	}
	return stack
}

func buildChatHistory(stack []*models.Status) []Message {
	chatHistory := []Message{
		{
			Role: "system",
			ChatContent: []ChatContent{
				{
					Type: "text",
					Text: config.SystemPrompt,
				},
			},
		},
	}

	botAcct := fmt.Sprintf("%s@%s", config.BotAccountName, config.FediDomain)

	reversedStack := make([]*models.Status, len(stack))
	for i, status := range stack {
		reversedStack[len(stack)-1-i] = status
	}

	for _, status := range reversedStack {
		t := status.Text
		if t == "" {
			t = status.Content
		}
		if t == "" {
			continue
		}
		statusText := ChatContent{
			Type: "text",
			Text: t,
		}
		msg := Message{
			Role: "user",
			ChatContent: []ChatContent{
				statusText,
			},
		}
		if status.Account.Acct == botAcct {
			msg.Role = "assistant"
		}
		for _, attachment := range status.MediaAttachments {
			if isValidImageAttachment(attachment) {
				// send req to attachment.URL, get image binary and convert to base64
				img := "data:image/jpeg;base64," + getBase64Image(attachment.URL)
				imgContent := ChatContent{
					Type: "image_url",
					ImageURL: &ImageContent{
						URL: img,
					},
				}
				msg.ChatContent = append(msg.ChatContent, imgContent)
			} else {
				msg.ChatContent[0].Text += fmt.Sprintf("\n【系统提示】媒体附件 %s 被跳过，因为数量可能超出限制或格式不受支持", filepath.Base(attachment.URL))
			}
		}
		chatHistory = append(chatHistory, msg)
	}

	return chatHistory
}

func isValidImageAttachment(attachment *models.Attachment) bool {
	validExtensions := []string{".jpg", ".jpeg", ".png"}
	ext := strings.ToLower(filepath.Ext(attachment.URL))
	for _, validExt := range validExtensions {
		if ext == validExt {
			return true
		}
	}
	return false
}

func getBase64Image(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to fetch image: %v", err)
		return ""
	}
	defer resp.Body.Close()

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read image: %v", err)
		return ""
	}

	return base64.StdEncoding.EncodeToString(imgBytes)
}

func printChatHistory(chatHistory []Message) {
	log.Println("Processing Chat History:")
	for _, msg := range chatHistory {
		log.Printf("Role: %s, Content: %v", msg.Role, msg.ChatContent)
	}
	log.Println("")
}

func callGPT(chatHistory []Message) string {
	url := fmt.Sprintf("%s/chat/completions", config.OpenAIAPIURL)
	payload, _ := json.Marshal(map[string]interface{}{
		"model":    config.OpenAIModel,
		"messages": chatHistory,
	})

	req, _ := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+config.OpenAIAPIKey)

	res, err := openAI.Do(req)
	if err != nil {
		log.Printf("Failed to call GPT service: %v", err)
		return "ERROR: 与GPT服务通信失败，若问题持续，请联系管理员"
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		log.Println("Invalid response format from GPT service")
		return "ERROR: 与GPT服务通信失败，若问题持续，请联系管理员"
	}

	message, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	if !ok {
		log.Println("Invalid message format in GPT response")
		return "ERROR: 与GPT服务通信失败，若问题持续，请联系管理员"
	}

	content, ok := message["content"].(string)
	if !ok {
		log.Println("Invalid content format in GPT message")
		return "ERROR: 与GPT服务通信失败，若问题持续，请联系管理员"
	}

	return content
}

func replyToStatus(status *models.Status, response string) {
	mentionAcct := fmt.Sprintf("@%s", status.Account.Acct)
	fullResponse := fmt.Sprintf("%s %s", mentionAcct, response)
	remaining := ""

	if len(fullResponse) > config.MaxChar {
		remaining = fullResponse[config.MaxChar:]
		fullResponse = fullResponse[:config.MaxChar]
	}

	params := statuses.NewStatusCreateParams().
		WithStatus(ptr(fullResponse)).
		WithInReplyToID(ptr(status.ID)).
		WithContentType(ptr("text/markdown")).
		WithLanguage(ptr(status.Language)).
		WithVisibility(ptr(status.Visibility)).
		WithLocalOnly(ptr(status.LocalOnly)).
		WithSensitive(ptr(status.Sensitive))

	if status.InteractionPolicy != nil {
		if len(status.InteractionPolicy.CanFavourite.Always) > 0 {
			params.SetInteractionPolicyCanFavouriteAlways0(ptr(string(status.InteractionPolicy.CanFavourite.Always[0])))
		}
		if len(status.InteractionPolicy.CanFavourite.WithApproval) > 0 {
			params.SetInteractionPolicyCanFavouriteWithApproval0(ptr(string(status.InteractionPolicy.CanFavourite.WithApproval[0])))
		}
		if len(status.InteractionPolicy.CanReblog.Always) > 0 {
			params.SetInteractionPolicyCanReblogAlways0(ptr(string(status.InteractionPolicy.CanReblog.Always[0])))
		}
		if len(status.InteractionPolicy.CanReblog.WithApproval) > 0 {
			params.SetInteractionPolicyCanReblogWithApproval0(ptr(string(status.InteractionPolicy.CanReblog.WithApproval[0])))
		}
		if len(status.InteractionPolicy.CanReply.Always) > 0 {
			params.SetInteractionPolicyCanReplyAlways0(ptr(string(status.InteractionPolicy.CanReply.Always[0])))
		}
		if len(status.InteractionPolicy.CanReply.WithApproval) > 0 {
			params.SetInteractionPolicyCanReplyWithApproval0(ptr(string(status.InteractionPolicy.CanReply.WithApproval[0])))
		}
	}

	if status.Visibility == "public" {
		params.SetVisibility(ptr("unlisted"))
	}
	if status.Visibility == "private" || status.Visibility == "mutuals_only" {
		params.SetVisibility(ptr("direct"))
	}
	if status.SpoilerText != "" {
		params.SpoilerText = ptr("re: " + status.SpoilerText)
	}

	reply, err := gts.Client.Statuses.StatusCreate(
		params,
		gts.Auth,
		func(op *runtime.ClientOperation) {
			op.ConsumesMediaTypes = []string{"multipart/form-data"}
		},
	)
	if err != nil {
		log.Printf("Failed to create reply status: %v", err)
		return
	}

	if remaining != "" {
		replyToStatus(reply.Payload, remaining)
	}
}

func ptr[T any](v T) *T { return &v }

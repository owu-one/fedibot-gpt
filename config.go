package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/joho/godotenv"
	gtsclient "github.com/owu-one/gotosocial-sdk/client"
	"github.com/owu-one/gotosocial-sdk/models"
	"golang.org/x/time/rate"
)

var (
	gts               Client
	openAI            *http.Client
	config            Config
	notificationStack []*models.Notification
)

type Client struct {
	Client  *gtsclient.GoToSocialSwaggerDocumentation
	Auth    runtime.ClientAuthInfoWriter
	limiter *rate.Limiter
	ctx     context.Context
}

type Config struct {
	OpenAIAPIKey        string
	OpenAIAPIURL        string
	OpenAIModel         string
	OpenAIModelExternal string
	FediDomain          string
	ClientKey           string
	ClientSecret        string
	AccessToken         string
	BotAccountName      string
	MaxChar             int
	MaxHistoryCount     int
	MaxHistoryChar      int
	SystemPrompt        string
}

type Message struct {
	Role        string        `json:"role"`
	ChatContent []ChatContent `json:"content"`
}

type ChatContent struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageContent `json:"image_url,omitempty"`
}

type ImageContent struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // default: auto
}

func init() {
	loadConfig()
	initClients()
}

func loadConfig() {
	godotenv.Load()

	config = Config{
		OpenAIAPIKey:        getEnv("OPENAI_API_KEY", ""),
		OpenAIAPIURL:        getEnv("OPENAI_API_URL", "https://api.openai.com/v1"),
		OpenAIModel:         getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAIModelExternal: getEnv("OPENAI_MODEL_EXTERNAL", "gpt-4o-mini"),
		FediDomain:          getEnv("FEDI_DOMAIN", ""),
		ClientKey:           getEnv("CLIENT_KEY", ""),
		ClientSecret:        getEnv("CLIENT_SECRET", ""),
		AccessToken:         getEnv("ACCESS_TOKEN", ""),
		BotAccountName:      getEnv("BOT_ACCOUNT_NAME", ""),
		MaxChar:             getEnvAsInt("MAX_CHAR", 450),
		MaxHistoryCount:     getEnvAsInt("MAX_HISTORY_COUNT", 6),
		MaxHistoryChar:      getEnvAsInt("MAX_HISTORY_CHAR", 5000),
		SystemPrompt:        getEnv("SYSTEM_PROMPT", ""),
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func initClients() {
	gts = Client{
		Client: gtsclient.New(
			httptransport.New(config.FediDomain, "", []string{"https"}), strfmt.Default,
		),
		Auth:    httptransport.BearerToken(config.AccessToken),
		limiter: rate.NewLimiter(1.0, 300),
		ctx:     context.Background(),
	}

	openAI = &http.Client{
		Timeout: time.Second * 30,
	}
}

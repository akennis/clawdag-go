package library

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"google.golang.org/genai"
)

type aiCallRequest struct {
	SystemText string
	Prompt     string
	MaxTokens  int64
}

type aiCallResult struct {
	Text         string
	InputTokens  int64
	OutputTokens int64
}

type aiCaller interface {
	call(ctx context.Context, req aiCallRequest) (aiCallResult, error)
}

// newAICaller creates a caller for the given provider and model.
// provider must be "claude" or "gemini"; model is passed through opaquely to the SDK.
// Returns an error for unknown providers so graphs fail fast at Setup.
func newAICaller(provider, model string) (aiCaller, error) {
	switch provider {
	case "claude":
		return &anthropicCaller{model: model}, nil
	case "gemini":
		return &geminiCaller{model: model}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q: must be \"claude\" or \"gemini\"", provider)
	}
}

// anthropicCaller calls the Anthropic Messages API.
// API key is read from CLAUDE_API_KEY.
type anthropicCaller struct{ model string }

func (c *anthropicCaller) call(ctx context.Context, req aiCallRequest) (aiCallResult, error) {
	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("CLAUDE_API_KEY")))
	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: req.MaxTokens,
		System:    []anthropic.TextBlockParam{{Text: req.SystemText}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt))},
	})
	if err != nil {
		return aiCallResult{}, err
	}
	var text string
	for _, block := range msg.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return aiCallResult{
		Text:         text,
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
	}, nil
}

// geminiCaller calls the Gemini GenerateContent API.
// API key is read from GEMINI_API_KEY.
type geminiCaller struct{ model string }

func (c *geminiCaller) call(ctx context.Context, req aiCallRequest) (aiCallResult, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: os.Getenv("GEMINI_API_KEY"),
	})
	if err != nil {
		return aiCallResult{}, fmt.Errorf("gemini: create client: %w", err)
	}
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(req.SystemText, genai.RoleUser),
		MaxOutputTokens:   int32(req.MaxTokens),
	}
	result, err := client.Models.GenerateContent(ctx, c.model, genai.Text(req.Prompt), config)
	if err != nil {
		return aiCallResult{}, fmt.Errorf("gemini: generate content: %w", err)
	}
	var inputTokens, outputTokens int64
	if result.UsageMetadata != nil {
		inputTokens = int64(result.UsageMetadata.PromptTokenCount)
		outputTokens = int64(result.UsageMetadata.CandidatesTokenCount)
	}
	return aiCallResult{
		Text:         result.Text(),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/wwz16/dagor/config"
	"google.golang.org/api/option"
)

// AIInputFormatter is an optional interface for In types to describe themselves in prompts.
type AIInputFormatter interface {
	FormatForPrompt() string
}

// AIOutputFormatter is an optional interface for Out types to describe the expected response format.
type AIOutputFormatter interface {
	ExpectedFormat() string
}

// AIResponseParser must be implemented by Out types that are structs (non-scalar, non-slice).
type AIResponseParser interface {
	ParseAIResponse(response string) error
}

// AIComputeOp is a generic AI-powered compute operator.
// In is the input type, Out is the output type.
// Do not register AIComputeOp directly — use a concrete variant like AIComputeMathOperandsToFloat64Op.
type AIComputeOp[In, Out any] struct {
	Input     *In    // single strongly-typed input
	Result    Out    // single strongly-typed output
	Reasoning string // always present

	operation  string
	maxRetries int
}

func (op *AIComputeOp[In, Out]) Setup(params *config.Params) error {
	op.operation = params.GetString("operation", "")
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *AIComputeOp[In, Out]) Reset() error { return nil }

func (op *AIComputeOp[In, Out]) InputFields() map[string]any {
	return map[string]any{
		"Input": &op.Input,
	}
}

func (op *AIComputeOp[In, Out]) OutputFields() map[string]any {
	return map[string]any{
		"Result":    &op.Result,
		"Reasoning": &op.Reasoning,
	}
}

func (op *AIComputeOp[In, Out]) SetInputField(field string, value any) error {
	switch field {
	case "Input":
		val, ok := value.(*In)
		if !ok {
			return fmt.Errorf("field Input: expected *%T, got %T", op.Input, value)
		}
		op.Input = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *AIComputeOp[In, Out]) ResetFields() {
	var zeroInput *In
	op.Input = zeroInput
	var zeroResult Out
	op.Result = zeroResult
	op.Reasoning = ""
}

func (op *AIComputeOp[In, Out]) Run(ctx context.Context) error {
	log.Printf("[DEBUG] AIComputeOp[%T]: operation=%q", op.Result, op.operation)

	apiKey := os.Getenv("GOOGLE_GENAI_API_KEY")
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("genai client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-flash-latest")
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"result":    {Type: genai.TypeString},
			"reasoning": {Type: genai.TypeString},
		},
		Required: []string{"result", "reasoning"},
	}

	// Build input description.
	var inputDesc string
	if op.Input != nil {
		if f, ok := any(op.Input).(AIInputFormatter); ok {
			inputDesc = f.FormatForPrompt()
		} else {
			inputDesc = fmt.Sprintf("%+v", *op.Input)
		}
	}

	// Build output format description.
	var formatDesc string
	var zeroOut Out
	if f, ok := any(&zeroOut).(AIOutputFormatter); ok {
		formatDesc = f.ExpectedFormat()
	} else {
		formatDesc = op.builtinFormatDescription()
	}

	basePrompt := fmt.Sprintf("You are computing: %s\nInput: %s\n%s",
		op.operation, inputDesc, formatDesc)

	var prevResponse, prevErr string
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		prompt := basePrompt
		if prevResponse != "" {
			prompt += fmt.Sprintf("\nPrevious response: %s\nParse error: %s\nTry again.", prevResponse, prevErr)
		}

		resp, err := model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			return fmt.Errorf("generate content: %w", err)
		}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return fmt.Errorf("no candidates in response")
		}

		var raw string
		for _, part := range resp.Candidates[0].Content.Parts {
			raw += fmt.Sprintf("%v", part)
		}

		var envelope struct {
			Result    string `json:"result"`
			Reasoning string `json:"reasoning"`
		}
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			prevResponse = raw
			prevErr = err.Error()
			log.Printf("[DEBUG] AIComputeOp: attempt %d envelope parse failed: %v", attempt+1, err)
			continue
		}

		if parseErr := op.parseResult(envelope.Result); parseErr != nil {
			prevResponse = raw
			prevErr = parseErr.Error()
			log.Printf("[DEBUG] AIComputeOp: attempt %d result parse failed: %v", attempt+1, parseErr)
			continue
		}

		op.Reasoning = envelope.Reasoning
		log.Printf("[DEBUG] AIComputeOp: result=%v reasoning=%q", op.Result, op.Reasoning)
		return nil
	}

	return fmt.Errorf("AIComputeOp: all %d attempts failed; last error: %s", op.maxRetries+1, prevErr)
}

func (op *AIComputeOp[In, Out]) builtinFormatDescription() string {
	var zeroOut Out
	switch any(&zeroOut).(type) {
	case *float64:
		return `Respond with JSON: {"result": "<numeric value as string>", "reasoning": "<explanation>"}`
	case *int:
		return `Respond with JSON: {"result": "<integer value as string>", "reasoning": "<explanation>"}`
	case *string:
		return `Respond with JSON: {"result": "<string value>", "reasoning": "<explanation>"}`
	case *[]float64:
		return `Respond with JSON: {"result": "[<float1>, <float2>, ...]", "reasoning": "<explanation>"}`
	case *[]string:
		return `Respond with JSON: {"result": "[\"str1\", \"str2\", ...]", "reasoning": "<explanation>"}`
	default:
		return `Respond with JSON: {"result": "<value>", "reasoning": "<explanation>"}`
	}
}

func (op *AIComputeOp[In, Out]) parseResult(raw string) error {
	raw = strings.TrimSpace(raw)
	switch v := any(&op.Result).(type) {
	case *float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("expected float64, got %q: %w", raw, err)
		}
		*v = f
	case *int:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("expected int, got %q: %w", raw, err)
		}
		*v = n
	case *string:
		*v = raw
	case *[]float64:
		var s []float64
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return fmt.Errorf("expected []float64, got %q: %w", raw, err)
		}
		*v = s
	case *[]string:
		var s []string
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return fmt.Errorf("expected []string, got %q: %w", raw, err)
		}
		*v = s
	case AIResponseParser:
		return v.ParseAIResponse(raw)
	default:
		return fmt.Errorf("unsupported output type %T; implement AIResponseParser", op.Result)
	}
	return nil
}

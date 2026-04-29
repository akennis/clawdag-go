package library

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/operator"
)

// ---- Concrete variants of AIComputeOp ----

const AIExtractStringSliceOpDescription = `AIExtractStringSliceOp: AI-powered extraction of a list from text.
  Params:   operation string — plain-English description (e.g. "extract all ingredient names from this recipe").
            max_retries string — parse retries (default "3").
  Inputs:   Input *string.
  Outputs:  Result []string (CSV), Reasoning string.`

// AIExtractStringSliceOp extracts a list of strings from arbitrary text.
type AIExtractStringSliceOp struct {
	AIComputeOp[string, []string]
}

const AIExtractMapOpDescription = `AIExtractMapOp: AI-powered extraction of key-value pairs from text.
  Params:   operation string — plain-English description (e.g. "extract name, email, and city from this contact info").
            max_retries string — parse retries (default "3").
  Inputs:   Input *string.
  Outputs:  Result map[string]string (key=value CSV), Reasoning string.`

// AIExtractMapOp extracts a fixed-key record from arbitrary text.
type AIExtractMapOp struct {
	AIComputeOp[string, map[string]string]
}

const AIParseNumberOpDescription = `AIParseNumberOp: AI-powered number extraction — converts text to float64.
  Params:   operation string — plain-English description (default: leave empty to extract the number from the text).
            max_retries string — parse retries (default "3").
  Inputs:   Input *string (e.g. "two thousand", "$1.2k", "the price is 42").
  Outputs:  Result float64, Reasoning string.`

// AIParseNumberOp converts free-form text to a float64.
type AIParseNumberOp struct {
	AIComputeOp[string, float64]
}

const AISummarizeOpDescription = `AISummarizeOp: AI-powered summarization of a list of strings into one result string.
  Params:   operation string — plain-English instruction (e.g. "summarize into one concise sentence").
            max_retries string — parse retries (default "3").
  Inputs:   Input *[]string — items to summarize.
  Outputs:  Result string, Reasoning string.`

// AISummarizeOp summarizes a slice of strings into a single string.
type AISummarizeOp struct {
	AIComputeOp[[]string, string]
}

// ---- Bespoke AI ops ----

const AIClassifyMultiLabelOpDescription = `AIClassifyMultiLabelOp: AI-powered multi-label classifier — maps input to zero or more categories.
  Params:   categories string — comma-separated list of valid labels (e.g. "billing,bug,feature,spam").
            max_retries string — parse/validation retries (default "3").
  Inputs:   Input *string.
  Outputs:  Result []string — subset of categories (CSV), Reasoning string.`

// AIClassifyMultiLabelOp classifies text into zero or more of a fixed set of categories.
type AIClassifyMultiLabelOp struct {
	Input     *string
	Result    []string
	Reasoning string

	categories []string
	catSet     map[string]bool
	maxRetries int
}

func (op *AIClassifyMultiLabelOp) Setup(params *config.Params) error {
	raw := params.GetString("categories", "")
	if raw == "" {
		return fmt.Errorf("AIClassifyMultiLabelOp: 'categories' param is required")
	}
	op.categories = nil // reset before appending so pool reuse doesn't accumulate
	op.catSet = make(map[string]bool)
	for _, c := range strings.Split(raw, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			op.categories = append(op.categories, c)
			op.catSet[c] = true
		}
	}
	if len(op.categories) < 2 {
		return fmt.Errorf("AIClassifyMultiLabelOp: at least 2 categories required, got %d", len(op.categories))
	}
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *AIClassifyMultiLabelOp) Reset() error { return nil }

func (op *AIClassifyMultiLabelOp) InputFields() map[string]any {
	return map[string]any{"Input": &op.Input}
}

func (op *AIClassifyMultiLabelOp) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result, "Reasoning": &op.Reasoning}
}

func (op *AIClassifyMultiLabelOp) SetInputField(field string, value any) error {
	switch field {
	case "Input":
		val, ok := value.(*string)
		if !ok {
			return fmt.Errorf("field Input: expected *string, got %T", value)
		}
		op.Input = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *AIClassifyMultiLabelOp) ResetFields() {
	op.Input = nil
	op.Result = nil
	op.Reasoning = ""
}

func (op *AIClassifyMultiLabelOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] AIClassifyMultiLabelOp: classifying %q into %v", *op.Input, op.categories)

	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("CLAUDE_API_KEY")))
	catList := strings.Join(op.categories, ", ")

	basePrompt := fmt.Sprintf(
		"Classify the following input into zero or more of these categories: %s.\n"+
			"Respond with matching categories as a comma-separated list. If none match, respond with an empty line.\n"+
			"Input: %s",
		catList, *op.Input,
	)

	prompt := basePrompt
	var lastErr string
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_6,
			MaxTokens: 256,
			System: []anthropic.TextBlockParam{
				{Text: "Respond with only the requested value. No explanation, no punctuation beyond commas, no formatting."},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})
		if err != nil {
			return fmt.Errorf("generate content: %w", err)
		}

		var raw string
		for _, block := range msg.Content {
			if block.Type == "text" {
				raw += block.Text
			}
		}
		raw = strings.TrimSpace(raw)

		var labels []string
		if raw != "" {
			for _, item := range strings.Split(raw, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					labels = append(labels, item)
				}
			}
		}

		var invalid []string
		for _, label := range labels {
			if !op.catSet[label] {
				invalid = append(invalid, label)
			}
		}

		if len(invalid) > 0 {
			lastErr = fmt.Sprintf("invalid categories %v not in %v", invalid, op.categories)
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious response contained invalid categories: %v. Use only: %s.", invalid, catList)
			log.Printf("[DEBUG] AIClassifyMultiLabelOp: attempt %d invalid labels: %s", attempt+1, lastErr)
			continue
		}

		op.Result = labels
		log.Printf("[DEBUG] AIClassifyMultiLabelOp: result=%v", op.Result)
		return nil
	}
	return fmt.Errorf("AIClassifyMultiLabelOp: all %d attempts failed; last error: %s", op.maxRetries+1, lastErr)
}

const AIScoreOpDescription = `AIScoreOp: AI-powered scoring — returns a float64 in [0,1] measuring a criterion.
  Params:   criterion string — what to measure (e.g. "relevance to the query", "toxicity").
            max_retries string — parse/validation retries (default "3").
  Inputs:   Input *string.
  Outputs:  Result float64 ∈ [0,1], Reasoning string.`

// AIScoreOp scores text against a criterion, returning a value in [0,1].
type AIScoreOp struct {
	Input     *string
	Result    float64
	Reasoning string

	criterion  string
	maxRetries int
}

func (op *AIScoreOp) Setup(params *config.Params) error {
	op.criterion = params.GetString("criterion", "")
	if op.criterion == "" {
		return fmt.Errorf("AIScoreOp: 'criterion' param is required")
	}
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *AIScoreOp) Reset() error { return nil }

func (op *AIScoreOp) InputFields() map[string]any {
	return map[string]any{"Input": &op.Input}
}

func (op *AIScoreOp) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result, "Reasoning": &op.Reasoning}
}

func (op *AIScoreOp) SetInputField(field string, value any) error {
	switch field {
	case "Input":
		val, ok := value.(*string)
		if !ok {
			return fmt.Errorf("field Input: expected *string, got %T", value)
		}
		op.Input = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *AIScoreOp) ResetFields() {
	op.Input = nil
	op.Result = 0
	op.Reasoning = ""
}

func (op *AIScoreOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] AIScoreOp: criterion=%q", op.criterion)

	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("CLAUDE_API_KEY")))

	basePrompt := fmt.Sprintf(
		"Score the following text for %s on a scale from 0.0 to 1.0.\n"+
			"Respond with only the numeric score. No explanation.\n"+
			"Text: %s",
		op.criterion, *op.Input,
	)

	prompt := basePrompt
	var lastErr string
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_6,
			MaxTokens: 16,
			System: []anthropic.TextBlockParam{
				{Text: "Respond with only a decimal number between 0.0 and 1.0. No other text."},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})
		if err != nil {
			return fmt.Errorf("generate content: %w", err)
		}

		var raw string
		for _, block := range msg.Content {
			if block.Type == "text" {
				raw += block.Text
			}
		}
		raw = strings.TrimSpace(raw)

		score, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			lastErr = fmt.Sprintf("expected float64, got %q: %v", raw, err)
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious response %q was not a valid number. Respond with only a decimal number between 0.0 and 1.0.", raw)
			log.Printf("[DEBUG] AIScoreOp: attempt %d parse failed: %s", attempt+1, lastErr)
			continue
		}
		if score < 0 || score > 1 {
			lastErr = fmt.Sprintf("score %v out of [0,1]", score)
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious score %v was out of range. Respond with a number between 0.0 and 1.0.", score)
			log.Printf("[DEBUG] AIScoreOp: attempt %d out of range: %s", attempt+1, lastErr)
			continue
		}

		op.Result = score
		log.Printf("[DEBUG] AIScoreOp: result=%v", op.Result)
		return nil
	}
	return fmt.Errorf("AIScoreOp: all %d attempts failed; last error: %s", op.maxRetries+1, lastErr)
}

const AIBoolOpDescription = `AIBoolOp: AI-powered yes/no predicate.
  Params:   predicate string — the question to answer about the input (e.g. "does this text contain PII?").
            max_retries string — parse/validation retries (default "3").
  Inputs:   Input *string.
  Outputs:  Result bool, Reasoning string.`

// AIBoolOp answers a yes/no predicate about the input text.
type AIBoolOp struct {
	Input     *string
	Result    bool
	Reasoning string

	predicate  string
	maxRetries int
}

func (op *AIBoolOp) Setup(params *config.Params) error {
	op.predicate = params.GetString("predicate", "")
	if op.predicate == "" {
		return fmt.Errorf("AIBoolOp: 'predicate' param is required")
	}
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *AIBoolOp) Reset() error { return nil }

func (op *AIBoolOp) InputFields() map[string]any {
	return map[string]any{"Input": &op.Input}
}

func (op *AIBoolOp) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result, "Reasoning": &op.Reasoning}
}

func (op *AIBoolOp) SetInputField(field string, value any) error {
	switch field {
	case "Input":
		val, ok := value.(*string)
		if !ok {
			return fmt.Errorf("field Input: expected *string, got %T", value)
		}
		op.Input = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *AIBoolOp) ResetFields() {
	op.Input = nil
	op.Result = false
	op.Reasoning = ""
}

func (op *AIBoolOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] AIBoolOp: predicate=%q", op.predicate)

	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("CLAUDE_API_KEY")))

	basePrompt := fmt.Sprintf(
		"Answer the following question about the text with only 'true' or 'false'.\n"+
			"Question: %s\nText: %s",
		op.predicate, *op.Input,
	)

	prompt := basePrompt
	var lastErr string
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_6,
			MaxTokens: 8,
			System: []anthropic.TextBlockParam{
				{Text: "Respond with only 'true' or 'false'."},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})
		if err != nil {
			return fmt.Errorf("generate content: %w", err)
		}

		var raw string
		for _, block := range msg.Content {
			if block.Type == "text" {
				raw += block.Text
			}
		}
		raw = strings.ToLower(strings.TrimSpace(raw))

		switch raw {
		case "true":
			op.Result = true
			log.Printf("[DEBUG] AIBoolOp: result=true")
			return nil
		case "false":
			op.Result = false
			log.Printf("[DEBUG] AIBoolOp: result=false")
			return nil
		default:
			lastErr = fmt.Sprintf("expected true or false, got %q", raw)
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious response %q was invalid. Respond with only 'true' or 'false'.", raw)
			log.Printf("[DEBUG] AIBoolOp: attempt %d invalid: %s", attempt+1, lastErr)
		}
	}
	return fmt.Errorf("AIBoolOp: all %d attempts failed; last error: %s", op.maxRetries+1, lastErr)
}

const AIBestMatchOpDescription = `AIBestMatchOp: AI-powered semantic selection — returns the index of the best-matching candidate.
  Params:   max_retries string — parse/validation retries (default "3").
  Inputs:   Query *string, Candidates *[]string.
  Outputs:  Result int (0-based index), Reasoning string.`

// AIBestMatchOp selects the best-matching candidate for a query, returning its 0-based index.
type AIBestMatchOp struct {
	Query      *string
	Candidates *[]string
	Result     int
	Reasoning  string

	maxRetries int
}

func (op *AIBestMatchOp) Setup(params *config.Params) error {
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *AIBestMatchOp) Reset() error { return nil }

func (op *AIBestMatchOp) InputFields() map[string]any {
	return map[string]any{"Query": &op.Query, "Candidates": &op.Candidates}
}

func (op *AIBestMatchOp) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result, "Reasoning": &op.Reasoning}
}

func (op *AIBestMatchOp) SetInputField(field string, value any) error {
	switch field {
	case "Query":
		val, ok := value.(*string)
		if !ok {
			return fmt.Errorf("field Query: expected *string, got %T", value)
		}
		op.Query = val
	case "Candidates":
		val, ok := value.(*[]string)
		if !ok {
			return fmt.Errorf("field Candidates: expected *[]string, got %T", value)
		}
		op.Candidates = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *AIBestMatchOp) ResetFields() {
	op.Query = nil
	op.Candidates = nil
	op.Result = 0
	op.Reasoning = ""
}

func (op *AIBestMatchOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] AIBestMatchOp: query=%q candidates=%v", *op.Query, *op.Candidates)

	n := len(*op.Candidates)
	if n == 0 {
		return fmt.Errorf("AIBestMatchOp: candidates list is empty")
	}

	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("CLAUDE_API_KEY")))

	var sb strings.Builder
	for i, c := range *op.Candidates {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i, c))
	}

	basePrompt := fmt.Sprintf(
		"Given the query, return the 0-based index of the best matching candidate.\n"+
			"Respond with only the integer index. No explanation.\n"+
			"Query: %s\nCandidates:\n%s",
		*op.Query, sb.String(),
	)

	prompt := basePrompt
	var lastErr string
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_6,
			MaxTokens: 8,
			System: []anthropic.TextBlockParam{
				{Text: "Respond with only an integer index."},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})
		if err != nil {
			return fmt.Errorf("generate content: %w", err)
		}

		var raw string
		for _, block := range msg.Content {
			if block.Type == "text" {
				raw += block.Text
			}
		}
		raw = strings.TrimSpace(raw)

		idx, err := strconv.Atoi(raw)
		if err != nil {
			lastErr = fmt.Sprintf("expected integer index, got %q: %v", raw, err)
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious response %q was not a valid integer. Respond with only the integer index.", raw)
			log.Printf("[DEBUG] AIBestMatchOp: attempt %d parse failed: %s", attempt+1, lastErr)
			continue
		}
		if idx < 0 || idx >= n {
			lastErr = fmt.Sprintf("index %d out of range [0,%d)", idx, n)
			prompt = basePrompt + fmt.Sprintf("\n\nIndex %d is out of range. Must be between 0 and %d.", idx, n-1)
			log.Printf("[DEBUG] AIBestMatchOp: attempt %d out of range: %s", attempt+1, lastErr)
			continue
		}

		op.Result = idx
		log.Printf("[DEBUG] AIBestMatchOp: result=%d", op.Result)
		return nil
	}
	return fmt.Errorf("AIBestMatchOp: all %d attempts failed; last error: %s", op.maxRetries+1, lastErr)
}

const AIRerankOpDescription = `AIRerankOp: AI-powered reranking — returns a permutation of candidate indices, best first.
  Params:   max_retries string — parse/validation retries (default "3").
  Inputs:   Query *string, Candidates *[]string.
  Outputs:  Result []int (permutation as CSV), Reasoning string.`

// AIRerankOp reranks candidates by relevance to a query, returning a permutation of 0-based indices.
type AIRerankOp struct {
	Query      *string
	Candidates *[]string
	Result     []int
	Reasoning  string

	maxRetries int
}

func (op *AIRerankOp) Setup(params *config.Params) error {
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *AIRerankOp) Reset() error { return nil }

func (op *AIRerankOp) InputFields() map[string]any {
	return map[string]any{"Query": &op.Query, "Candidates": &op.Candidates}
}

func (op *AIRerankOp) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result, "Reasoning": &op.Reasoning}
}

func (op *AIRerankOp) SetInputField(field string, value any) error {
	switch field {
	case "Query":
		val, ok := value.(*string)
		if !ok {
			return fmt.Errorf("field Query: expected *string, got %T", value)
		}
		op.Query = val
	case "Candidates":
		val, ok := value.(*[]string)
		if !ok {
			return fmt.Errorf("field Candidates: expected *[]string, got %T", value)
		}
		op.Candidates = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *AIRerankOp) ResetFields() {
	op.Query = nil
	op.Candidates = nil
	op.Result = nil
	op.Reasoning = ""
}

func (op *AIRerankOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] AIRerankOp: query=%q candidates=%v", *op.Query, *op.Candidates)

	n := len(*op.Candidates)
	if n == 0 {
		return fmt.Errorf("AIRerankOp: candidates list is empty")
	}

	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("CLAUDE_API_KEY")))

	var sb strings.Builder
	for i, c := range *op.Candidates {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i, c))
	}

	basePrompt := fmt.Sprintf(
		"Rerank the following candidates by relevance to the query, best first.\n"+
			"Respond with only the 0-based indices as a comma-separated list. No explanation.\n"+
			"Query: %s\nCandidates:\n%s",
		*op.Query, sb.String(),
	)

	prompt := basePrompt
	var lastErr string
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_6,
			MaxTokens: 64,
			System: []anthropic.TextBlockParam{
				{Text: "Respond with only a comma-separated list of integers."},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})
		if err != nil {
			return fmt.Errorf("generate content: %w", err)
		}

		var raw string
		for _, block := range msg.Content {
			if block.Type == "text" {
				raw += block.Text
			}
		}
		raw = strings.TrimSpace(raw)

		parts := strings.Split(raw, ",")
		indices := make([]int, 0, len(parts))
		var parseErr string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			idx, err := strconv.Atoi(p)
			if err != nil {
				parseErr = fmt.Sprintf("expected integer, got %q: %v", p, err)
				break
			}
			indices = append(indices, idx)
		}
		if parseErr != "" {
			lastErr = parseErr
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious response %q was invalid: %s. Respond with comma-separated integers only.", raw, parseErr)
			log.Printf("[DEBUG] AIRerankOp: attempt %d parse failed: %s", attempt+1, lastErr)
			continue
		}

		if len(indices) != n {
			lastErr = fmt.Sprintf("expected %d indices, got %d", n, len(indices))
			prompt = basePrompt + fmt.Sprintf("\n\nMust return exactly %d indices (one per candidate).", n)
			log.Printf("[DEBUG] AIRerankOp: attempt %d wrong count: %s", attempt+1, lastErr)
			continue
		}
		seen := make(map[int]bool, n)
		var dupErr string
		for _, idx := range indices {
			if idx < 0 || idx >= n {
				dupErr = fmt.Sprintf("index %d out of range [0,%d)", idx, n)
				break
			}
			if seen[idx] {
				dupErr = fmt.Sprintf("duplicate index %d", idx)
				break
			}
			seen[idx] = true
		}
		if dupErr != "" {
			lastErr = dupErr
			prompt = basePrompt + fmt.Sprintf("\n\nPrevious response was invalid: %s. Return each index 0-%d exactly once.", dupErr, n-1)
			log.Printf("[DEBUG] AIRerankOp: attempt %d invalid permutation: %s", attempt+1, lastErr)
			continue
		}

		op.Result = indices
		log.Printf("[DEBUG] AIRerankOp: result=%v", op.Result)
		return nil
	}
	return fmt.Errorf("AIRerankOp: all %d attempts failed; last error: %s", op.maxRetries+1, lastErr)
}

func init() {
	operator.RegisterOp[AIExtractStringSliceOp]()
	operator.RegisterOp[AIExtractMapOp]()
	operator.RegisterOp[AIParseNumberOp]()
	operator.RegisterOp[AISummarizeOp]()
	operator.RegisterOp[AIClassifyMultiLabelOp]()
	operator.RegisterOp[AIScoreOp]()
	operator.RegisterOp[AIBoolOp]()
	operator.RegisterOp[AIBestMatchOp]()
	operator.RegisterOp[AIRerankOp]()
}

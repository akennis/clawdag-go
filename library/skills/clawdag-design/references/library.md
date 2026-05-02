# Available Library Ops

## Math
AddOp: deterministic addition. Inputs: A *float64, B *float64. Output: Result float64.
SubOp: A minus B. Inputs: A *float64, B *float64. Output: Result float64.
DivOp: A divided by B. Inputs: A *float64, B *float64. Output: Result float64. Error if B==0.
MulOp: A multiplied by B. Inputs: A *float64, B *float64. Output: Result float64.
RoundOp: rounds Value to nearest integer. Input: Value *float64. Output: Result float64.
ClampOp: clamps Value to [Min, Max]. Inputs: Value *float64, Min *float64, Max *float64. Output: Result float64.
SumOp: sums all values in a slice. Input: Values *[]float64. Output: Result float64.
MinOp: returns the minimum value in a slice. Input: Values *[]float64. Output: Result float64. Error if empty.
MaxOp: returns the maximum value in a slice. Input: Values *[]float64. Output: Result float64. Error if empty.
PackMathOperandsOp: packs two float64 inputs into a MathOperands struct. Inputs: A *float64, B *float64. Output: Result MathOperands.


## String
StringLookupOp: looks up Key in a hardcoded string→string map; returns "" on miss.
  Params: map — JSON-encoded key→value pairs (e.g. {"hamburger":"ketchup","hotdog":"mustard"}).
  Input:  Key *string.
  Output: Result string (empty string if key not found).
StringToLowerOp: converts a string to lowercase. Input: Value *string. Output: Result string.
StringConcatOp: concatenates two strings. Inputs: A *string, B *string. Output: Result string.
StringSplitOp: splits a string by a separator. Param: sep (default ","). Input: Input *string. Output: Result []string.
RegexMatchOp: reports whether the input matches a compiled regex. Param: pattern (required). Input: Input *string. Output: Match bool.
RegexExtractOp: returns the first match (or submatch group 1 if present) of a regex. Param: pattern (required). Input: Input *string. Output: Result string (empty if no match).


## Bool
BoolNotOp: logical NOT. Input: Value *bool. Output: Result bool.
BoolAndOp: logical AND. Inputs: A *bool, B *bool. Output: Result bool.
BoolOrOp: logical OR. Inputs: A *bool, B *bool. Output: Result bool.


## Predicate — float
IfFloatGtOp: reports whether A > B. Inputs: A *float64, B *float64. Output: Match bool.
IfFloatLtOp: reports whether A < B. Inputs: A *float64, B *float64. Output: Match bool.
IfFloatEqOp: reports whether A == B. Inputs: A *float64, B *float64. Output: Match bool.
IfFloatGeOp: reports whether A >= B. Inputs: A *float64, B *float64. Output: Match bool.
IfFloatLeOp: reports whether A <= B. Inputs: A *float64, B *float64. Output: Match bool.


## Predicate — int
IfIntGtOp: reports whether A > B. Inputs: A *int, B *int. Output: Match bool.
IfIntLtOp: reports whether A < B. Inputs: A *int, B *int. Output: Match bool.
IfIntEqOp: reports whether A == B. Inputs: A *int, B *int. Output: Match bool.
IfIntGeOp: reports whether A >= B. Inputs: A *int, B *int. Output: Match bool.
IfIntLeOp: reports whether A <= B. Inputs: A *int, B *int. Output: Match bool.


## Predicate — string
IfStringContainsOp: reports whether A contains B as a substring. Inputs: A *string, B *string. Output: Match bool.
IfStringHasPrefixOp: reports whether A starts with B. Inputs: A *string, B *string. Output: Match bool.
IfStringHasSuffixOp: reports whether A ends with B. Inputs: A *string, B *string. Output: Match bool.
IfStringRegexMatchOp: reports whether the input matches a compiled regex. Param: pattern (required). Input: Input *string. Output: Match bool.
IfStringEqOp: reports whether two strings are equal. Inputs: A *string, B *string. Output: Match bool.


## Predicate — empty / range
IfEmptyStringOp: reports whether Value is nil or the empty string. Input: Value *string. Output: Match bool.
IfEmptySliceStringOp: reports whether Value is nil or has length 0. Input: Value *[]string. Output: Match bool.
IfEmptySliceFloat64Op: reports whether Value is nil or has length 0. Input: Value *[]float64. Output: Match bool.
BetweenFloatOp: reports whether Min <= Value <= Max (inclusive on both ends). Inputs: Value *float64, Min *float64, Max *float64. Output: Match bool.


## Select / Switch / Default
SelectStringOp: ternary; returns IfTrue when Cond is true, otherwise IfFalse. Inputs: Cond *bool, IfTrue *string, IfFalse *string. Output: Result string.
SelectFloat64Op: ternary; returns IfTrue when Cond is true, otherwise IfFalse. Inputs: Cond *bool, IfTrue *float64, IfFalse *float64. Output: Result float64.
SelectIntOp: ternary; returns IfTrue when Cond is true, otherwise IfFalse. Inputs: Cond *bool, IfTrue *int, IfFalse *int. Output: Result int.
SelectBoolOp: ternary; returns IfTrue when Cond is true, otherwise IfFalse. Inputs: Cond *bool, IfTrue *bool, IfFalse *bool. Output: Result bool.
SwitchStringOp: looks up Key in a params-configured cases map; returns the configured default on miss.
  Params: cases — JSON-encoded key→value pairs (e.g. {"red":"stop","green":"go"}).
          default — string returned when Key is nil or not in cases (default "").
  Input:  Key *string.
  Output: Result string.
DefaultStringOp: returns Default when Value is nil or the empty string; otherwise returns Value. Inputs: Value *string, Default *string. Output: Result string.
DefaultFloat64Op: returns Default when Value is nil; zero is treated as a valid value. Inputs: Value *float64, Default *float64. Output: Result float64.
DefaultIntOp: returns Default when Value is nil; zero is treated as a valid value. Inputs: Value *int, Default *int. Output: Result int.


## Slice
SliceLenOp: returns the length of a string slice. Input: Input *[]string. Output: Result int.
SliceAtOp: returns the element at a given index. Param: index (int, used when Index wire is absent). Inputs: Input *[]string, Index *int (optional wire). Output: Result string.
SliceFirstOp: returns the first element. Input: Input *[]string. Output: Result string. Error if empty.
SliceLastOp: returns the last element. Input: Input *[]string. Output: Result string. Error if empty.
SliceContainsOp: reports whether a slice contains a value. Inputs: Input *[]string, Value *string. Output: Match bool.
SliceJoinOp: joins a string slice with a separator. Param: sep (default ","). Input: Input *[]string. Output: Result string.
SliceFilterEqOp: returns elements equal to Value. Inputs: Input *[]string, Value *string. Output: Result []string.
SliceTopKOp: returns indices of the K highest scores in descending order. Param: k (int). Input: Scores *[]float64. Output: Result []int.


## AI
ModeSelectOp: AI-powered classifier — maps arbitrary input text to exactly one of a fixed set of categories.
  Params:   categories string — comma-separated list of valid output values (e.g. "arithmetic expression,city name").
            max_retries string — parse/validation retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string — the text to classify.
  Outputs:  Result string — exactly one of the specified categories.
AIComputeStringToStringOp: AI-powered string→string computation.
  Params:   operation string — plain-English description (e.g. "suggest a condiment that pairs with the given food").
            max_retries string — parse retries (default "3").
  Inputs:   Input *string — the query string.
  Outputs:  Result string, Reasoning string.
AIComputeMathOperandsToFloat64Op: AI-powered fallback for operations not available in the library.
  Params:   operation string — plain-English description of what to compute (e.g. "multiply A by B").
            max_retries string — number of parse retries (default "3").
  Inputs:   Input *MathOperands (connect PackMathOperandsOp's Result wire).
  Outputs:  Result float64, Reasoning string.
AIExtractStringSliceOp: AI-powered extraction of a list from text.
  Params:   operation string — plain-English description (e.g. "extract all ingredient names from this recipe").
            max_retries string — parse retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string.
  Outputs:  Result []string (CSV), Reasoning string.
AIExtractMapOp: AI-powered extraction of key-value pairs from text.
  Params:   operation string — plain-English description (e.g. "extract name, email, and city from this contact info").
            max_retries string — parse retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string.
  Outputs:  Result map[string]string (key=value CSV), Reasoning string.
AIParseNumberOp: AI-powered number extraction — converts text to float64.
  Params:   operation string — plain-English description (default: leave empty to extract the number from the text).
            max_retries string — parse retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string (e.g. "two thousand", "$1.2k", "the price is 42").
  Outputs:  Result float64, Reasoning string.
AISummarizeOp: AI-powered summarization of a list of strings into one result string.
  Params:   operation string — plain-English instruction (e.g. "summarize into one concise sentence").
            max_retries string — parse retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *[]string — items to summarize.
  Outputs:  Result string, Reasoning string.
AIClassifyMultiLabelOp: AI-powered multi-label classifier — maps input to zero or more categories.
  Params:   categories string — comma-separated list of valid labels (e.g. "billing,bug,feature,spam").
            max_retries string — parse/validation retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string.
  Outputs:  Result []string — subset of categories (CSV), Reasoning string.
AIScoreOp: AI-powered scoring — returns a float64 in [0,1] measuring a criterion.
  Params:   criterion string — what to measure (e.g. "relevance to the query", "toxicity").
            max_retries string — parse/validation retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string.
  Outputs:  Result float64 ∈ [0,1], Reasoning string.
AIBoolOp: AI-powered yes/no predicate.
  Params:   predicate string — the question to answer about the input (e.g. "does this text contain PII?").
            max_retries string — parse/validation retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Input *string.
  Outputs:  Result bool, Reasoning string.
AIBestMatchOp: AI-powered semantic selection — returns the index of the best-matching candidate.
  Params:   max_retries string — parse/validation retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Query *string, Candidates *[]string.
  Outputs:  Result int (0-based index), Reasoning string.
AIRerankOp: AI-powered reranking — returns a permutation of candidate indices, best first.
  Params:   max_retries string — parse/validation retries (default "3").
            provider string — AI provider: "claude" (default) or "gemini".
            model string — model name passed through to the provider (default: "claude-sonnet-4-6").
  Inputs:   Query *string, Candidates *[]string.
  Outputs:  Result []int (permutation as CSV), Reasoning string.


## Time
CityTimeOp: returns the current time for a supported city.
  Input:  City *string — must be "New York" or "Tokyo"; any other value is a graph execution error.
  Output: Result string — current local time formatted as RFC3339.


## IO
FileReadOp: reads a file from disk. Input: Path *string. Output: Content string.
EnvOp: reads an environment variable. Input: Name *string. Output: Value string (empty if unset).
HTTPGetOp: performs an HTTP GET request. Input: URL *string. Outputs: Body string, StatusCode int.


## JSON
JSONExtractOp: extracts a value from a JSON string using a dot-separated path. Numeric path segments index into arrays (e.g. "meals.0.name"). Inputs: JSON *string, Path *string. Output: Value string (JSON-encoded leaf, or "" if not found).


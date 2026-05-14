# Stock Analyzer Example

This example demonstrates a hybrid deterministic/AI workflow that fetches live stock data and news from Yahoo Finance, performs deterministic calculations, and generates an AI-driven investment recommendation.

## What this example shows

- **Zero Custom Operators**: Demonstrates how to build complex logic using only the `sparsi-go` standard library.
- **Dynamic URL Building**: Uses `RegisterConst` and `StringConcatOp` to build API URLs for the given ticker.
- **Parallel Data Fetching**: Fetches stock quotes and news headlines in parallel using `HTTPGetOp`.
- **JSON Extraction**: Uses `JSONExtractOp` to pull specific fields from nested API responses.
- **Deterministic Math**: Uses `SubFloatOp` to calculate the price change since the previous close.
- **AI-Powered Fallbacks**: Uses `AIParseNumberOp` to convert extracted strings to numeric values.
- **Sentiment Analysis**: Uses `AIScoreOp` to determine the sentiment of the latest news headline.
- **Compound Prompt Engineering**: Uses a chain of `StringConcatOp` to assemble all signals into a single analysis prompt.

## Prerequisites

- `GEMINI_API_KEY` environment variable.

## Usage

```bash
# Analyze Apple (default)
export GEMINI_API_KEY=your_key_here
go run ./examples/stock-analyzer

# Analyze a specific ticker
go run ./examples/stock-analyzer --ticker TSLA
```

## DAG Structure

1. **Input**: Ticker symbol via `ContextValOp`.
2. **Setup**: Build Yahoo Finance URLs via `StringConcatOp`.
3. **Fetch**: GET quote and news JSON in parallel.
4. **Extract**: Pull price, previous close, and headline via `JSONExtractOp`.
5. **Score**: Run `AIScoreOp` on the headline.
6. **Calculate**: `SubFloatOp` for price change.
7. **Consolidate**: Chain 10 string concats to build the final AI prompt.
8. **Recommend**: `AIComputeStringToStringOp` produces the final advice.

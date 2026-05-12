# rag-bm25

Retrieval-augmented question answering over a small local knowledge base,
**with source-file citations**.

The example loads every `.txt` file under `testdata/kb/`, tagging each
`library.Document` with `Metadata["source"] = "<filename>.txt"`. The
documents are indexed by an in-memory BM25 retriever (`bm25.go`) and
registered as the process default via `library.SetDefaultRetriever`. The
graph then runs four vertices:

1. lifts the question into the graph (`question_const`)
2. retrieves the top-3 matching passages (`RetrieveOp`, k=3)
3. labels each passage with its source filename and asks the LLM to end with
   a `Sources: <files>` trailer (`BuildRAGPromptOp` + `AIComputeStringToStringOp`)
4. splits the response into answer body and cited filenames
   (`ParseCitationsOp`)

The driver filters cited filenames against the loaded KB (so an LLM
hallucinating `fictional.txt` gets dropped with a stderr warning) and prints
the answer followed by a `Sources: ...` line listing exactly which KB files
the model says it used.

The point of the example is the **pattern**, not BM25 specifically. To swap
in any other backend — pgvector, sqlite-vec, Pinecone, a hosted search
service — implement `library.Retriever` and populate `Document.Metadata`
with whatever the platform returns (`source_url`, `highlights`, scores,
ACLs). Call `SetDefaultRetriever` with your implementation; the graph stays
identical.

## Running

```
export CLAUDE_API_KEY=sk-...
cd examples/rag-bm25
go run . --question "How long does shipping take?"
```

Stderr prints which passages BM25 selected (with their relevance scores) so
you can see retrieval working before the LLM answers. Stdout receives the
answer body followed by a blank line and `Sources: file1.txt, file2.txt`
listing only the KB files the model says materially supported the answer
(off-corpus or hallucinated filenames are filtered).

## Try these

```
go run . --question "How do I return an item?"
go run . --question "Is my heat pump supported?"
go run . --question "What payment methods do you accept?"
go run . --question "My display is blank, what should I do?"
```

Ask something off-corpus to see the grounded behavior:

```
go run . --question "What's the weather like today?"
```

The prompt instructs the model to reply
`"I don't know based on the provided context."` when the retrieved passages
don't cover the question.

## Plugging in your own retriever

```go
type MyRetriever struct{ /* … */ }

func (r *MyRetriever) Retrieve(ctx context.Context, query string, k int) ([]library.Document, error) {
    // call your vector store / search service here
    return []library.Document{
        {
            ID:      hit.PrimaryKey,
            Content: hit.BodyField,
            Score:   hit.RelevanceScore,
            Metadata: map[string]any{
                "source":     hit.SourceFilename,   // used by this example for citations
                "source_url": hit.URL,              // e.g. for clickable citations
                "highlights": hit.MatchedSnippets,  // []string
                "updated_at": hit.UpdatedAt,        // time.Time
            },
        },
    }, nil
}

func main() {
    library.SetDefaultRetriever(&MyRetriever{ /* … */ })
    // ... build and run the graph as in main.go
}
```

`Document.ID` and `Document.Score` are populated by you; `Metadata` is a
free-form bag the framework passes through unchanged. Downstream graph
vertices can wire `Documents` for ID/score/metadata awareness, or `Texts`
(the parallel `[]string`) for the common case of just feeding content into
an AI op.

The citation behavior in this example assumes `Metadata["source"]` holds a
human-readable filename string. If your Retriever uses different metadata
keys (e.g. `url` instead of `source`), update `sourceFilename` in `main.go`
to match — the framework doesn't prescribe any particular schema.

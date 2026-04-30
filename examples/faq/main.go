package main

import (
	"context"
	"fmt"
	"log"

	"github.com/panjf2000/ants/v2"
	"github.com/wwz16/dagor"
	"github.com/wwz16/dagor/graph"

	"github.com/akennis/clawdag-go/library"
)

func main() {
	faqs := []string{
		"Shipping takes 3-5 business days.",
		"Contact support at support@example.com.",
		"Returns must be shipped within 3 days.",
	}

	library.RegisterConst("query_const", "What is the return policy?")
	library.RegisterConst("faq_const", faqs)

	g, err := graph.NewBuilder("faq_lookup").
		Vertex("query_const").Op("query_const").
		Output("Result", "user_question").

		Vertex("faq_const").Op("faq_const").
		Output("Result", "faq_entries").

		Vertex("best_faq").Op("AIBestMatchOp").
		Input("Query", "user_question").
		Input("Candidates", "faq_entries").
		Output("Result", "best_index").
		Output("Reasoning", "match_reasoning").

		Vertex("get_faq_text").Op("SliceAtOp").
		Input("Input", "faq_entries").
		Input("Index", "best_index").
		Output("Result", "faq_answer").

		Build()
	if err != nil {
		log.Fatalf("build graph: %v", err)
	}

	pool, err := ants.NewPool(4)
	if err != nil {
		log.Fatalf("ants pool: %v", err)
	}
	defer pool.Release()

	eng, err := dagor.NewEngine(g, pool)
	if err != nil {
		log.Fatalf("new engine: %v", err)
	}

	if err := eng.Run(context.Background()); err != nil {
		log.Fatalf("run: %v", err)
	}

	if raw, ok := eng.GetOutput("faq_answer"); ok {
		fmt.Printf("FAQ Answer: %v\n", *(raw.(*string)))
	} else {
		fmt.Println("faq_answer wire not found")
	}

	if raw, ok := eng.GetOutput("match_reasoning"); ok {
		fmt.Printf("Match Reasoning: %v\n", *(raw.(*string)))
	} else {
		fmt.Println("match_reasoning wire not found")
	}
}

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/panjf2000/ants/v2"
	"github.com/wwz16/dagor"
	"github.com/wwz16/dagor/graph"
	"github.com/wwz16/dagor/operator"

	"github.com/akennis/clawdag-go/library"
)

func init() {
	operator.RegisterOp[library.AddOp]()
}

func main() {
	var a, b float64
	fmt.Print("a: ")
	if _, err := fmt.Scan(&a); err != nil {
		log.Fatalf("read a: %v", err)
	}
	fmt.Print("b: ")
	if _, err := fmt.Scan(&b); err != nil {
		log.Fatalf("read b: %v", err)
	}

	library.RegisterConst("a_const", a)
	library.RegisterConst("b_const", b)

	g, err := graph.NewBuilder("add").
		Vertex("a_const").Op("a_const").
		Output("Result", "a").

		Vertex("b_const").Op("b_const").
		Output("Result", "b").

		Vertex("add").Op("AddOp").
		Input("A", "a").
		Input("B", "b").
		Output("Result", "result").
		Build()
	if err != nil {
		log.Fatalf("build graph: %v", err)
	}

	pool, err := ants.NewPool(2)
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

	raw, ok := eng.GetOutput("result")
	if !ok {
		log.Fatal("result wire not found")
	}
	fmt.Printf("%v\n", *(raw.(*float64)))
}

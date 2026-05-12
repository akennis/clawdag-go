package library

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingEmbeddingFactory captures every Embedder call and returns a nil
// EmbeddingClient. Tests using it must not invoke Embed on the returned
// client; they only verify wiring (which factory was selected, what
// provider/model/ref was passed).
type recordingEmbeddingFactory struct {
	mu    sync.Mutex
	calls []embeddingFactoryCall
	err   error
}

type embeddingFactoryCall struct {
	provider string
	model    string
	ref      string
}

func (f *recordingEmbeddingFactory) Embedder(_ context.Context, provider, model, ref string) (EmbeddingClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, embeddingFactoryCall{provider: provider, model: model, ref: ref})
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

func (f *recordingEmbeddingFactory) snapshot() []embeddingFactoryCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]embeddingFactoryCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// blockingEmbeddingFactory blocks Embedder on the supplied ctx until
// cancellation, then returns ctx.Err(). Used to assert that
// ResolveEmbeddingClient bounds the factory call with FactoryTimeout.
type blockingEmbeddingFactory struct{}

func (blockingEmbeddingFactory) Embedder(ctx context.Context, _, _, _ string) (EmbeddingClient, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// withEmbeddingFactories swaps the default factory and registry to a clean
// slate for the duration of a test, restoring on cleanup. Mirrors
// withFactories in ai_factory_test.go.
func withEmbeddingFactories(t *testing.T, def EmbeddingClientFactory) {
	t.Helper()
	embeddingFactoryMu.Lock()
	prevDefault := defaultEmbeddingFactory
	prevRegistry := embeddingFactoryRegistry
	defaultEmbeddingFactory = def
	embeddingFactoryRegistry = map[string]EmbeddingClientFactory{}
	embeddingFactoryMu.Unlock()
	t.Cleanup(func() {
		embeddingFactoryMu.Lock()
		defaultEmbeddingFactory = prevDefault
		embeddingFactoryRegistry = prevRegistry
		embeddingFactoryMu.Unlock()
	})
}

func TestResolveEmbeddingFactory_DefaultWhenNoIDSet(t *testing.T) {
	def := &recordingEmbeddingFactory{}
	withEmbeddingFactories(t, def)
	if got := resolveEmbeddingFactory(""); got != def {
		t.Fatalf("resolveEmbeddingFactory(\"\") = %p, want default %p", got, def)
	}
}

func TestResolveEmbeddingFactory_RegisteredIDWins(t *testing.T) {
	def := &recordingEmbeddingFactory{}
	tenant := &recordingEmbeddingFactory{}
	withEmbeddingFactories(t, def)
	RegisterEmbeddingClientFactory("tenant-a", tenant)
	if got := resolveEmbeddingFactory("tenant-a"); got != tenant {
		t.Fatalf("resolveEmbeddingFactory(\"tenant-a\") = %p, want tenant %p", got, tenant)
	}
}

func TestResolveEmbeddingFactory_UnknownIDFallsBackToDefault(t *testing.T) {
	def := &recordingEmbeddingFactory{}
	withEmbeddingFactories(t, def)
	if got := resolveEmbeddingFactory("nope"); got != def {
		t.Fatalf("resolveEmbeddingFactory(\"nope\") = %p, want default %p", got, def)
	}
}

func TestSetDefaultEmbeddingClientFactory_NilResetsToEnv(t *testing.T) {
	withEmbeddingFactories(t, &recordingEmbeddingFactory{})
	SetDefaultEmbeddingClientFactory(nil)
	if _, ok := resolveEmbeddingFactory("").(*EnvEmbeddingClientFactory); !ok {
		t.Fatalf("after SetDefaultEmbeddingClientFactory(nil), default = %T, want *EnvEmbeddingClientFactory", resolveEmbeddingFactory(""))
	}
}

func TestRegisterEmbeddingClientFactory_NilDeregisters(t *testing.T) {
	def := &recordingEmbeddingFactory{}
	tenant := &recordingEmbeddingFactory{}
	withEmbeddingFactories(t, def)
	RegisterEmbeddingClientFactory("tenant-a", tenant)
	RegisterEmbeddingClientFactory("tenant-a", nil)
	if got := resolveEmbeddingFactory("tenant-a"); got != def {
		t.Fatalf("after deregister, resolveEmbeddingFactory(\"tenant-a\") = %p, want default %p", got, def)
	}
}

func TestResolveEmbeddingClient_PassesProviderModelRef(t *testing.T) {
	def := &recordingEmbeddingFactory{}
	withEmbeddingFactories(t, def)
	ctx := WithEmbeddingCredentials(context.Background(), EmbeddingCredentials{Ref: "vault://prod/voyage"})
	if _, err := ResolveEmbeddingClient(ctx, "voyage", "voyage-3"); err != nil {
		t.Fatalf("ResolveEmbeddingClient: %v", err)
	}
	calls := def.snapshot()
	if len(calls) != 1 {
		t.Fatalf("factory calls = %d, want 1", len(calls))
	}
	if calls[0].provider != "voyage" || calls[0].model != "voyage-3" || calls[0].ref != "vault://prod/voyage" {
		t.Fatalf("factory call = %+v, want provider=voyage model=voyage-3 ref=vault://prod/voyage", calls[0])
	}
}

func TestResolveEmbeddingClient_RegisteredFactoryWinsOverDefault(t *testing.T) {
	def := &recordingEmbeddingFactory{}
	tenant := &recordingEmbeddingFactory{}
	withEmbeddingFactories(t, def)
	RegisterEmbeddingClientFactory("tenant-a", tenant)
	ctx := WithEmbeddingCredentials(context.Background(), EmbeddingCredentials{Ref: "ref-x", FactoryID: "tenant-a"})
	if _, err := ResolveEmbeddingClient(ctx, "openai", "text-embedding-3-small"); err != nil {
		t.Fatalf("ResolveEmbeddingClient: %v", err)
	}
	if got := tenant.snapshot(); len(got) != 1 || got[0].ref != "ref-x" {
		t.Fatalf("tenant calls = %+v, want one call with ref=ref-x", got)
	}
	if got := def.snapshot(); len(got) != 0 {
		t.Fatalf("default factory calls = %+v, want none", got)
	}
}

func TestResolveEmbeddingClient_FactoryErrorPropagates(t *testing.T) {
	want := errors.New("vault denied")
	withEmbeddingFactories(t, &recordingEmbeddingFactory{err: want})
	_, err := ResolveEmbeddingClient(context.Background(), "openai", "text-embedding-3-small")
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("ResolveEmbeddingClient err = %v, want wrapping %v", err, want)
	}
}

func TestResolveEmbeddingClient_FactoryTimeoutFires(t *testing.T) {
	withEmbeddingFactories(t, blockingEmbeddingFactory{})
	ctx := WithEmbeddingCredentials(context.Background(), EmbeddingCredentials{FactoryTimeout: 50 * time.Millisecond})
	start := time.Now()
	_, err := ResolveEmbeddingClient(ctx, "voyage", "voyage-3")
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ResolveEmbeddingClient err = %v, want wrapping context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ResolveEmbeddingClient took %v, expected ~50ms", elapsed)
	}
}

func TestResolveEmbeddingClient_ZeroTimeoutDisablesDeadline(t *testing.T) {
	// With FactoryTimeout=0 and a fast (recording) factory, ResolveEmbeddingClient
	// must reach the factory and return its (nil) result rather than producing a
	// deadline error.
	withEmbeddingFactories(t, &recordingEmbeddingFactory{})
	ctx := WithEmbeddingCredentials(context.Background(), EmbeddingCredentials{Ref: "ref", FactoryTimeout: 0})
	if _, err := ResolveEmbeddingClient(ctx, "voyage", "voyage-3"); err != nil {
		t.Fatalf("ResolveEmbeddingClient with FactoryTimeout=0: %v", err)
	}
}

func TestWithEmbeddingCredentials_RoundTrip(t *testing.T) {
	creds := EmbeddingCredentials{Ref: "vault://x", FactoryID: "tenant-a", FactoryTimeout: 750 * time.Millisecond}
	ctx := WithEmbeddingCredentials(context.Background(), creds)
	got := EmbeddingCredentialsFromContext(ctx)
	if got != creds {
		t.Fatalf("credentials round-trip mismatch: got %+v, want %+v", got, creds)
	}
}

func TestEmbeddingCredentialsFromContext_EmptyByDefault(t *testing.T) {
	got := EmbeddingCredentialsFromContext(context.Background())
	if (got != EmbeddingCredentials{}) {
		t.Fatalf("EmbeddingCredentialsFromContext(plain ctx) = %+v, want zero value", got)
	}
}

func TestEnvEmbeddingClientFactory_RejectsUnknownProvider(t *testing.T) {
	f := &EnvEmbeddingClientFactory{}
	_, err := f.Embedder(context.Background(), "openai", "text-embedding-3-small", "")
	if err == nil {
		t.Fatalf("Embedder(openai): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "RegisterEmbeddingClientFactory") {
		t.Fatalf("error %q should mention RegisterEmbeddingClientFactory so users know how to fix", err)
	}
}

func TestRegisterEmbeddingClientFactory_ConcurrentRegistrationIsSafe(t *testing.T) {
	withEmbeddingFactories(t, &recordingEmbeddingFactory{})
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			RegisterEmbeddingClientFactory("id", &recordingEmbeddingFactory{})
			_ = resolveEmbeddingFactory("id")
		}(i)
	}
	wg.Wait()
}

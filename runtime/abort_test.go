package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/provider"
	"github.com/kasra-sh/zendawn-go/session"
)

// blockUntilCancelProvider hangs in Complete until ctx is cancelled.
type blockUntilCancelProvider struct{}

func (b *blockUntilCancelProvider) ID() string { return "mock" }
func (b *blockUntilCancelProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Tools: true, Streaming: true}
}
func (b *blockUntilCancelProvider) Complete(ctx context.Context, req provider.Request, on provider.StreamHandler) (provider.Response, error) {
	<-ctx.Done()
	return provider.Response{}, ctx.Err()
}

func TestAbortCancelsBlockedComplete(t *testing.T) {
	tmp := t.TempDir()
	wd := filepath.Join(tmp, "ws")
	_ = os.MkdirAll(wd, 0o755)

	doc := &config.Document{Single: &config.Agent{
		ID: "a", Instructions: "x",
		Model: config.ModelConfig{Provider: "mock", ID: "m"},
	}}
	rt := New(doc)
	rt.WorkDir = wd
	rt.RegisterProvider(&blockUntilCancelProvider{})

	s, err := rt.Create("a")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	go func() {
		reason, err := rt.Submit(context.Background(), s.ID, session.UserInput{Text: "hi"})
		if err != nil {
			done <- "err:" + err.Error()
			return
		}
		done <- reason
	}()

	// Wait until drain has installed DrainCancel.
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.Lock()
		ready := s.Busy && s.DrainCancel != nil
		s.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("drain did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	rt.Abort(s.ID)

	select {
	case reason := <-done:
		if reason != session.StopAborted {
			t.Fatalf("want aborted, got %q", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not return after Abort")
	}
}

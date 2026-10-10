package usage

import (
	"context"
	"sync/atomic"
	"testing"
)

type drainTestPlugin struct {
	entered chan struct{}
	release chan struct{}
	count   atomic.Int64
}

func (p *drainTestPlugin) HandleUsage(context.Context, Record) {
	if p.count.Add(1) == 1 {
		close(p.entered)
		<-p.release
	}
}

func TestWaitDeliversQueuedRecordsBeforePersistence(t *testing.T) {
	m := NewManager(8)
	plugin := &drainTestPlugin{entered: make(chan struct{}), release: make(chan struct{})}
	m.Register(plugin)
	m.Publish(context.Background(), Record{})
	<-plugin.entered
	m.Publish(context.Background(), Record{})
	m.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Wait(ctx); err == nil {
		t.Fatal("blocked plugin should prevent draining")
	}
	close(plugin.release)
	if err := m.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if plugin.count.Load() != 2 {
		t.Fatal("queued usage was lost")
	}
}

func TestWaitAfterStopWithoutStart(t *testing.T) {
	m := NewManager(0)
	m.Stop()
	if err := m.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

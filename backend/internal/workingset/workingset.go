package workingset

import (
	"context"
	"sync"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

type WorkingSet struct {
	mu          sync.RWMutex
	packages    map[string]*model.Package
	dispatch    chan *model.Package
	subsMu      sync.RWMutex
	subscribers []chan *model.Package
}

func New(queueSize int) *WorkingSet {
	return &WorkingSet{
		packages: make(map[string]*model.Package),
		dispatch: make(chan *model.Package, queueSize),
	}
}

// Subscribe returns a private channel that receives Signal and scheduler dispatches.
// Pool goroutines read from these channels so they do not compete with the public
// Dispatch channel used for monitoring and tests.
func (ws *WorkingSet) Subscribe(queueSize int) <-chan *model.Package {
	ch := make(chan *model.Package, queueSize)
	ws.subsMu.Lock()
	ws.subscribers = append(ws.subscribers, ch)
	ws.subsMu.Unlock()
	return ch
}

func (ws *WorkingSet) Seed(pkgs []*model.Package) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for _, p := range pkgs {
		ws.packages[p.Project+"/"+p.Name] = p
	}
}

// Add inserts a package into the working set if not already present and notifies
// the public Dispatch channel. Pool goroutines are not triggered by Add; they
// rely on Signal (or the scheduler) to pick up work.
func (ws *WorkingSet) Add(pkg *model.Package) {
	key := pkg.Project + "/" + pkg.Name
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if _, exists := ws.packages[key]; exists {
		return
	}
	ws.packages[key] = pkg
	ws.sendPublic(pkg)
}

// Signal updates the package in the working set and notifies both the public
// Dispatch channel and all subscriber (pool) channels.
func (ws *WorkingSet) Signal(pkg *model.Package) {
	key := pkg.Project + "/" + pkg.Name
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.packages[key] = pkg
	ws.send(pkg)
}

func (ws *WorkingSet) Remove(key string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	delete(ws.packages, key)
}

func (ws *WorkingSet) Dispatch() <-chan *model.Package {
	return ws.dispatch
}

func (ws *WorkingSet) StartScheduler(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ws.mu.RLock()
				for _, p := range ws.packages {
					ws.send(p)
				}
				ws.mu.RUnlock()
			}
		}
	}()
}

// sendPublic sends to the public dispatch channel only (non-blocking).
// Must be called with ws.mu held.
func (ws *WorkingSet) sendPublic(pkg *model.Package) {
	select {
	case ws.dispatch <- pkg:
	default:
	}
}

// sendSubscribers sends to all subscriber channels (non-blocking).
// Must be called with ws.mu held.
func (ws *WorkingSet) sendSubscribers(pkg *model.Package) {
	ws.subsMu.RLock()
	for _, ch := range ws.subscribers {
		select {
		case ch <- pkg:
		default:
		}
	}
	ws.subsMu.RUnlock()
}

// send sends to both the public dispatch channel and all subscriber channels.
// Must be called with ws.mu held.
func (ws *WorkingSet) send(pkg *model.Package) {
	ws.sendPublic(pkg)
	ws.sendSubscribers(pkg)
}

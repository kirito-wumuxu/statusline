package server

import (
	"sync"
	"time"

	"statusline/internal/storage"
)

type SignalFunc func(pid int) error
type VerifyFunc func(pid int) bool

type Refresher struct {
	store    *storage.Store
	interval time.Duration
	signalFn SignalFunc
	verifyFn VerifyFunc

	mu      sync.Mutex
	stopCh  chan struct{}
	running bool
}

func NewRefresher(store *storage.Store, interval time.Duration) *Refresher {
	return &Refresher{
		store:    store,
		interval: interval,
		signalFn: defaultSignal,
		verifyFn: defaultVerifyProcess,
		stopCh:   make(chan struct{}),
	}
}

func (r *Refresher) SetSignalFunc(fn SignalFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signalFn = fn
}

func (r *Refresher) SetVerifyFunc(fn VerifyFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.verifyFn = fn
}

func (r *Refresher) Start() {
	r.mu.Lock()
	if r.running || r.interval <= 0 {
		r.mu.Unlock()
		return
	}
	r.running = true
	stopCh := r.stopCh
	r.mu.Unlock()

	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.TriggerAll()
			case <-stopCh:
				return
			}
		}
	}()
}

func (r *Refresher) TriggerAll() int {
	return 0
}

func (r *Refresher) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return
	}
	close(r.stopCh)
	r.running = false
}

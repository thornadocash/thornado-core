package observer

import "sync"

type Delay struct {
	mu      sync.Mutex
	running bool
}

func NewDelay() *Delay {
	return &Delay{
		mu:      sync.Mutex{},
		running: false,
	}
}

func (d *Delay) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.running = true
}

func (d *Delay) Done() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.running = false
}

func (d *Delay) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

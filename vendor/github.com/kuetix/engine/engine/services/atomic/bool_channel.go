package atomic

import (
	"sync"
)

// BoolChannel write sequences of bool into a channel
type BoolChannel struct {
	ch chan bool

	closingCh      chan interface{}
	writersWG      sync.WaitGroup
	writersWGMutex sync.Mutex
}

// NewBoolChannel creates a BoolChannel
func NewBoolChannel() *BoolChannel {
	return &BoolChannel{
		ch:        make(chan bool),
		closingCh: make(chan interface{}),
	}
}

// Read returns the channel to write
func (p *BoolChannel) Read() <-chan bool {
	if p == nil {
		// Handle the nil case, return a closed channel or take other appropriate actions
		emptyChan := make(chan bool)
		close(emptyChan) // return a closed empty channel to avoid blocking
		return emptyChan
	}
	return p.ch
}

// Write into the channel in a different goroutine
func (p *BoolChannel) Write(data bool) {
	go func(data bool) {
		p.writersWGMutex.Lock()
		p.writersWG.Add(1)
		p.writersWGMutex.Unlock()
		defer p.writersWG.Done()

		select {
		case <-p.closingCh:
			return
		default:
		}

		select {
		case <-p.closingCh:
		case p.ch <- data:
		}
	}(data)
}

// Close Closes channel, draining any blocked writers
func (p *BoolChannel) Close() {
	close(p.closingCh)

	go func() {
		for range p.ch {
		}
	}()

	p.writersWGMutex.Lock()
	p.writersWG.Wait()
	p.writersWGMutex.Unlock()

	close(p.ch)
}

// CloseWithoutDraining closes a channel; without draining any pending writers, this method
// will block until reads have unblocked all writers
func (p *BoolChannel) CloseWithoutDraining() {
	close(p.closingCh)

	p.writersWGMutex.Lock()
	p.writersWG.Wait()
	p.writersWGMutex.Unlock()

	close(p.ch)
}

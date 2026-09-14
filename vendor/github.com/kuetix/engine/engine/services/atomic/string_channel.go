package atomic

import (
	"sync"
)

// StringChannel write sequences of string into a channel
type StringChannel struct {
	ch chan string

	closingCh      chan interface{}
	writersWG      sync.WaitGroup
	writersWGMutex sync.Mutex
}

// NewStringChannel creates an StringChannel
//
//goland:noinspection GoUnusedExportedFunction
func NewStringChannel() *StringChannel {
	return &StringChannel{
		ch:        make(chan string),
		closingCh: make(chan interface{}),
	}
}

// Read returns the channel to write
func (p *StringChannel) Read() <-chan string {
	if p == nil {
		// Handle the nil case, return a closed channel or take other appropriate actions
		emptyChan := make(chan string)
		close(emptyChan) // return a closed empty channel to avoid blocking
		return emptyChan
	}
	return p.ch
}

// Write into the channel in a different goroutine
func (p *StringChannel) Write(data string) {
	go func(data string) {
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
func (p *StringChannel) Close() {
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
func (p *StringChannel) CloseWithoutDraining() {
	close(p.closingCh)

	p.writersWGMutex.Lock()
	p.writersWG.Wait()
	p.writersWGMutex.Unlock()

	close(p.ch)
}

package utils

import (
	"context"
)

// UnlimitedChannel defines unlimited channel struct.
type UnlimitedChannel struct {
	in     chan interface{}
	out    chan interface{}
	done   chan struct{}
	deque  *Deque
	ctx    context.Context
	cancel func()
}

// In returns input channel.
func (c *UnlimitedChannel) In() chan<- interface{} {
	return c.in
}

// Out returns output channel.
func (c *UnlimitedChannel) Out() <-chan interface{} {
	return c.out
}

// Close close the unlimited channel.
func (c *UnlimitedChannel) Close() {
	c.cancel()
}

// Done returns done channel.
func (c *UnlimitedChannel) Done() <-chan struct{} {
	return c.done
}

// Len returns length of channel.
// Remember there is no lock, so be careful when you call Len().
func (c *UnlimitedChannel) Len() uint64 {
	return c.deque.Len()
}

// Dump returns data stuck in channel.
// Remember there is no lock, so be careful when you call Dump().
func (c *UnlimitedChannel) Dump() []interface{} {
	return c.deque.Slice()
}

// NewUnlimitedChannel returns an unlimited channel object.
func NewUnlimitedChannel() *UnlimitedChannel {
	ctx, cancel := context.WithCancel(context.Background())
	c := &UnlimitedChannel{
		in:     make(chan interface{}),
		out:    make(chan interface{}),
		done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
		deque:  NewDeque(),
	}
	go func() {
		defer func() {
			c.in = nil
			c.out = nil
			close(c.done)
		}()
		for {
			// Priority Done().
			select {
			case <-ctx.Done():
				return
			default:
			}

			out, ok := c.deque.Head()
			if !ok {
				select {
				case <-ctx.Done():
					return
				case in := <-c.in:
					c.deque.PushBack(in)
				}
				continue
			}

			select {
			case <-ctx.Done():
				return
			case in := <-c.in:
				c.deque.PushBack(in)
			case c.out <- out:
				c.deque.PopFront()
			}
		}
	}()
	return c
}

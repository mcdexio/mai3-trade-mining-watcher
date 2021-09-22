package cacher

import "sync"

// Const defines a constant cacher which is lazy-loaded.
type Const struct {
	sync.Once
	value interface{}
	load  func() interface{}
}

// NewConst returns a const cacher.
func NewConst(load func() interface{}) *Const {
	if load == nil {
		panic("nil loader func")
	}
	return &Const{
		value: nil,
		load:  load,
	}
}

// IsLoaded returns if the const is loaded.
func (c *Const) IsLoaded() bool {
	return c.value != nil
}

// Get returns the cached value pointer.
func (c *Const) Get() interface{} {
	c.Do(func() {
		v := c.load()
		if v == nil {
			panic("invalid loader")
		}
		c.value = v
	})
	return c.value
}

// Clear clears the cached value pointer.
func (c *Const) Clear() {
	c.Once = sync.Once{}
}

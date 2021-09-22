package utils

// Deque defines deque struct.
type Deque struct {
	start uint64
	end   uint64
	cap   uint64
	len   uint64
	data  []interface{}
}

// NewDeque returns a deque object.
func NewDeque() *Deque {
	return &Deque{
		cap:  1,
		data: make([]interface{}, 1),
	}
}

func (d *Deque) copyTo(data []interface{}) {
	if d.len == 0 {
		return
	}
	if d.start < d.end {
		copy(data, d.data[d.start:d.end])
	} else {
		copy(data, d.data[d.start:d.cap])
		copy(data[d.cap-d.start:], d.data[:d.end])
	}
}

func (d *Deque) expand() {
	d.resize(d.cap << 1)
}

func (d *Deque) shrink() {
	d.resize(d.cap >> 1)
}

func (d *Deque) resize(c uint64) {
	data := make([]interface{}, c)
	d.copyTo(data)
	d.cap = c
	d.data = data
	d.start = 0
	d.end = d.len
}

func (d *Deque) next(pos uint64) uint64 {
	return (pos + 1) % d.cap
}

func (d *Deque) prev(pos uint64) uint64 {
	// Because pos type is unsigned and cap is pow of 2, loop back works.
	return (pos - 1) % d.cap
}

// PushBack pushes an object to back.
func (d *Deque) PushBack(obj interface{}) {
	if d.cap == d.len {
		d.expand()
	}
	d.data[d.end] = obj
	d.end = d.next(d.end)
	d.len = d.len + 1
}

// PushFront pushes an object to front.
func (d *Deque) PushFront(obj interface{}) {
	if d.cap == d.len {
		d.expand()
	}
	d.start = d.prev(d.start)
	d.data[d.start] = obj
	d.len = d.len + 1
}

func (d *Deque) checkWhetherShrink() {
	if (d.cap >> 2) > d.len {
		d.shrink()
	}
}

// PopBack pops an object from back.
func (d *Deque) PopBack() (interface{}, bool) {
	if d.len == 0 {
		return nil, false
	}
	d.end = d.prev(d.end)
	ret := d.data[d.end]
	d.data[d.end] = nil
	d.len = d.len - 1
	d.checkWhetherShrink()
	return ret, true
}

// PopFront pops an object from front.
func (d *Deque) PopFront() (interface{}, bool) {
	if d.len == 0 {
		return nil, false
	}
	ret := d.data[d.start]
	d.data[d.start] = nil
	d.start = d.next(d.start)
	d.len = d.len - 1
	d.checkWhetherShrink()
	return ret, true
}

// Head peeks head.
func (d *Deque) Head() (interface{}, bool) {
	if d.len == 0 {
		return nil, false
	}
	return d.data[d.start], true
}

// Back peeks back.
func (d *Deque) Back() (interface{}, bool) {
	if d.len == 0 {
		return nil, false
	}
	return d.data[d.prev(d.end)], true
}

// Len returns length.
func (d *Deque) Len() uint64 {
	return d.len
}

// Slice converts to slice.
func (d *Deque) Slice() []interface{} {
	data := make([]interface{}, d.len)
	d.copyTo(data)
	return data
}

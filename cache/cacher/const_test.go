package cacher

import (
	"crypto/rand"
	"github.com/stretchr/testify/suite"
	"sync"
	"testing"
)

type ConstSuite struct {
	suite.Suite
}

func (s *ConstSuite) TestConst() {
	// should panic with nil load func
	s.Require().Panics(func() {
		NewConst(nil)
	})

	// should panic with load func returning nil
	s.Require().Panics(func() {
		NewConst(func() interface{} { return nil }).Get()
	})

	// should always get the same value.
	bCacher := NewConst(func() interface{} {
		b := make([]byte, 4)
		n, err := rand.Read(b)
		s.Require().Equal(4, n)
		s.Require().Nil(err)
		return &b
	})

	values := make([]*[]byte, 20)
	var wg sync.WaitGroup
	for k := 0; k < 20; k++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			values[i] = (bCacher.Get()).(*[]byte)
		}(k)
	}
	wg.Wait()

	for k := 1; k < 20; k++ {
		s.Require().Equal(values[0], values[k])
	}

	// should get another one after clear.
	bCacher.Clear()
	value := (bCacher.Get()).(*[]byte)
	s.Require().NotEqual(values[0], value)
}

func TestConst(t *testing.T) {
	suite.Run(t, new(ConstSuite))
}

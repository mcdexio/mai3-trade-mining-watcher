package utils

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type DequeTestSuite struct {
	suite.Suite
}

func (s *DequeTestSuite) TestPushBackPopFront() {
	expected := []Deque{
		{
			start: 0,
			end:   0,
			len:   1,
			cap:   1,
			data:  []interface{}{0},
		},
		{
			start: 0,
			end:   0,
			len:   2,
			cap:   2,
			data:  []interface{}{0, 1},
		},
		{
			start: 0,
			end:   3,
			len:   3,
			cap:   4,
			data:  []interface{}{0, 1, 2, nil},
		},
		{
			start: 0,
			end:   0,
			len:   4,
			cap:   4,
			data:  []interface{}{0, 1, 2, 3},
		},
		{
			start: 0,
			end:   5,
			len:   5,
			cap:   8,
			data:  []interface{}{0, 1, 2, 3, 4, nil, nil, nil},
		},
		{
			start: 0,
			end:   6,
			len:   6,
			cap:   8,
			data:  []interface{}{0, 1, 2, 3, 4, 5, nil, nil},
		},
		{
			start: 0,
			end:   7,
			len:   7,
			cap:   8,
			data:  []interface{}{0, 1, 2, 3, 4, 5, 6, nil},
		},
		{
			start: 0,
			end:   0,
			len:   8,
			cap:   8,
			data:  []interface{}{0, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			start: 1,
			end:   0,
			len:   7,
			cap:   8,
			data:  []interface{}{nil, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			start: 2,
			end:   0,
			len:   6,
			cap:   8,
			data:  []interface{}{nil, nil, 2, 3, 4, 5, 6, 7},
		},
		{
			start: 3,
			end:   0,
			len:   5,
			cap:   8,
			data:  []interface{}{nil, nil, nil, 3, 4, 5, 6, 7},
		},
		{
			start: 4,
			end:   0,
			len:   4,
			cap:   8,
			data:  []interface{}{nil, nil, nil, nil, 4, 5, 6, 7},
		},
		{
			start: 5,
			end:   0,
			len:   3,
			cap:   8,
			data:  []interface{}{nil, nil, nil, nil, nil, 5, 6, 7},
		},
		{
			start: 6,
			end:   0,
			len:   2,
			cap:   8,
			data:  []interface{}{nil, nil, nil, nil, nil, nil, 6, 7},
		},
		{
			start: 0,
			end:   1,
			len:   1,
			cap:   4,
			data:  []interface{}{7, nil, nil, nil},
		},
		{
			start: 0,
			end:   0,
			len:   0,
			cap:   2,
			data:  []interface{}{nil, nil},
		},
	}
	d := NewDeque()
	idx := 0
	for i := 0; i < 8; i++ {
		d.PushBack(i)
		s.Require().Equal(expected[idx], *d)
		idx++
	}
	for i := 0; i < 8; i++ {
		d.PopFront()
		s.Require().Equal(expected[idx], *d)
		idx++
	}
}

func (s *DequeTestSuite) TestPushFrontPopBack() {
	expected := []Deque{
		{
			start: 0,
			end:   0,
			len:   1,
			cap:   1,
			data:  []interface{}{0},
		},
		{
			start: 1,
			end:   1,
			len:   2,
			cap:   2,
			data:  []interface{}{0, 1},
		},
		{
			start: 3,
			end:   2,
			len:   3,
			cap:   4,
			data:  []interface{}{1, 0, nil, 2},
		},
		{
			start: 2,
			end:   2,
			len:   4,
			cap:   4,
			data:  []interface{}{1, 0, 3, 2},
		},
		{
			start: 7,
			end:   4,
			len:   5,
			cap:   8,
			data:  []interface{}{3, 2, 1, 0, nil, nil, nil, 4},
		},
		{
			start: 6,
			end:   4,
			len:   6,
			cap:   8,
			data:  []interface{}{3, 2, 1, 0, nil, nil, 5, 4},
		},
		{
			start: 5,
			end:   4,
			len:   7,
			cap:   8,
			data:  []interface{}{3, 2, 1, 0, nil, 6, 5, 4},
		},
		{
			start: 4,
			end:   4,
			len:   8,
			cap:   8,
			data:  []interface{}{3, 2, 1, 0, 7, 6, 5, 4},
		},
		{
			start: 4,
			end:   3,
			len:   7,
			cap:   8,
			data:  []interface{}{3, 2, 1, nil, 7, 6, 5, 4},
		},
		{
			start: 4,
			end:   2,
			len:   6,
			cap:   8,
			data:  []interface{}{3, 2, nil, nil, 7, 6, 5, 4},
		},
		{
			start: 4,
			end:   1,
			len:   5,
			cap:   8,
			data:  []interface{}{3, nil, nil, nil, 7, 6, 5, 4},
		},
		{
			start: 4,
			end:   0,
			len:   4,
			cap:   8,
			data:  []interface{}{nil, nil, nil, nil, 7, 6, 5, 4},
		},
		{
			start: 4,
			end:   7,
			len:   3,
			cap:   8,
			data:  []interface{}{nil, nil, nil, nil, 7, 6, 5, nil},
		},
		{
			start: 4,
			end:   6,
			len:   2,
			cap:   8,
			data:  []interface{}{nil, nil, nil, nil, 7, 6, nil, nil},
		},
		{
			start: 0,
			end:   1,
			len:   1,
			cap:   4,
			data:  []interface{}{7, nil, nil, nil},
		},
		{
			start: 0,
			end:   0,
			len:   0,
			cap:   2,
			data:  []interface{}{nil, nil},
		},
	}
	d := NewDeque()
	idx := 0
	for i := 0; i < 8; i++ {
		d.PushFront(i)
		s.Require().Equal(expected[idx], *d)
		idx++
	}
	for i := 0; i < 8; i++ {
		d.PopBack()
		s.Require().Equal(expected[idx], *d)
		idx++
	}
}

func (s *DequeTestSuite) TestHeadBack() {
	d := NewDeque()

	data, ok := d.Head()
	s.Require().Nil(data)
	s.Require().False(ok)

	data, ok = d.Back()
	s.Require().Nil(data)
	s.Require().False(ok)

	d.PushBack(1)
	d.PushBack(2)

	data, ok = d.Head()
	s.Require().Equal(1, data)
	s.Require().True(ok)

	data, ok = d.Back()
	s.Require().Equal(2, data)
	s.Require().True(ok)
}

func TestDeque(t *testing.T) {
	suite.Run(t, new(DequeTestSuite))
}

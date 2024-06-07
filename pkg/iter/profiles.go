package iter

import (
	"github.com/grafana/dskit/multierror"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/util/loser"
)

type Timestamp interface {
	Timestamp() model.Time
}

type Profile interface {
	Timestamp
	Fingerprint() model.Fingerprint
}

func lessProfile(p1, p2 Profile) bool {
	if p1.Fingerprint() == p2.Fingerprint() {
		return p1.Timestamp() < p2.Timestamp()
	}
	return p1.Fingerprint() < p2.Fingerprint()
}

type MergeIterator[P Profile] struct {
	tree        *loser.Tree[P, Iterator[P]]
	closeErrs   multierror.MultiError
	current     P
	deduplicate bool
}

// NewMergeIterator returns an iterator that k-way merges the given iterators.
// The given iterators must be sorted by series fingerprint and timestamp themselves.
// Optionally, the iterator can deduplicate profiles with the same fingerprints and timestamps.
func NewMergeIterator[P Profile](max P, deduplicate bool, iters ...Iterator[P]) Iterator[P] {
	if len(iters) == 0 {
		return NewEmptyIterator[P]()
	}
	if len(iters) == 1 {
		// No need to merge a single iterator.
		// We should never allow a single iterator to be passed in because
		return iters[0]
	}
	iter := &MergeIterator[P]{
		deduplicate: deduplicate,
		current:     max,
	}
	iter.tree = loser.New(
		iters,
		max,
		func(s Iterator[P]) P {
			return s.At()
		},
		func(p1, p2 P) bool {
			return lessProfile(p1, p2)
		},
		func(s Iterator[P]) {
			if err := s.Close(); err != nil {
				iter.closeErrs.Add(err)
			}
		})
	return iter
}

func (i *MergeIterator[P]) Next() bool {
	for i.tree.Next() {
		next := i.tree.Winner()

		if !i.deduplicate {
			i.current = next.At()
			return true
		}
		if next.At().Fingerprint() != i.current.Fingerprint() ||
			next.At().Timestamp() != i.current.Timestamp() {
			i.current = next.At()
			return true
		}

	}
	return false
}

func (i *MergeIterator[P]) At() P {
	return i.current
}

func (i *MergeIterator[P]) Err() error {
	return i.tree.Err()
}

func (i *MergeIterator[P]) Close() error {
	i.tree.Close()
	return i.closeErrs.Err()
}

type TimeRangedIterator[T Timestamp] struct {
	Iterator[T]
	min, max model.Time
}

func NewTimeRangedIterator[T Timestamp](it Iterator[T], min, max model.Time) Iterator[T] {
	return &TimeRangedIterator[T]{
		Iterator: it,
		min:      min,
		max:      max,
	}
}

func (it *TimeRangedIterator[T]) Next() bool {
	for it.Iterator.Next() {
		if it.At().Timestamp() < it.min {
			continue
		}
		if it.At().Timestamp() > it.max {
			return false
		}
		return true
	}
	return false
}

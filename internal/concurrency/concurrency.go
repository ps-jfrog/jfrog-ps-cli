// Package concurrency provides small, generic helpers for running a bulk
// operation's per-item work (one HTTP call, or a couple, per item) with
// several items in flight at once, instead of one at a time -- the
// difference between a 500-repo delete taking minutes versus seconds.
//
// Every helper requires fn to be independent per item: no shared mutable
// state (maps, slices, counters) touched without fn's own synchronization,
// and no ordering dependency between items (item B must not need item A's
// side effect to have already happened). Bulk operations in this codebase
// that don't hold that property -- e.g. a shared lookup cache mutated
// per-item, or logically sequential stages like access restore's
// users-then-groups-then-permissions -- are left sequential; only the
// independent-item loops inside such a stage are converted.
package concurrency

import (
	"sync"

	"jfrogpscli/internal/progress"
)

// DefaultParallelism bounds how many workers run at once across this
// package's helpers when a caller doesn't need a different bound -- enough
// to meaningfully speed up bulk operations against a JPD without
// overwhelming it or tripping rate limits. Matches the concurrency
// internal/access's backup fetcher has used since before this package
// existed.
const DefaultParallelism = 8

// Map runs fn(items[i]) for every item with up to parallelism goroutines in
// flight at once (parallelism <= 0 uses DefaultParallelism), then flattens
// every item's zero-or-more results back together in original item order,
// regardless of completion order. Every item's fn always runs to
// completion -- Map never aborts early because of one item's outcome, so
// callers whose result type already records per-item success/failure
// (rather than returning a Go error) get every item processed, exactly
// like this codebase's bulk delete/sync/create loops already do
// sequentially. onProgress, if given, is called after each item completes
// (regardless of outcome) with the number of items completed so far (not
// the item's position), so it stays monotonically increasing under
// concurrency. onProgress is called from every worker goroutine, so it
// must be safe for concurrent use -- progress.Bars.Update already is.
func Map[T, R any](items []T, parallelism int, fn func(T) []R, onProgress ...progress.Reporter) []R {
	n := len(items)
	if n == 0 {
		return nil
	}
	if parallelism <= 0 {
		parallelism = DefaultParallelism
	}

	perItem := make([][]R, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelism)
	var progressMu sync.Mutex
	done := 0

	for i, it := range items {
		wg.Add(1)
		go func(i int, it T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			perItem[i] = fn(it)
			// The increment and the report must be one critical section:
			// reporting outside the lock would let two workers race to
			// call onProgress, with no guarantee the one that incremented
			// done first also calls onProgress first -- observed as the
			// bar's count occasionally ticking backward.
			progressMu.Lock()
			done++
			progress.Report(onProgress, done, n)
			progressMu.Unlock()
		}(i, it)
	}
	wg.Wait()

	var out []R
	for _, rs := range perItem {
		out = append(out, rs...)
	}
	return out
}

// One adapts a fn that returns exactly one result per item (the common
// case) to Map's variable-results-per-item shape.
func One[T, R any](items []T, parallelism int, fn func(T) R, onProgress ...progress.Reporter) []R {
	return Map(items, parallelism, func(it T) []R { return []R{fn(it)} }, onProgress...)
}

// MapOrErr is like One, but fn can fail per item; every item still runs to
// completion (one item's error never stops another in-flight or pending
// item), but if any item failed, MapOrErr returns (nil, the lowest-index
// item's error) instead of partial results -- matching this codebase's
// existing read-then-report bulk functions that stop at the first failure
// and return nothing, just checked after the (now concurrent) work
// finishes rather than before each next item starts. Safe to use for
// read-only fn; reconsider for anything that mutates a JPD, since every
// item's mutation still happens even though the caller only sees the first
// error.
func MapOrErr[T, R any](items []T, parallelism int, fn func(T) (R, error), onProgress ...progress.Reporter) ([]R, error) {
	n := len(items)
	if n == 0 {
		return nil, nil
	}
	if parallelism <= 0 {
		parallelism = DefaultParallelism
	}

	results := make([]R, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelism)
	var progressMu sync.Mutex
	done := 0

	for i, it := range items {
		wg.Add(1)
		go func(i int, it T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, err := fn(it)
			results[i] = r
			errs[i] = err
			progressMu.Lock()
			done++
			progress.Report(onProgress, done, n)
			progressMu.Unlock()
		}(i, it)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// MapSliceOrErr is like MapOrErr, but fn produces zero or more results per
// item instead of exactly one -- the error-checked counterpart to Map.
// Every item still runs to completion; if any item failed, MapSliceOrErr
// returns (nil, the lowest-index item's error) instead of partial results.
// Safe for read-only fn; see MapOrErr's caveat for anything that mutates a
// JPD.
func MapSliceOrErr[T, R any](items []T, parallelism int, fn func(T) ([]R, error), onProgress ...progress.Reporter) ([]R, error) {
	n := len(items)
	if n == 0 {
		return nil, nil
	}
	if parallelism <= 0 {
		parallelism = DefaultParallelism
	}

	perItem := make([][]R, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelism)
	var progressMu sync.Mutex
	done := 0

	for i, it := range items {
		wg.Add(1)
		go func(i int, it T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rs, err := fn(it)
			perItem[i] = rs
			errs[i] = err
			progressMu.Lock()
			done++
			progress.Report(onProgress, done, n)
			progressMu.Unlock()
		}(i, it)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var out []R
	for _, rs := range perItem {
		out = append(out, rs...)
	}
	return out, nil
}

package concurrency

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOne_PreservesOrderRegardlessOfCompletionOrder(t *testing.T) {
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	got := One(items, 4, func(i int) int {
		// Later items sleep less, so they'd finish first if order weren't
		// tracked by index.
		time.Sleep(time.Duration(10-i) * time.Millisecond)
		return i * 10
	})
	for i, v := range got {
		if want := i * 10; v != want {
			t.Fatalf("index %d: got %d, want %d", i, v, want)
		}
	}
}

func TestOne_EveryItemRuns(t *testing.T) {
	items := make([]int, 50)
	for i := range items {
		items[i] = i
	}
	var calls atomic.Int64
	got := One(items, 8, func(i int) int {
		calls.Add(1)
		return i
	})
	if calls.Load() != 50 {
		t.Fatalf("expected 50 calls, got %d", calls.Load())
	}
	if len(got) != 50 {
		t.Fatalf("expected 50 results, got %d", len(got))
	}
}

func TestOne_EmptyInput(t *testing.T) {
	got := One([]int{}, 8, func(i int) int { return i })
	if len(got) != 0 {
		t.Fatalf("expected no results, got %v", got)
	}
}

func TestOne_DefaultParallelismWhenNonPositive(t *testing.T) {
	// Just exercise the <=0 branch; correctness of ordering/count is
	// covered above regardless of which parallelism value is used.
	got := One([]int{1, 2, 3}, 0, func(i int) int { return i * i })
	want := []int{1, 4, 9}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestOne_ProgressIsMonotonicAndReachesTotal(t *testing.T) {
	items := make([]int, 30)
	var mu sync.Mutex
	var lastDone int
	var mismatches int
	got := One(items, 6, func(i int) int {
		return i
	}, func(done, total int) {
		if total != 30 {
			t.Errorf("expected total=30, got %d", total)
		}
		mu.Lock()
		defer mu.Unlock()
		if done < lastDone {
			mismatches++
		}
		lastDone = done
	})
	if len(got) != 30 {
		t.Fatalf("expected 30 results, got %d", len(got))
	}
	if lastDone != 30 {
		t.Fatalf("expected progress to reach 30, got %d", lastDone)
	}
	if mismatches > 0 {
		t.Fatalf("progress went backwards %d time(s)", mismatches)
	}
}

func TestMap_FlattensVariableResultsPerItemInOrder(t *testing.T) {
	items := []int{1, 2, 3}
	got := Map(items, 3, func(i int) []string {
		out := make([]string, i)
		for j := 0; j < i; j++ {
			out[j] = fmt.Sprintf("%d-%d", i, j)
		}
		return out
	})
	want := []string{"1-0", "2-0", "2-1", "3-0", "3-1", "3-2"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestMap_ItemWithNoResultsIsSkipped(t *testing.T) {
	items := []int{1, 2, 3, 4}
	got := Map(items, 4, func(i int) []int {
		if i%2 == 0 {
			return nil
		}
		return []int{i}
	})
	want := []int{1, 3}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMapOrErr_AllSucceed(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	got, err := MapOrErr(items, 5, func(i int) (int, error) {
		return i * i, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 4, 9, 16, 25}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestMapOrErr_ReturnsLowestIndexErrorAndDiscardsResults(t *testing.T) {
	items := []int{0, 1, 2, 3, 4}
	var calls atomic.Int64
	got, err := MapOrErr(items, 5, func(i int) (int, error) {
		calls.Add(1)
		if i == 2 || i == 4 {
			return 0, fmt.Errorf("boom at %d", i)
		}
		return i, nil
	})
	if got != nil {
		t.Fatalf("expected nil results on error, got %v", got)
	}
	if err == nil || err.Error() != "boom at 2" {
		t.Fatalf("expected the lowest-index error (\"boom at 2\"), got %v", err)
	}
	// Every item still ran, even though index 2 failed -- errors don't stop
	// other in-flight or pending items.
	if calls.Load() != int64(len(items)) {
		t.Fatalf("expected every item to run, got %d calls", calls.Load())
	}
}

func TestMapOrErr_EmptyInput(t *testing.T) {
	got, err := MapOrErr([]int{}, 4, func(i int) (int, error) { return i, nil })
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil) for empty input, got (%v, %v)", got, err)
	}
}

func TestMapSliceOrErr_FlattensInOrderWhenAllSucceed(t *testing.T) {
	items := []int{1, 2, 3}
	got, err := MapSliceOrErr(items, 3, func(i int) ([]string, error) {
		out := make([]string, i)
		for j := 0; j < i; j++ {
			out[j] = fmt.Sprintf("%d-%d", i, j)
		}
		return out, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"1-0", "2-0", "2-1", "3-0", "3-1", "3-2"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestMapSliceOrErr_ReturnsLowestIndexErrorAndDiscardsResults(t *testing.T) {
	items := []int{0, 1, 2, 3}
	var calls atomic.Int64
	got, err := MapSliceOrErr(items, 4, func(i int) ([]int, error) {
		calls.Add(1)
		if i == 1 {
			return nil, fmt.Errorf("boom at %d", i)
		}
		return []int{i, i}, nil
	})
	if got != nil {
		t.Fatalf("expected nil results on error, got %v", got)
	}
	if err == nil || err.Error() != "boom at 1" {
		t.Fatalf("expected \"boom at 1\", got %v", err)
	}
	if calls.Load() != int64(len(items)) {
		t.Fatalf("expected every item to run, got %d calls", calls.Load())
	}
}

func TestMapSliceOrErr_EmptyInput(t *testing.T) {
	got, err := MapSliceOrErr([]int{}, 4, func(i int) ([]int, error) { return []int{i}, nil })
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil) for empty input, got (%v, %v)", got, err)
	}
}

func TestMapOrErr_ErrorsAreIndependentOfArbitraryError(t *testing.T) {
	// Sanity check that a sentinel error value survives the round trip
	// (errors.Is works on the returned error).
	sentinel := errors.New("sentinel")
	_, err := MapOrErr([]int{1}, 1, func(i int) (int, error) {
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

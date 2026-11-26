package migration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// paced calls Wait for n bytes the given times, spending work between the calls, and
// returns how long it all took.
func paced(t *testing.T, l *byteRateLimiter, n, times int, work time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	for range times {
		require.NoError(t, l.Wait(context.Background(), n))
		time.Sleep(work)
	}
	return time.Since(start)
}

// The work between two waits counts towards the pace: at 1 MiB/s, 64 KiB is due every
// 62.5 ms, and 40 ms of work in between does not add to that.
func TestRateLimiterCountsWorkTowardsThePace(t *testing.T) {
	l := newByteRateLimiter(1)
	took := paced(t, l, 64<<10, 8, 40*time.Millisecond)
	// 8 x 62.5 ms = 500 ms; waiting the full cost after the work would take 820 ms
	require.Greater(t, took, 450*time.Millisecond)
	require.Less(t, took, 700*time.Millisecond)
}

// Work slower than the rate is not slowed down further.
func TestRateLimiterAddsNoWaitToSlowWork(t *testing.T) {
	l := newByteRateLimiter(1)
	took := paced(t, l, 64<<10, 8, 100*time.Millisecond)
	// 8 x 100 ms of work; waiting the full cost after the work would take 1300 ms
	require.Less(t, took, 1000*time.Millisecond)
}

// Idle time earns at most maxRateCredit of credit.
func TestRateLimiterBoundsTheBurstAfterIdling(t *testing.T) {
	l := newByteRateLimiter(1)
	time.Sleep(maxRateCredit + 300*time.Millisecond)
	start := time.Now()
	// 1 MiB is covered by the credit, the next 512 KiB is due 500 ms later
	require.NoError(t, l.Wait(context.Background(), 1<<20))
	require.Less(t, time.Since(start), 100*time.Millisecond)
	require.NoError(t, l.Wait(context.Background(), 512<<10))
	require.Greater(t, time.Since(start), 400*time.Millisecond)
}

// A rate change applies to the bytes being waited for.
func TestRateLimiterAppliesARateChangeWhileWaiting(t *testing.T) {
	l := newByteRateLimiter(MinRateMB)
	go func() {
		time.Sleep(100 * time.Millisecond)
		l.SetRateMB(MaxRateMB)
	}()
	start := time.Now()
	// 1 MiB at 0.5 MiB/s would take 2 s; at 10 MiB/s it is paid by 100 ms
	require.NoError(t, l.Wait(context.Background(), 1<<20))
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestRateLimiterStopsOnCancel(t *testing.T) {
	l := newByteRateLimiter(MinRateMB)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	require.ErrorIs(t, l.Wait(ctx, 10<<20), context.Canceled)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

// Bytes that cost more than maxRateCredit are waited out once, not again and again: at
// 1 MiB/s, 1.5 MiB is due 1.5 s after the pace started.
func TestRateLimiterWaitsOutACostLongerThanTheCredit(t *testing.T) {
	l := newByteRateLimiter(1)
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_ = l.Wait(context.Background(), 3<<19)
		done <- time.Since(start)
	}()
	select {
	case took := <-done:
		require.Greater(t, took, 1300*time.Millisecond)
		require.Less(t, took, 2000*time.Millisecond)
	case <-time.After(4 * time.Second):
		t.Fatal("Wait did not return")
	}
}

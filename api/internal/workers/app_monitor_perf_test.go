package workers

import (
	"context"
	"crypto/rand"
	"math/big"
	"runtime"
	"sync"
	"testing"
	"time"
)

// simulateOriginal mimics the current unbounded goroutine creation
func simulateOriginal(numApps int, concurrency int, jitterMax time.Duration, workDur time.Duration) {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < numApps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Jitter BEFORE semaphore acquisition
			n, err := rand.Int(rand.Reader, big.NewInt(int64(jitterMax)))
			jitter := int64(0)
			if err == nil {
				jitter = n.Int64()
			}
			time.Sleep(time.Duration(jitter))

			sem <- struct{}{}
			defer func() { <-sem }()

			// Simulate work
			time.Sleep(workDur)
		}(i)
	}
	wg.Wait()
}

// simulateWorkerPool mimics the worker pool implementation
func simulateWorkerPool(ctx context.Context, numApps int, concurrency int, jitterMax time.Duration, workDur time.Duration) {
	type app struct {
		id int
	}
	appChan := make(chan app)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range appChan {
				// Jitter in worker
				n, err := rand.Int(rand.Reader, big.NewInt(int64(jitterMax)))
				jitter := int64(0)
				if err == nil {
					jitter = n.Int64()
				}
				time.Sleep(time.Duration(jitter))
				// Simulate work
				time.Sleep(workDur)
			}
		}()
	}

	for i := 0; i < numApps; i++ {
		select {
		case appChan <- app{id: i}:
		case <-ctx.Done():
			break
		}
	}
	close(appChan)
	wg.Wait()
}

func BenchmarkHealthCheck(b *testing.B) {
	numApps := 100
	concurrency := 10
	jitterMax := 10 * time.Millisecond
	workDur := 5 * time.Millisecond

	b.Run("Original", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			simulateOriginal(numApps, concurrency, jitterMax, workDur)
		}
	})

	b.Run("WorkerPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			simulateWorkerPool(context.Background(), numApps, concurrency, jitterMax, workDur)
		}
	})
}

func TestGoroutineUsage(t *testing.T) {
	numApps := 200
	concurrency := 10
	jitterMax := 50 * time.Millisecond
	workDur := 10 * time.Millisecond

	t.Run("OriginalGoroutinePeak", func(t *testing.T) {
		startGR := runtime.NumGoroutine()
		done := make(chan bool)
		go func() {
			simulateOriginal(numApps, concurrency, jitterMax, workDur)
			done <- true
		}()

		time.Sleep(20 * time.Millisecond)
		peakGR := runtime.NumGoroutine() - startGR
		t.Logf("Original Peak Goroutines: %d", peakGR)
		if peakGR < 100 {
			t.Errorf("Expected many goroutines, got %d", peakGR)
		}
		<-done
	})

	t.Run("WorkerPoolGoroutinePeak", func(t *testing.T) {
		startGR := runtime.NumGoroutine()
		done := make(chan bool)
		go func() {
			simulateWorkerPool(context.Background(), numApps, concurrency, jitterMax, workDur)
			done <- true
		}()

		time.Sleep(20 * time.Millisecond)
		peakGR := runtime.NumGoroutine() - startGR
		t.Logf("WorkerPool Peak Goroutines: %d", peakGR)
		if peakGR > concurrency+5 {
			t.Errorf("Expected around %d goroutines, got %d", concurrency, peakGR)
		}
		<-done
	})
}

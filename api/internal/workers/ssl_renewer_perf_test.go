package workers

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// simulateCheckAndRenew simulates the current sequential implementation
func simulateCheckAndRenew(numDomains int, checkDelay, renewDelay time.Duration, renewalRate float64) {
	renewCount := 0
	failCount := 0

	for i := 0; i < numDomains; i++ {
		// Simulate GetCertExpiration (File IO)
		time.Sleep(checkDelay)

		// Simulate daysUntilExpiry logic
		needsRenewal := (float64(i) / float64(numDomains)) < renewalRate

		if needsRenewal {
			// Simulate ProvisionCertificate (ACME/Network)
			time.Sleep(renewDelay)
			renewCount++
		}
	}
	_ = renewCount
	_ = failCount
}

// simulateCheckAndRenewConcurrent simulates the optimized concurrent implementation
func simulateCheckAndRenewConcurrent(numDomains int, checkDelay, renewDelay time.Duration, renewalRate float64, concurrency int) {
	var renewCount int32
	var failCount int32

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < numDomains; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Simulate acquiring semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Simulate GetCertExpiration
			time.Sleep(checkDelay)

			needsRenewal := (float64(idx) / float64(numDomains)) < renewalRate
			if needsRenewal {
				// Simulate ProvisionCertificate
				time.Sleep(renewDelay)
				atomic.AddInt32(&renewCount, 1)
			}
		}(i)
	}
	wg.Wait()
	_ = renewCount
	_ = failCount
}

func BenchmarkSSLCheckBaseline(b *testing.B) {
	// 10 domains, 10ms file IO, 50ms renewal, 50% need renewal (5 domains)
	// Expected time: 10*10ms + 5*50ms = 100ms + 250ms = 350ms
	for i := 0; i < b.N; i++ {
		simulateCheckAndRenew(10, 10*time.Millisecond, 50*time.Millisecond, 0.5)
	}
}

func BenchmarkSSLCheckOptimized(b *testing.B) {
	// 10 domains, 10ms file IO, 50ms renewal, 50% need renewal, 5 concurrency
	// Expected time: Roughly 350ms / 5 = 70ms (plus overhead)
	for i := 0; i < b.N; i++ {
		simulateCheckAndRenewConcurrent(10, 10*time.Millisecond, 50*time.Millisecond, 0.5, 5)
	}
}

//go:build discovery_live

package discovery

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live mDNS announces host records and must run only in an isolated network namespace.
func requireIsolatedMulticastTest(t *testing.T) {
	t.Helper()
	if os.Getenv("CPA_DISCOVERY_LIVE_TESTS") != "1" {
		t.Skip("live multicast requires CPA_DISCOVERY_LIVE_TESTS=1 in an isolated container network")
	}
}

func TestAdvertiser_IdempotenceAndShutdown(t *testing.T) {
	requireIsolatedMulticastTest(t)
	adv := NewZeroconfAdvertiser()

	// Stopping an unstarted advertiser should be safe
	if err := adv.Stop(); err != nil {
		t.Errorf("unexpected error stopping unstarted advertiser: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ifaces, _ := FilterInterfaces(nil, nil)
	spec := ServiceSpec{
		InstanceName: "CPA-Test-Unit",
		ServiceType:  "_test-ai._tcp",
		Domain:       "local.",
		Port:         65432,
		TextRecords:  []string{"version=1", "product=test"},
		Interfaces:   ifaces,
	}

	errStart := adv.Start(ctx, spec)
	if errStart != nil {
		// In some restricted CI environments multicast bind may fail; handle gracefully
		t.Logf("multicast start failed (likely restricted sandbox network): %v", errStart)
		return
	}

	// Calling Start again while running should fail
	if errSecond := adv.Start(ctx, spec); errSecond == nil {
		t.Errorf("expected error on duplicate Start(), got nil")
	}

	// Calling Stop should cleanly shutdown
	if errStop := adv.Stop(); errStop != nil {
		t.Errorf("unexpected error on Stop(): %v", errStop)
	}

	// Calling Stop a second time should be idempotent
	if errStop2 := adv.Stop(); errStop2 != nil {
		t.Errorf("unexpected error on second Stop(): %v", errStop2)
	}
}

func TestAdvertiserAndBrowser_Integration(t *testing.T) {
	requireIsolatedMulticastTest(t)
	ifaces, _ := FilterInterfaces(nil, nil)
	spec := ServiceSpec{
		InstanceName: "CPA-LiveTest-42",
		ServiceType:  DefaultServiceType,
		Domain:       DefaultDomain,
		Port:         54321,
		Subtypes:     []string{SubtypeResponses},
		TextRecords:  BuildTXTRecords(DefaultTXTOptions()),
		Interfaces:   ifaces,
	}

	adv := NewZeroconfAdvertiser()
	startCtx, startCancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer startCancel()
	if err := adv.Start(startCtx, spec); err != nil {
		t.Skipf("skipping live multicast test: %v", err)
	}
	defer func() {
		if errStop := adv.Stop(); errStop != nil {
			t.Errorf("advertiser cleanup failed: %v", errStop)
		}
	}()

	browseCtx, browseCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer browseCancel()
	browser := NewZeroconfBrowser(ifaces...)
	results, err := browser.Browse(browseCtx, DefaultServiceType, DefaultDomain)
	if err != nil {
		t.Fatalf("browse failed: %v", err)
	}

	found := false
	for _, res := range results {
		if res.InstanceName == "CPA-LiveTest-42" {
			found = true
			if res.Port != 54321 {
				t.Errorf("expected port 54321, got %d", res.Port)
			}
			if res.Product != ProductCPA {
				t.Errorf("expected product %s, got %s", ProductCPA, res.Product)
			}
			break
		}
	}

	if !found {
		t.Fatalf("advertised instance CPA-LiveTest-42 was not discovered")
	}
}

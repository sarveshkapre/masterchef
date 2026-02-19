package control

import "testing"

func TestLeakDetectionReportsLeakAfterThreshold(t *testing.T) {
	store := NewLeakDetectionStore()
	if _, err := store.SetPolicy(LeakDetectionPolicy{
		MinSamples:               3,
		MemoryGrowthPercent:      20,
		GoroutineGrowthPercent:   20,
		FileDescriptorGrowthPerc: 20,
	}); err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	inputs := []ResourceSnapshot{
		{Component: "scheduler", MemoryMB: 100, Goroutines: 80, OpenFDs: 40},
		{Component: "scheduler", MemoryMB: 120, Goroutines: 95, OpenFDs: 50},
		{Component: "scheduler", MemoryMB: 140, Goroutines: 110, OpenFDs: 55},
	}
	var report LeakReport
	var err error
	for _, input := range inputs {
		report, err = store.Observe(input)
		if err != nil {
			t.Fatalf("observe failed: %v", err)
		}
	}
	if !report.LeakDetected || len(report.Reasons) == 0 {
		t.Fatalf("expected leak detection report, got %+v", report)
	}
}

func TestLeakDetectionNoLeakWithStableMetrics(t *testing.T) {
	store := NewLeakDetectionStore()
	if _, err := store.SetPolicy(LeakDetectionPolicy{
		MinSamples:               3,
		MemoryGrowthPercent:      30,
		GoroutineGrowthPercent:   30,
		FileDescriptorGrowthPerc: 30,
	}); err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	inputs := []ResourceSnapshot{
		{Component: "dispatcher", MemoryMB: 200, Goroutines: 150, OpenFDs: 70},
		{Component: "dispatcher", MemoryMB: 202, Goroutines: 151, OpenFDs: 72},
		{Component: "dispatcher", MemoryMB: 205, Goroutines: 152, OpenFDs: 73},
	}
	for _, input := range inputs {
		if _, err := store.Observe(input); err != nil {
			t.Fatalf("observe failed: %v", err)
		}
	}
	reports := store.Reports()
	if len(reports) != 1 || reports[0].LeakDetected {
		t.Fatalf("expected no leak detected for stable metrics, got %+v", reports)
	}
}

package main

import (
	"testing"
)

func TestPrimaryMonitorRectCoversOrigin(t *testing.T) {
	area, ok := primaryMonitorRect()
	if !ok {
		t.Skip("no primary monitor reported")
	}
	if area.Dx() <= 0 || area.Dy() <= 0 {
		t.Fatalf("empty primary monitor: %v", area)
	}
	// Windows keeps the primary monitor's top left at the virtual origin.
	if area.Min.X != 0 || area.Min.Y != 0 {
		t.Fatalf("primary monitor should start at 0,0: %v", area)
	}
}

func TestIndicatorRegionIsInsideThePrimaryMonitor(t *testing.T) {
	area, ok := primaryMonitorRect()
	if !ok {
		t.Skip("no primary monitor reported")
	}
	const size = 320
	region := rectWithin(area, size)
	if !region.In(area) {
		t.Fatalf("indicator region %v escapes the primary monitor %v", region, area)
	}
	if region.Dx() != size || region.Dy() != size {
		t.Fatalf("indicator region size: %v", region)
	}
	// Its corner must be the monitor's own bottom right, not the virtual
	// desktop's (extra monitors extend the desktop to the side).
	if region.Max != area.Max {
		t.Fatalf("indicator region must sit at the primary's bottom right: %v vs %v", region, area)
	}
}

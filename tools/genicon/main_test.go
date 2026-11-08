package main

import "testing"

// The enlarged drawing on the macOS plate must stay inside the plate, with
// some of the plate left around it, in both variants.
func TestMacDrawingStaysOnPlate(t *testing.T) {
	for _, small := range []bool{false, true} {
		ss := shapes(22.5, small)
		plate := ss[0]
		closest := 100.0
		for y := 0.0; y <= 100; y += 0.25 {
			for x := 0.0; x <= 100; x += 0.25 {
				for _, s := range ss[1:] {
					if s.dist(x, y) < 0 {
						closest = min(closest, -plate.dist(x, y))
						break
					}
				}
			}
		}
		if closest < 1 {
			t.Errorf("small=%v: the drawing comes within %.2f of the plate's edge", small, closest)
		}
		t.Logf("small=%v: scale %.3f, closest to the edge %.2f", small, macScale(small), closest)
	}
}

package main

import (
	"math"
	"testing"
	"time"
)

func TestTenSecondAverageLookbackAndBucketWidths(t *testing.T) {
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{Characters: []string{"A", "Outside"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{
			{Second: start.Unix() - 10, Outgoing: 10000}, // Too old for any displayed average.
			{Second: start.Unix() - 9, Outgoing: 90, Incoming: 10},
			{Second: start.Unix() - 1, Outgoing: 100},
			{Second: start.Unix(), Outgoing: 50, Incoming: 20},
			{Second: start.Unix() + 8, Outgoing: 200, Incoming: 40},
			{Second: start.Unix() + 16, Outgoing: 1000, Incoming: 100},
			{Second: start.Unix() + 17, Outgoing: 99999}, // Exclusive range end.
		}},
		{Character: "Outside", Points: []damagePoint{{Second: start.Unix() - 1, Incoming: 9999}}},
	}}
	// Known one-second rolling rates. Outgoing 100 at -1 lasts through second 8;
	// outgoing 50 at 0 lasts through second 9. All windows divide by ten.
	wantOut := []float64{24, 15, 15, 15, 15, 15, 15, 15, 35, 25, 20, 20, 20, 20, 20, 20, 120}
	wantIn := []float64{3, 2, 2, 2, 2, 2, 2, 2, 6, 6, 4, 4, 4, 4, 4, 4, 14}
	for _, bucket := range []time.Duration{time.Second, 3 * time.Second, 10 * time.Second, time.Minute} {
		view := buildView(snapshot, start, start.Add(17*time.Second), bucket)
		averaged := view.tenSecondAverage(snapshot)
		if len(averaged) != 1 || averaged[0].Name != "A" {
			t.Fatal("averaging added a character without damage in the selected window")
		}
		for bin, gotOut := range averaged[0].Outgoing {
			first := bin * int(bucket/time.Second)
			end := min(len(wantOut), first+int(bucket/time.Second))
			var out, in float64
			for second := first; second < end; second++ {
				out += wantOut[second]
				in += wantIn[second]
			}
			out /= float64(end - first)
			in /= float64(end - first)
			if math.Abs(gotOut-out) > 1e-10 || math.Abs(averaged[0].Incoming[bin]-in) > 1e-10 {
				t.Fatalf("%s bucket %d: got %.8f out / %.8f in, want %.8f / %.8f", bucket, bin, gotOut, averaged[0].Incoming[bin], out, in)
			}
		}
		if view.Series[0].TotalOutgoing != 1250 || view.Series[0].TotalIncoming != 160 || averaged[0].TotalOutgoing != 1250 {
			t.Fatal("averaging changed raw damage totals")
		}
		averaged[0].Outgoing[0] = -1
		if view.Series[0].Outgoing[0] < 0 {
			t.Fatal("averaging shares and overwrites the raw data arrays")
		}
	}
}

func TestTenSecondAverageIncludesIdleSecondsAndShortRanges(t *testing.T) {
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{Characters: []string{"A"}, Logs: []*parsedLog{{Character: "A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 100}}}}}
	view := buildView(snapshot, start, start.Add(20*time.Second), time.Second)
	for bin, value := range view.tenSecondAverage(snapshot)[0].Outgoing {
		want := float64(0)
		if bin < 10 {
			want = 10
		}
		if value != want {
			t.Fatalf("impulse at second %d = %v DPS, want %v", bin, value, want)
		}
	}
	view = buildView(snapshot, start, start.Add(time.Second), time.Second)
	if got := view.tenSecondAverage(snapshot)[0].Outgoing[0]; got != 10 {
		t.Fatalf("short range changed the ten-second denominator: %v", got)
	}
	view = buildView(snapshot, start.Add(time.Hour), start.Add(time.Hour+time.Second), time.Second)
	if len(view.tenSecondAverage(snapshot)) != 0 {
		t.Fatal("empty range acquired an averaged series")
	}
}

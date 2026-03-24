// soak-plot reads JSONL soak metrics and writes an SVG chart to stdout.
//
// Usage: soak-plot < metrics.jsonl > chart.svg
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
)

type metric struct {
	ElapsedSecs  float64 `json:"elapsed_secs"`
	OpsPerSec    float64 `json:"ops_per_sec"`
	CumOpsPerSec float64 `json:"cum_ops_per_sec"`
	HeapAllocMB  float64 `json:"heap_alloc_mb"`
	TotalOps     int64   `json:"total_ops"`
	TotalErrors  int64   `json:"total_errors"`
}

const (
	width     = 900
	height    = 400
	padLeft   = 70
	padRight  = 20
	padTop    = 40
	padBottom = 50
	plotW     = width - padLeft - padRight
	plotH     = height - padTop - padBottom
)

func main() {
	var points []metric
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var m metric
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		points = append(points, m)
	}
	if len(points) == 0 {
		fmt.Fprintln(os.Stderr, "no data points")
		os.Exit(1)
	}

	maxTime := points[len(points)-1].ElapsedSecs
	if maxTime == 0 {
		maxTime = 1
	}

	// Find max ops/sec across both series
	maxOps := 0.0
	for _, p := range points {
		maxOps = max(maxOps, p.OpsPerSec)
		maxOps = max(maxOps, p.CumOpsPerSec)
	}
	maxOps = ceilNice(maxOps)
	if maxOps == 0 {
		maxOps = 1
	}

	// SVG output
	fmt.Printf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" font-family="monospace" font-size="12">
`, width, height)
	fmt.Println(`<rect width="100%" height="100%" fill="#fafafa"/>`)

	// Title
	last := points[len(points)-1]
	title := fmt.Sprintf("Soak Test: %.0fs, %d ops, %d errors", last.ElapsedSecs, last.TotalOps, last.TotalErrors)
	fmt.Printf(`<text x="%d" y="20" font-size="14" font-weight="bold" text-anchor="middle">%s</text>
`, width/2, title)

	// Axes
	fmt.Printf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#333" stroke-width="1"/>
`, padLeft, padTop, padLeft, padTop+plotH)
	fmt.Printf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#333" stroke-width="1"/>
`, padLeft, padTop+plotH, padLeft+plotW, padTop+plotH)

	// Y-axis gridlines and labels
	for i := 0; i <= 4; i++ {
		y := padTop + plotH - (plotH * i / 4)
		val := maxOps * float64(i) / 4
		fmt.Printf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#ddd" stroke-width="1"/>
`, padLeft, y, padLeft+plotW, y)
		fmt.Printf(`<text x="%d" y="%d" text-anchor="end" fill="#666">%.0f</text>
`, padLeft-5, y+4, val)
	}

	// X-axis labels
	for i := 0; i <= 4; i++ {
		x := padLeft + (plotW * i / 4)
		val := maxTime * float64(i) / 4
		fmt.Printf(`<text x="%d" y="%d" text-anchor="middle" fill="#666">%s</text>
`, x, padTop+plotH+20, fmtDuration(val))
	}

	// Axis titles
	fmt.Printf(`<text x="%d" y="%d" text-anchor="middle" fill="#333">elapsed</text>
`, padLeft+plotW/2, height-5)
	fmt.Printf(`<text x="15" y="%d" text-anchor="middle" fill="#333" transform="rotate(-90,15,%d)">ops/sec</text>
`, padTop+plotH/2, padTop+plotH/2)

	// Plot interval ops/sec
	fmt.Printf(`<polyline fill="none" stroke="#2563eb" stroke-width="1.5" points="`)
	for _, p := range points {
		x := padLeft + int(p.ElapsedSecs/maxTime*float64(plotW))
		y := padTop + plotH - int(p.OpsPerSec/maxOps*float64(plotH))
		fmt.Printf("%d,%d ", x, y)
	}
	fmt.Println(`"/>`)

	// Plot cumulative ops/sec
	fmt.Printf(`<polyline fill="none" stroke="#dc2626" stroke-width="1.5" stroke-dasharray="6,3" points="`)
	for _, p := range points {
		x := padLeft + int(p.ElapsedSecs/maxTime*float64(plotW))
		y := padTop + plotH - int(p.CumOpsPerSec/maxOps*float64(plotH))
		fmt.Printf("%d,%d ", x, y)
	}
	fmt.Println(`"/>`)

	// Legend
	ly := padTop + 15
	fmt.Printf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#2563eb" stroke-width="2"/>
`, padLeft+10, ly, padLeft+30, ly)
	fmt.Printf(`<text x="%d" y="%d" fill="#2563eb">interval ops/sec</text>
`, padLeft+35, ly+4)
	ly += 18
	fmt.Printf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#dc2626" stroke-width="2" stroke-dasharray="6,3"/>
`, padLeft+10, ly, padLeft+30, ly)
	fmt.Printf(`<text x="%d" y="%d" fill="#dc2626">cumulative ops/sec</text>
`, padLeft+35, ly+4)

	fmt.Println(`</svg>`)
}

func fmtDuration(secs float64) string {
	if secs < 60 {
		return fmt.Sprintf("%.0fs", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%.0fm", secs/60)
	}
	return fmt.Sprintf("%.1fh", secs/3600)
}

func ceilNice(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	norm := v / mag
	switch {
	case norm <= 1:
		return mag
	case norm <= 2:
		return 2 * mag
	case norm <= 5:
		return 5 * mag
	default:
		return 10 * mag
	}
}

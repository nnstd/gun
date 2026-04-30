package perf_hooks

import (
	"math"
	"sort"
	"sync"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

var (
	dataMu       sync.Mutex
	dataMap      = map[*jsvalue.JSValue]*histogramData{}
	tickerMap    = map[*jsvalue.JSValue]*time.Ticker{}
	tickerStopCh = map[*jsvalue.JSValue]chan struct{}{}
)

type histogramData struct {
	values    []float64
	min       float64
	max       float64
	sum       float64
	sumSq     float64
	count     int64
	exceeds   int64
	lowest    float64
	highest   float64
	figures   int
	lastDelta time.Time
}

func newHistogramData(lowest, highest float64, figures int) *histogramData {
	return &histogramData{
		lowest:    lowest,
		highest:   highest,
		figures:   figures,
		lastDelta: time.Now(),
	}
}

func (h *histogramData) record(val float64) {
	if val < h.lowest {
		val = h.lowest
	}
	if val > h.highest {
		h.exceeds++
		val = h.highest
	}
	h.values = append(h.values, val)
	h.count++
	h.sum += val
	h.sumSq += val * val
	if val < h.min || h.count == 1 {
		h.min = val
	}
	if val > h.max || h.count == 1 {
		h.max = val
	}
}

func (h *histogramData) percentile(p float64) float64 {
	if h.count == 0 {
		return 0
	}
	sorted := make([]float64, len(h.values))
	copy(sorted, h.values)
	sort.Float64s(sorted)
	idx := int(math.Ceil(float64(len(sorted))*p/100.0)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (h *histogramData) reset() {
	h.values = nil
	h.min = 0
	h.max = 0
	h.sum = 0
	h.sumSq = 0
	h.count = 0
	h.exceeds = 0
	h.lastDelta = time.Now()
}

func getPercentilesMap(hd *histogramData) *jsvalue.JSValue {
	m := jsvalue.NewMap()
	for p := 1; p <= 100; p++ {
		m.MethodCall("set", jsvalue.NewNumber(float64(p)), jsvalue.NewNumber(hd.percentile(float64(p))))
	}
	return m
}

func createHistogramJSValue(hd *histogramData) *jsvalue.JSValue {
	obj := jsvalue.NewObject()

	obj.Set("count", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(float64(hd.count))
	}))
	obj.Set("countBigInt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(float64(hd.count))
	}))
	obj.Set("exceeds", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(float64(hd.exceeds))
	}))
	obj.Set("exceedsBigInt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(float64(hd.exceeds))
	}))
	obj.Set("min", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if hd.count == 0 {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(hd.min)
	}))
	obj.Set("minBigInt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if hd.count == 0 {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(hd.min)
	}))
	obj.Set("max", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if hd.count == 0 {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(hd.max)
	}))
	obj.Set("maxBigInt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if hd.count == 0 {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(hd.max)
	}))
	obj.Set("mean", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if hd.count == 0 {
			return jsvalue.NewNumber(0)
		}
		return jsvalue.NewNumber(hd.sum / float64(hd.count))
	}))
	obj.Set("stddev", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if hd.count < 2 {
			return jsvalue.NewNumber(0)
		}
		mean := hd.sum / float64(hd.count)
		variance := hd.sumSq/float64(hd.count) - mean*mean
		if variance < 0 {
			variance = 0
		}
		return jsvalue.NewNumber(math.Sqrt(variance))
	}))
	obj.Set("percentiles", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return getPercentilesMap(hd)
	}))
	obj.Set("percentilesBigInt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return getPercentilesMap(hd)
	}))

	obj.Set("percentile", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		p := args[0].Number()
		return jsvalue.NewNumber(hd.percentile(p))
	}))
	obj.Set("percentileBigInt", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewNumber(0)
		}
		p := args[0].Number()
		return jsvalue.NewNumber(hd.percentile(p))
	}))
	obj.Set("reset", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		hd.reset()
		return jsvalue.NewUndefined()
	}))

	return obj
}

func createRecordableHistogram(hd *histogramData) *jsvalue.JSValue {
	obj := createHistogramJSValue(hd)

	obj.Set("record", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		hd.record(args[0].Number())
		return jsvalue.NewUndefined()
	}))
	obj.Set("recordDelta", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		delta := time.Since(hd.lastDelta)
		hd.lastDelta = time.Now()
		hd.record(float64(delta))
		return jsvalue.NewUndefined()
	}))
	obj.Set("add", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) < 1 || args[0] == nil {
			return jsvalue.NewUndefined()
		}
		dataMu.Lock()
		defer dataMu.Unlock()
		other, ok := dataMap[args[0]]
		if !ok || other == nil {
			return jsvalue.NewUndefined()
		}
		for _, v := range other.values {
			hd.record(v)
		}
		return jsvalue.NewUndefined()
	}))

	return obj
}

func createIntervalHistogram(resolution float64) *jsvalue.JSValue {
	hd := newHistogramData(0, 1e9, 3)
	obj := createRecordableHistogram(hd)

	dataMu.Lock()
	dataMap[obj] = hd
	dataMu.Unlock()

	obj.Set("enable", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		dataMu.Lock()
		defer dataMu.Unlock()
		if _, exists := tickerMap[obj]; exists {
			return jsvalue.NewUndefined()
		}
		res := time.Duration(resolution * float64(time.Millisecond))
		if res <= 0 {
			res = 10 * time.Millisecond
		}
		ticker := time.NewTicker(res)
		stopCh := make(chan struct{})
		tickerMap[obj] = ticker
		tickerStopCh[obj] = stopCh
		go func() {
			last := time.Now()
			for {
				select {
				case <-ticker.C:
					now := time.Now()
					delay := float64(now.Sub(last))
					last = now
					dataMu.Lock()
					hd.record(delay)
					dataMu.Unlock()
				case <-stopCh:
					return
				}
			}
		}()
		return jsvalue.NewUndefined()
	}))
	obj.Set("disable", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		dataMu.Lock()
		defer dataMu.Unlock()
		if ticker, ok := tickerMap[obj]; ok {
			ticker.Stop()
			delete(tickerMap, obj)
		}
		if stopCh, ok := tickerStopCh[obj]; ok {
			close(stopCh)
			delete(tickerStopCh, obj)
		}
		return jsvalue.NewUndefined()
	}))

	return obj
}

func CreateHistogram(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	lowest := 1.0
	highest := 9007199254740991.0
	figures := 3

	if len(args) > 0 && args[0] != nil {
		opts := args[0]
		if v := opts.Get("lowest"); v != nil && v.Type() != jsvalue.TypeUndefined {
			lowest = v.Number()
		}
		if v := opts.Get("highest"); v != nil && v.Type() != jsvalue.TypeUndefined {
			highest = v.Number()
		}
		if v := opts.Get("figures"); v != nil && v.Type() != jsvalue.TypeUndefined {
			figures = v.Int()
		}
	}

	hd := newHistogramData(lowest, highest, figures)
	obj := createRecordableHistogram(hd)

	dataMu.Lock()
	dataMap[obj] = hd
	dataMu.Unlock()

	return obj
}

func MonitorEventLoopDelay(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	resolution := 10.0

	if len(args) > 0 && args[0] != nil {
		opts := args[0]
		if v := opts.Get("resolution"); v != nil && v.Type() != jsvalue.TypeUndefined {
			resolution = v.Number()
		}
	}

	return createIntervalHistogram(resolution)
}

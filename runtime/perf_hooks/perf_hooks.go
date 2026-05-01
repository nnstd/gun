package perf_hooks

import (
	"fmt"
	"slices"
	"time"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	jserror "github.com/nnstd/gun/runtime/builtin/error"
)

type observer struct {
	callback   *jsvalue.JSValue
	entryTypes []string
	buffered   []*jsvalue.JSValue
}

var (
	processStart time.Time
	timeOriginMs float64
	entries      []perfEntry
	observers    []observer
)

func init() {
	processStart = time.Now()
	timeOriginMs = float64(processStart.UnixMilli())
}

func PerformanceNow() *jsvalue.JSValue {
	return jsvalue.NewNumber(time.Since(processStart).Seconds() * 1000)
}

func TimeOrigin() *jsvalue.JSValue {
	return jsvalue.NewNumber(timeOriginMs)
}

func Mark(name, options *jsvalue.JSValue) *jsvalue.JSValue {
	if name == nil || name.Type() == jsvalue.TypeUndefined {
		panic(jserror.TypeError.Call(jsvalue.NewString("The \"name\" argument must be of type string or an instance of PerformanceMark. Received undefined")))
	}
	if name.Type() != jsvalue.TypeString {
		panic(jserror.TypeError.Call(jsvalue.NewString(fmt.Sprintf("The \"name\" argument must be of type string or an instance of PerformanceMark. Received %s", name.TypeString()))))
	}
	if name.String() == "" {
		panic(jserror.TypeError.Call(jsvalue.NewString("The \"name\" argument must not be empty")))
	}

	startTime := PerformanceNow().Number()
	detail := jsvalue.NewNull()
	if options != nil && options.Type() == jsvalue.TypeObject {
		if st := options.Get("startTime"); st != nil && st.Type() != jsvalue.TypeUndefined {
			startTime = st.Number()
		}
		if d := options.Get("detail"); d != nil && d.Type() != jsvalue.TypeUndefined {
			detail = d
		}
	}

	e := perfEntry{
		name:      name.String(),
		entryType: "mark",
		startTime: startTime,
		duration:  0,
		detail:    detail,
	}
	entries = append(entries, e)

	entryJS := createEntryJSValue(e)
	notifyObservers("mark")
	return entryJS
}

func Measure(name, startOrOptions, end *jsvalue.JSValue) *jsvalue.JSValue {
	if name == nil || name.Type() == jsvalue.TypeUndefined {
		panic(jserror.TypeError.Call(jsvalue.NewString("The \"name\" argument must be of type string or an instance of PerformanceMeasure. Received undefined")))
	}
	if name.Type() != jsvalue.TypeString {
		panic(jserror.TypeError.Call(jsvalue.NewString(fmt.Sprintf("The \"name\" argument must be of type string or an instance of PerformanceMeasure. Received %s", name.TypeString()))))
	}

	var startTs, endTs float64
	var detail *jsvalue.JSValue = jsvalue.NewNull()
	useDuration := false
	var duration float64

	if startOrOptions == nil || startOrOptions.Type() == jsvalue.TypeUndefined {
		startTs = 0
	} else if startOrOptions.Type() == jsvalue.TypeString {
		if t, ok := findMarkStartTime(startOrOptions.String()); ok {
			startTs = t
		} else {
			panic(jserror.Error.Call(jsvalue.NewString(fmt.Sprintf("The mark \"%s\" does not exist", startOrOptions.String()))))
		}
	} else if startOrOptions.Type() == jsvalue.TypeNumber {
		startTs = startOrOptions.Number()
	} else if startOrOptions.Type() == jsvalue.TypeObject {
		if s := startOrOptions.Get("start"); s != nil && s.Type() != jsvalue.TypeUndefined {
			if s.Type() == jsvalue.TypeString {
				if t, ok := findMarkStartTime(s.String()); ok {
					startTs = t
				} else {
					panic(jserror.Error.Call(jsvalue.NewString(fmt.Sprintf("The mark \"%s\" does not exist", s.String()))))
				}
			} else {
				startTs = s.Number()
			}
		}
		if d := startOrOptions.Get("duration"); d != nil && d.Type() == jsvalue.TypeNumber {
			useDuration = true
			duration = d.Number()
		}
		if dt := startOrOptions.Get("detail"); dt != nil && dt.Type() != jsvalue.TypeUndefined {
			detail = dt
		}
	}

	if useDuration {
		endTs = startTs + duration
	} else {
		if end != nil && end.Type() == jsvalue.TypeString {
			if t, ok := findMarkStartTime(end.String()); ok {
				endTs = t
			} else {
				panic(jserror.Error.Call(jsvalue.NewString(fmt.Sprintf("The mark \"%s\" does not exist", end.String()))))
			}
		} else if startOrOptions != nil && startOrOptions.Type() == jsvalue.TypeObject {
			if e := startOrOptions.Get("end"); e != nil && e.Type() != jsvalue.TypeUndefined {
				if e.Type() == jsvalue.TypeString {
					if t, ok := findMarkStartTime(e.String()); ok {
						endTs = t
					} else {
						panic(jserror.Error.Call(jsvalue.NewString(fmt.Sprintf("The mark \"%s\" does not exist", e.String()))))
					}
				} else {
					endTs = e.Number()
				}
			} else {
				endTs = PerformanceNow().Number()
			}
		} else if end != nil && end.Type() == jsvalue.TypeNumber {
			endTs = end.Number()
		} else {
			endTs = PerformanceNow().Number()
		}
	}

	e := perfEntry{
		name:      name.String(),
		entryType: "measure",
		startTime: startTs,
		duration:  endTs - startTs,
		detail:    detail,
	}
	entries = append(entries, e)

	entryJS := createEntryJSValue(e)
	notifyObservers("measure")
	return entryJS
}

func ClearMarks(name *jsvalue.JSValue) {
	filtered := make([]perfEntry, 0, len(entries))
	for _, e := range entries {
		if e.entryType != "mark" {
			filtered = append(filtered, e)
			continue
		}
		if name == nil || name.Type() == jsvalue.TypeUndefined || name.Type() == jsvalue.TypeNull {
			continue
		}
		if e.name == name.String() {
			continue
		}
		filtered = append(filtered, e)
	}
	entries = filtered
}

func ClearMeasures(name *jsvalue.JSValue) {
	filtered := make([]perfEntry, 0, len(entries))
	for _, e := range entries {
		if e.entryType != "measure" {
			filtered = append(filtered, e)
			continue
		}
		if name == nil || name.Type() == jsvalue.TypeUndefined || name.Type() == jsvalue.TypeNull {
			continue
		}
		if e.name == name.String() {
			continue
		}
		filtered = append(filtered, e)
	}
	entries = filtered
}

func ClearResourceTimings(name *jsvalue.JSValue) {
	filtered := make([]perfEntry, 0, len(entries))
	for _, e := range entries {
		if e.entryType != "resource" {
			filtered = append(filtered, e)
			continue
		}
		if name == nil || name.Type() == jsvalue.TypeUndefined || name.Type() == jsvalue.TypeNull {
			continue
		}
		if e.name == name.String() {
			continue
		}
		filtered = append(filtered, e)
	}
	entries = filtered
}

func GetEntries() *jsvalue.JSValue {
	return entriesToJSValue(entries)
}

func GetEntriesByName(name, entryType *jsvalue.JSValue) *jsvalue.JSValue {
	nameStr := ""
	if name != nil && name.Type() != jsvalue.TypeUndefined {
		nameStr = name.String()
	}
	var typeStr string
	if entryType != nil && entryType.Type() != jsvalue.TypeUndefined {
		typeStr = entryType.String()
	}

	var filtered []perfEntry
	for _, e := range entries {
		if e.name != nameStr {
			continue
		}
		if typeStr != "" && e.entryType != typeStr {
			continue
		}
		filtered = append(filtered, e)
	}
	return entriesToJSValue(filtered)
}

func GetEntriesByType(entryType *jsvalue.JSValue) *jsvalue.JSValue {
	typeStr := ""
	if entryType != nil && entryType.Type() != jsvalue.TypeUndefined {
		typeStr = entryType.String()
	}

	var filtered []perfEntry
	for _, e := range entries {
		if e.entryType == typeStr {
			filtered = append(filtered, e)
		}
	}
	return entriesToJSValue(filtered)
}

func SetResourceTimingBufferSize(maxSize *jsvalue.JSValue) {
}

func ToJSON() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("timeOrigin", jsvalue.NewNumber(timeOriginMs))
	obj.Set("entries", entriesToJSValue(entries))
	return obj
}

func notifyObservers(entryType string) {
	for _, obs := range observers {
		if len(obs.entryTypes) == 0 || slices.Contains(obs.entryTypes, entryType) {
			obs.callback.Call(entriesToJSValue(entries))
		}
	}
}

func createEntryJSValue(e perfEntry) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("name", jsvalue.NewString(e.name))
	obj.Set("entryType", jsvalue.NewString(e.entryType))
	obj.Set("startTime", jsvalue.NewNumber(e.startTime))
	obj.Set("duration", jsvalue.NewNumber(e.duration))
	obj.Set("detail", e.detail)
	return obj
}

func findMarkStartTime(name string) (float64, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].entryType == "mark" && entries[i].name == name {
			return entries[i].startTime, true
		}
	}
	return 0, false
}

func entriesToJSValue(filtered []perfEntry) *jsvalue.JSValue {
	elems := make([]*jsvalue.JSValue, len(filtered))
	for i, e := range filtered {
		elems[i] = createEntryJSValue(e)
	}
	return jsvalue.NewArray(elems...)
}

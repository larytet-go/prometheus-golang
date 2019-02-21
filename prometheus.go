package go_utils

import (
	"bytes"
	"fmt"
	zaplogger "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	_ "unsafe" // required to use //go:linkname
)

// Return text compliant with what https://github.com/prometheus produces
// I assume that there are only counters
// For every field I add lines like these:
// # HELP api_hits Total API calls
// # TYPE api_hits counter
// api_hits 0
func PrometheusStructure(data interface{}, skip []string) string {
	skipSet := make(map[string]bool)
	for _, v := range skip {
		skipSet[v] = true
	}

	t := reflect.TypeOf(data)
	v := reflect.ValueOf(data)
	var buffer bytes.Buffer
	for i := 0; i < v.NumField(); i++ {
		fieldName := t.Field(i).Name
		if _, ok := skipSet[fieldName]; ok {
			continue
		}
		tag := string(t.Field(i).Tag)
		filedValue := v.Field(i)
		buffer.WriteString(fmt.Sprintf("HELP %s %s\nTYPE %s counter\n%s %v\n", fieldName, tag, fieldName, fieldName, filedValue))
	}

	return buffer.String()
}

func PrometheusStructure(data interface{}, skip []string) string {
	skipSet := make(map[string]bool)
	for _, v := range skip {
		skipSet[v] = true
	}

	t := reflect.TypeOf(data)
	v := reflect.ValueOf(data)
	var buffer bytes.Buffer
	for i := 0; i < v.NumField(); i++ {
		fieldName := t.Field(i).Name
		fieldComment := ""
		if _, ok := skipSet[fieldName]; ok {
			continue
		}
		tag := string(t.Field(i).Tag)

		if match := reStructureTag.FindStringSubmatch(tag); match != nil {
			fieldName = match[1]
			fieldComment = match[2]
		}

		fieldValue := v.Field(i)
		fieldType := v.Field(i).Type()

		if fieldType == reflect.TypeOf(&histogram.Histogram{}) {
			//reflect.ValueOf(&t).MethodByName("Foo").Call([]reflect.Value{})
			inArgs := make([]reflect.Value, 2)
			inArgs[0] = reflect.ValueOf(fieldName)
			inArgs[1] = reflect.ValueOf(fieldComment)
			output := fieldValue.MethodByName("Prometheus").Call(inArgs)
			s, _ := output[0].Interface().(string)
			buffer.WriteString(s)
		} else {
			buffer.WriteString(fmt.Sprintf("HELP %s %s\nTYPE %s counter\n%s %v\n", fieldName, tag, fieldName, fieldName, fieldValue))
		}
	}

	return buffer.String()
}


// Usage:
// for bucket.Next() {
//	 upperLimit:=GetUpperLimit()
// }
type Bucket interface {
	next() bool
	getUpperLimit() int
	getBin(v int) int
}

type ExponentialBucket struct {
	start     int
	count     int
	step      int
	logStep   float64
	callsNext int
}

func NewExponentialBucket(start int, step int, count int) *ExponentialBucket {
	return &ExponentialBucket{
		start:     start,
		count:     count,
		step:      step,
		logStep:   math.Log(float64(step)),
		callsNext: 0,
	}
}

func (eb *ExponentialBucket) next() bool {
	return (eb.callsNext < eb.count)
}

func (eb *ExponentialBucket) getUpperLimit() int {
	multiplier := math.Pow(float64(eb.step), float64(eb.callsNext))
	res := float64(eb.start) * multiplier
	eb.callsNext++
	return int(res)
}

func (eb *ExponentialBucket) getBin(v int) int {
	if v < eb.start {
		return 0
	}
	r := math.Log(float64(v)/float64(eb.start)) / eb.logStep
	return int(math.Floor(r))
}

type Histogram struct {
	bucket      Bucket
	upperLimits []int
	bins        []int64
	count       int64
	sum         int64
}

func NewHistogram(bucket Bucket) *Histogram {
	h := &Histogram{
		bucket: bucket,
	}
	h.bins = make([]int64, 0)
	h.upperLimits = make([]int, 0)
	for bucket.next() {
		h.bins = append(h.bins, 0)
		upperLimit := bucket.getUpperLimit()
		h.upperLimits = append(h.upperLimits, upperLimit)
	}
	return h
}

func (h *Histogram) GetBins() []int64 {
	return h.bins
}

// Update the buckets
func (h *Histogram) Add(v int) {
	bin := h.bucket.getBin(v)
	if bin >= len(h.bins) {
		bin = len(h.bins) - 1
	}

	// Histogram API is not very fast. Call to atomics does not
	// impact the overall performance too much
	atomic.AddInt64(&(h.bins[bin]), 1)
	atomic.AddInt64(&h.count, 1)
	atomic.AddInt64(&h.sum, int64(v))
}

func (h *Histogram) Sprintf(delim string) string {
	return strings.Trim(strings.Replace(fmt.Sprint(h.bins), " ", delim, -1), "[]")
}

// Return text compliant with what https://github.com/prometheus produces
func (h *Histogram) Prometheus(name string, comment string) string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("# HELP %s %s\n", name, comment))
	buffer.WriteString(fmt.Sprintf("# TYPE %s histogram\n", name))
	for i := 0; i < len(h.bins); i++ {
		value := h.bins[i]
		upperLimit := h.upperLimits[i]
		buffer.WriteString(fmt.Sprintf("%s_latency_bucket{le=\"%d\"} %d\n", name, upperLimit, value))
	}
	buffer.WriteString(fmt.Sprintf("%s_sum %d\n", name, h.sum))
	buffer.WriteString(fmt.Sprintf("%s_count %d\n", name, h.count))
	return buffer.String()
}



type accumulatorCounter struct {
	summ    uint64
	updates uint64
}

// This accumulator is fast, but not thread safe. Race when
// calling Tick() and Add() and between calls to Add() produces not reliable result
// Use InitSync(), TickSync() and AddSync() if thread safety is desired
type Accumulator struct {
	counters []accumulatorCounter
	cursor   uint64
	size     uint64
	count    uint64
	mutex    *sync.Mutex
	Name     string
}

type AccumulatorResult struct {
	Nonzero   bool
	MaxWindow uint64
	Max       uint64
	Results   []uint64
}

func New(name string, size uint64) *Accumulator {
	a := &Accumulator{
		counters: make([]accumulatorCounter, size),
		size:     size,
		count:    0,
		Name:     name,
	}
	a.Reset()
	return a
}

func NewSync(name string, size uint64) *Accumulator {
	a := New(name, size)
	a.mutex = &sync.Mutex{}
	return a
}

func (a *Accumulator) Reset() {
	a.cursor = 0
	a.count = 0
	// Probably faster than call to make()
	for i := uint64(0); i < a.size; i++ {
		a.counters[i].summ = 0
		a.counters[i].updates = 0
	}
}

func (a *Accumulator) Size() uint64 {
	return a.size
}

func (a *Accumulator) incCursor(cursor uint64) uint64 {
	if cursor >= (a.size - 1) {
		return 0
	} else {
		return (cursor + 1)
	}
}

func (a *Accumulator) decCursor(cursor uint64) uint64 {
	if cursor > (0) {
		return cursor - 1
	} else {
		return (a.size - 1)
	}
}

// Return the accumulator for the last Tick
func (a *Accumulator) PeekSumm() uint64 {
	cursor := a.decCursor(a.cursor)
	return a.counters[cursor].summ
}

// Return average for the last Tick
func (a *Accumulator) PeekAverage() uint64 {
	cursor := a.decCursor(a.cursor)
	return (a.counters[cursor].summ / a.counters[cursor].updates)
}

// Return the results - averages over the window of "size" entries
// Use "divider" to normalize the output in the same copy path
func (a *Accumulator) GetAverage(divider uint64) AccumulatorResult {
	return a.getResult(divider, true)
}

// Use "divider" to normalize the output in the same copy path
func (a *Accumulator) GetSumm(divider uint64) AccumulatorResult {
	return a.getResult(divider, false)
}

func (a *Accumulator) GetSummSync(divider uint64) AccumulatorResult {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.getResult(divider, false)
}

// Use "divider" to normalize the output in the same copy path
func (a *Accumulator) getResult(divider uint64, average bool) AccumulatorResult {
	var nonzero bool = false
	if divider == 0 {
		divider = 1
	}
	size := a.size
	var cursor uint64 = a.cursor
	if size > a.count {
		size = a.count
		cursor = a.size
	}
	results := make([]uint64, size)
	max := uint64(0)
	maxWindow := uint64(0)
	for i := uint64(0); i < size; i++ {
		cursor = a.incCursor(cursor)
		updates := a.counters[cursor].updates
		if updates > 0 {
			nonzero = true
			summ := a.counters[cursor].summ
			if maxWindow < summ {
				maxWindow = summ
			}
			var result uint64
			if average {
				result = (summ / (divider * updates))
			} else {
				result = (summ / divider)
			}
			if max < result {
				max = result
			}
			results[i] = result
		} else {
			results[i] = 0
		}
	}
	return AccumulatorResult{
		Results:   results,
		Nonzero:   nonzero,
		Max:       max,
		MaxWindow: maxWindow,
	}
}

func (a *Accumulator) Add(value uint64) {
	cursor := a.cursor
	a.counters[cursor].summ += value
	a.counters[cursor].updates += 1
}

func (a *Accumulator) AddSync(value uint64) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.Add(value)
}

func (a *Accumulator) Tick() {
	cursor := a.incCursor(a.cursor)
	a.cursor = cursor
	a.counters[cursor].summ = 0
	a.counters[cursor].updates = 0
	a.count += 1
}

func (a *Accumulator) TickSync() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.Tick()
}

func SprintfSliceUint64(valueFormat string, columns int, a []uint64) string {
	if valueFormat == "" {
		valueFormat = "%d "
	}
	if columns <= 0 {
		columns = 4
	}
	var buffer bytes.Buffer
	for col, v := range a {
		if (col%columns == 0) && (col != 0) {
			buffer.WriteString("\n")
		}
		buffer.WriteString(fmt.Sprintf(valueFormat, v))
	}
	s := buffer.String()
	s = s[:(len(s) - 1)]
	return s
}

func (a *Accumulator) Sprintf(nameFormat string, noDataFormat string, valueFormat string, columns int, divider uint64, average bool) string {
	var result AccumulatorResult
	if average {
		result = a.GetAverage(divider)
	} else {
		result = a.GetSumm(divider)
	}
	if nameFormat == "" {
		nameFormat = "%s\n\t%v                         \n"
	}
	if noDataFormat == "" {
		noDataFormat = "%s\n\tNo requests in the last %d seconds\n"
	}

	if result.Nonzero {
		return fmt.Sprintf(nameFormat, a.Name, SprintfSliceUint64(valueFormat, columns, result.Results))
	} else {
		return fmt.Sprintf(noDataFormat, a.Name, a.Size())
	}
}

type ExponentialBucket struct {
	Start  int
	Factor int
	Count  int
}

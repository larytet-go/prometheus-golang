package go_utils

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
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

type ExponentialBucket struct {
	Start  int
	Factor int
	Count  int
}

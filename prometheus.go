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

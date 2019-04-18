# prometheus-golang


Given a structure 

	statistics struct {
		ticker                 uint64 `1s tick implemented in the code`
		hits                   uint64 `Count of hits of the HTTP server, includes debug interfaces`
		hitApi                 uint64 `Count of the API calls`
		status5xx              uint64 `Number of 5xx errors returned by the API`
	}
	
Returns

	HELP ticker 1s tick implemented in the code
	TYPE ticker counter
	ticker 0
	HELP hits Count of hits of the HTTP server, includes debug interfaces
	TYPE hits counter
	hits 1
	HELP hitApi 
	TYPE hitApi counter
	hitApi 0
	HELP status5xx 
	TYPE status5xx counter
	status5xx 0

which is accidentally what Prometheus would generate 

```
You can also initialize a structure of Prometheus counters 
var statistics struct {
	Hit                  prometheus.Counter
}

func registerPrometheusCounters(data interface{}) {
	v := reflect.ValueOf(data).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := string(t.Field(i).Tag)
		fieldName := t.Field(i).Name
		if len(tag) == 0 {
			tag = fieldName
		}
		newCounter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: tag,
			Help: tag,
		})
		f := v.Field(i)
		f.Set(reflect.ValueOf(newCounter))
		prometheus.Register(newCounter)
	}
}
registerPrometheusCounters(&statistics)
```

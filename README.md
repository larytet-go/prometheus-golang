# prometheus-golang


Given a structure ```
	statistics struct {
		ticker                 uint64 `1s tick implemented in the code`
		hits                   uint64 `Count of hits of the HTTP server, includes debug interfaces`
		hitApi                 uint64 `Count of the API calls`
		status5xx              uint64 `Number of 5xx errors returned by the API`
```
Returns ```
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
```

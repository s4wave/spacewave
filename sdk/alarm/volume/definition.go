package alarm_volume

import (
	"time"

	alarm_watch "github.com/s4wave/spacewave/sdk/alarm/watch"
)

// Definition is one store health Alarm: the queries its watch runs, the
// function that decides it from their results, its title and its hold time.
type Definition struct {
	// Name is the Alarm's name, such as "volume-gc-stuck".
	Name string
	// Title is the Alarm's title.
	Title string
	// Hold is how long the condition must hold before the Alarm fires.
	Hold time.Duration
	// Queries are the PromQL queries the function reads, by name.
	Queries []*alarm_watch.Query
	// Source is the TypeScript source of the function. It fires while any
	// query returns a series, and lists each volume in the summary.
	Source string
}

// NodeParameters returns the parameters of an alarm/watch node that runs the
// Alarm with the function stored at functionKey, querying the Prometheus
// target or Dashboard at sourceKey.
func (d *Definition) NodeParameters(functionKey, sourceKey string) map[string]string {
	params := map[string]string{
		alarm_watch.ParamFunction: functionKey,
		alarm_watch.ParamTitle:    d.Title,
		alarm_watch.ParamSource:   sourceKey,
		alarm_watch.ParamHold:     d.Hold.String(),
	}
	for _, q := range d.Queries {
		params[alarm_watch.ParamQueryPrefix+q.GetName()] = q.GetExpr()
	}
	return params
}

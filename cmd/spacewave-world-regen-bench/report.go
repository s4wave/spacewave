//go:build !js && !wasip1

package main

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
)

// reportArgs summarize the latency file by the stage each commit ran in.
type reportArgs struct {
	// Path is the database file.
	Path string
}

// BuildFlags binds the flags.
func (a *reportArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "path", Usage: "database file", Destination: &a.Path, Required: true},
	}
}

// Run prints each stage's commit count and latency quantiles.
func (a *reportArgs) Run(*cli.Context) error {
	// Group the latencies by stage, in the order stages first appear.
	f, err := os.Open(latencyPath(a.Path))
	if err != nil {
		return err
	}
	defer f.Close()
	byStage := make(map[string][]time.Duration)
	var order []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.SplitN(sc.Text(), " ", 3)
		if len(fields) != 3 {
			continue
		}
		ns, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return err
		}
		if _, ok := byStage[fields[2]]; !ok {
			order = append(order, fields[2])
		}
		byStage[fields[2]] = append(byStage[fields[2]], time.Duration(ns))
	}
	if err := sc.Err(); err != nil {
		return err
	}

	// Print the quantiles of each.
	fmt.Printf("%-14s %7s %10s %10s %10s\n", "stage", "commits", "p50", "p99", "max")
	for _, stage := range order {
		d := byStage[stage]
		slices.Sort(d)
		q := func(p float64) time.Duration { return d[min(len(d)-1, int(p*float64(len(d))))] }
		fmt.Printf("%-14s %7d %10v %10v %10v\n", stage, len(d), q(0.5).Round(time.Microsecond), q(0.99).Round(time.Microsecond), d[len(d)-1].Round(time.Microsecond))
	}
	return nil
}

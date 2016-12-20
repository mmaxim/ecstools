package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	ui "github.com/gizak/termui"
	"github.com/mmaxim/ecstools/libecs"
	"github.com/reconquest/loreley"
)

func main() {
	rc := mainInner()
	os.Exit(rc)
}

func truncateARN(arn string, enabled bool) string {
	if !enabled {
		return arn
	}
	toks := strings.Split(arn, "/")
	return toks[1]
}

func renderSvcs(ecs *libecs.ECS) (string, string) {

	services, err := ecs.ListServices()
	if err != nil {
		fmt.Printf("failed to list services: %s", err.Error())
	}

	buffer := &bytes.Buffer{}
	w := tabwriter.NewWriter(buffer, 0, 3, 5, ' ', tabwriter.FilterHTML)
	fmt.Fprintf(w, "[Name\tRun\tPend\tTask\tCPU%%\tMemory%%](fg-red)\n")
	for _, s := range services {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%f\t%f\n", s.Name, s.RunningCount, s.PendingCount,
			truncateARN(s.TaskDefinition, true), s.Metrics.CPU, s.Metrics.Memory)
	}
	w.Flush()
	loreley.DelimLeft = "<"
	loreley.DelimRight = ">"
	svcResult, _ := loreley.CompileAndExecuteToString(buffer.String(), nil, nil)

	buffer.Reset()
	w = tabwriter.NewWriter(buffer, 0, 3, 5, ' ', tabwriter.FilterHTML)
	fmt.Fprintf(w, "[Service\tStatus\tTask\tCreated At\tInstance ID\tInstance CPU%%](fg-red)\n")
	for _, svc := range services {
		for _, task := range svc.Tasks {
			ca := task.CreatedAt.Format("01-02-2006 15:04")
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%f%%\n", svc.Name, task.Status,
				truncateARN(task.TaskDefinition, true), ca, task.InstanceMetrics.ID,
				task.InstanceMetrics.CPU)
		}
	}
	w.Flush()
	taskResult, _ := loreley.CompileAndExecuteToString(buffer.String(), nil, nil)

	return svcResult, taskResult
}

func mainInner() int {
	if err := ui.Init(); err != nil {
		panic(err)
	}
	defer ui.Close()

	var clusterName, region string
	var shortArns bool

	flag.StringVar(&clusterName, "cluster", "gregord", "cluster name")
	flag.StringVar(&region, "region", "us-east-1", "AWS region name")
	flag.BoolVar(&shortArns, "short-arns", true, "display only last part of ARN")
	flag.Parse()

	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: clusterName,
		Region:  region,
	})
	if err != nil {
		fmt.Printf("failed to create ECS API object: %s", err.Error())
		return 3
	}

	svcRes, taskRes := renderSvcs(ecs)
	svcui := ui.NewPar(svcRes)
	svcui.Height = 7
	svcui.PaddingLeft = 3
	svcui.PaddingBottom = 1
	svcui.PaddingTop = 1
	svcui.BorderLabel = "Services"
	svcui.BorderFg = ui.ColorYellow

	taskui := ui.NewPar(taskRes)
	taskui.Height = 8
	taskui.PaddingLeft = 3
	taskui.PaddingBottom = 1
	taskui.PaddingTop = 1
	taskui.BorderLabel = "Tasks"
	taskui.BorderFg = ui.ColorYellow

	g := ui.NewGauge()
	g.Percent = 0
	g.Height = 3
	g.Y = 11
	g.BorderLabel = "Refresh"
	g.BarColor = ui.ColorRed
	g.BorderFg = ui.ColorWhite
	g.BorderLabelFg = ui.ColorCyan

	// build
	ui.Body.AddRows(
		ui.NewRow(
			ui.NewCol(12, 0, g),
		),
		ui.NewRow(
			ui.NewCol(12, 0, svcui),
		),
		ui.NewRow(
			ui.NewCol(12, 0, taskui),
		),
	)

	// calculate layout
	ui.Body.Align()

	ui.Render(ui.Body)

	ui.Handle("/sys/kbd/q", func(ui.Event) {
		ui.StopLoop()
	})
	ui.Handle("/timer/1s", func(e ui.Event) {

		t := e.Data.(ui.EvtTimer)
		g.Percent = int((float64(t.Count%10) / 10.0) * 100)
		if t.Count%10 == 0 {
			svcres, taskres := renderSvcs(ecs)
			svcui.Text = svcres
			taskui.Text = taskres
		}
		ui.Render(ui.Body)
	})

	ui.Loop()

	return 0
}

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
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

func getInstanceID(task libecs.Task) string {
	if task.InstanceMetrics != nil {
		return task.InstanceMetrics.ID
	}
	return "n/a"
}

func getInstanceCPU(task libecs.Task) string {
	if task.InstanceMetrics != nil {
		return fmt.Sprintf("%f%%", task.InstanceMetrics.CPU)
	}
	return "n/a"
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
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", svc.Name, task.Status,
				truncateARN(task.TaskDefinition, true), ca, getInstanceID(task),
				getInstanceCPU(task))
		}
	}
	w.Flush()
	taskResult, _ := loreley.CompileAndExecuteToString(buffer.String(), nil, nil)

	return svcResult, taskResult
}

type workerRes struct {
	svcRes  string
	taskRes string
}

func startRefreshWorker(workCh chan struct{}, ecs *libecs.ECS, svcui *widgets.Paragraph) (resCh chan workerRes) {
	resCh = make(chan workerRes, 1)
	go func() {
		for range workCh {
			var res workerRes
			svcui.Title = "Services (refreshing)"
			ui.Render(svcui)
			res.svcRes, res.taskRes = renderSvcs(ecs)
			svcui.Title = "Services"
			ui.Render(svcui)
			resCh <- res
		}
	}()
	workCh <- struct{}{}
	return resCh
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

	workCh := make(chan struct{}, 1)
	width := 120

	svcui := widgets.NewParagraph()
	svcui.PaddingLeft = 3
	svcui.PaddingBottom = 1
	svcui.PaddingTop = 1
	svcui.Title = "Services"
	svcui.SetRect(0, 0, width, 40)
	svcui.TitleStyle.Fg = ui.ColorYellow

	resCh := startRefreshWorker(workCh, ecs, svcui)

	taskui := widgets.NewParagraph()
	taskui.PaddingLeft = 3
	taskui.PaddingBottom = 1
	taskui.PaddingTop = 1
	taskui.Title = "Tasks"
	taskui.TitleStyle.Fg = ui.ColorYellow
	taskui.SetRect(0, 40, width, 80)

	g := widgets.NewGauge()
	g.Percent = 0
	g.Title = "Refresh"
	g.BarColor = ui.ColorRed
	g.TitleStyle.Fg = ui.ColorWhite
	g.BorderStyle.Fg = ui.ColorCyan
	g.SetRect(0, 80, width, 85)

	ui.Render(svcui, taskui, g)

	uiEvents := ui.PollEvents()
	count := 0
	refreshLen := 20
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return 0
			}
		case res := <-resCh:
			svcui.Text = res.svcRes
			taskui.Text = res.taskRes
			ui.Render(svcui, taskui)
		case <-time.After(time.Second):
			g.Percent = int((float64(count%refreshLen) / float64(refreshLen)) * 100)
			if count%10 == 0 {
				workCh <- struct{}{}
			}
			count++
			ui.Render(g)
		}
	}
}

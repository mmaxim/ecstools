package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"strings"

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

func mainInner() int {
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

	services, err := ecs.ListServices()
	if err != nil {
		fmt.Printf("failed to list services: %s", err.Error())
	}

	buffer := &bytes.Buffer{}
	w := tabwriter.NewWriter(buffer, 0, 3, 5, ' ', tabwriter.FilterHTML)
	fmt.Fprintf(w, "<fg 13><bold>Name\tRunning\tPending\tTask\tCPU%%\tMemory%%<reset>\n")
	for _, s := range services {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%f\t%f\n", s.Name, s.RunningCount, s.PendingCount,
			truncateARN(s.TaskDefinition, shortArns), s.Metrics.CPU, s.Metrics.Memory)
	}
	w.Flush()
	fmt.Fprintf(w, "\n")
	w.Flush()

	fmt.Fprintf(w, "<fg 13><bold>Service\tStatus\tTask\tCreated At\tInstance ID\tInstance CPU%%<reset>\n")
	for _, svc := range services {
		for _, task := range svc.Tasks {
			ca := task.CreatedAt.Format("01-02-2006 15:04")
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%f%%\n", svc.Name, task.Status,
				truncateARN(task.TaskDefinition, shortArns), ca, task.InstanceMetrics.ID,
				task.InstanceMetrics.CPU)
		}
	}
	w.Flush()

	loreley.DelimLeft = "<"
	loreley.DelimRight = ">"
	result, err := loreley.CompileAndExecuteToString(buffer.String(), nil, nil)
	if err != nil {
		fmt.Printf("error formating output: %s", err.Error())
		return 3
	}

	fmt.Print(result)

	return 0
}

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mmaxim/ecstools/libecs"
)

func main() {
	rc := mainInner()
	os.Exit(rc)
}

func mainInner() int {
	var clusterName, serviceName, region string
	var shortArns bool

	flag.StringVar(&clusterName, "cluster", "gregord", "cluster name")
	flag.StringVar(&serviceName, "service", "gregord", "srvice name")
	flag.StringVar(&region, "region", "us-east-1", "AWS region name")
	flag.BoolVar(&shortArns, "short-arns", true, "display only last part of ARN")
	flag.Parse()

	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: clusterName,
		Region:  region,
	})
	if err != nil {
		fmt.Printf("failed to create ECS API object: %s\n", err)
		return 3
	}

	tasks, err := ecs.ListTasks(serviceName)
	if err != nil {
		fmt.Printf("failed to list services: %s\n", err)
	}

	output := libecs.NewColorServiceOutputer(shortArns)
	if err := output.DisplayTasks(tasks, os.Stdout); err != nil {
		fmt.Printf("failed to display: %s\n", err)
		return 3
	}

	return 0
}

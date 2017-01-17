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

	output := libecs.NewColorServiceOutputer(shortArns)
	if err := output.Display(services, os.Stdout); err != nil {
		fmt.Println("failed to display: %s", err.Error())
		return 3
	}

	return 0
}

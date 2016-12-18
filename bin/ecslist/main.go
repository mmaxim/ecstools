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

	flag.StringVar(&clusterName, "cluster", "cluster", "cluster name")
	flag.StringVar(&region, "region", "us-east-1", "AWS region name")
	flag.Parse()

	ecs, err := libecs.New(clusterName, region)
	if err != nil {
		fmt.Printf("failed to create ECS API object: %s", err.Error())
		return 3
	}

	services, err := ecs.ListServices()
	if err != nil {
		fmt.Printf("failed to list services: %s", err.Error())
	}

	for _, s := range services {
		fmt.Printf("%s %s\n", s.Name, s.Arn)
	}

	return 0
}

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mmaxim/ecstools/libecs"
)

func main() {
	rc := mainInner()
	os.Exit(rc)
}

func mainInner() int {
	var clusterName, serviceName, region string

	flag.StringVar(&clusterName, "cluster", "gregord", "cluster name")
	flag.StringVar(&serviceName, "service", "gregord", "service name")
	flag.StringVar(&region, "region", "us-east-1", "AWS region name")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Printf("wrong number of arguments, please specify a graph type (cpu or mem)\n")
		return 3
	}
	typ := args[0]

	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: clusterName,
		Region:  region,
	})
	if err != nil {
		fmt.Printf("failed to create ECS API object: %s\n", err)
		return 3
	}

	var res io.Reader
	switch typ {
	case "cpu":
		res, err = ecs.GetServiceCPUGraph(serviceName, time.Hour*24)
	case "mem":
		res, err = ecs.GetServiceMemoryGraph(serviceName, time.Hour*24)
	default:
		fmt.Printf("unknown graph type: %s\n", typ)
		return 3
	}
	if err != nil {
		fmt.Printf("failed to get graph: %s\n", err)
		return 3
	}

	if _, err := io.Copy(os.Stdout, res); err != nil {
		fmt.Printf("failed to write result: %s\n", err)
		return 3
	}
	return 0
}

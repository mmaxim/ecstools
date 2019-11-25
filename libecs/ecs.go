package libecs

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/ecs"
)

type ECSConfig struct {
	Cluster string
	Region  string
}

type ECS struct {
	ecs        *ecs.ECS
	cloudwatch *cloudwatch.CloudWatch
	config     ECSConfig
}

type Task struct {
	Arn             string
	InstanceArn     string
	Status          string
	DesiredStatus   string
	TaskDefinition  string
	CreatedAt       time.Time
	InstanceMetrics *InstanceMetrics
}

type Service struct {
	Name           string
	Arn            string
	RunningCount   int
	PendingCount   int
	TaskDefinition string
	Tasks          []Task
	Metrics        ServiceMetrics
}

type InstanceMetrics struct {
	CPU float64
	ID  string
}

type ServiceMetrics struct {
	CPU    float64
	Memory float64
}

func New(config ECSConfig) (*ECS, error) {
	ret := &ECS{
		config: config,
	}

	sess, err := session.NewSession(&aws.Config{Region: aws.String(config.Region)})
	if err != nil {
		return nil, err
	}
	ret.ecs = ecs.New(sess)
	ret.cloudwatch = cloudwatch.New(sess)

	return ret, nil
}

func (e *ECS) cluster() string {
	return e.config.Cluster
}

func (e *ECS) region() string {
	return e.config.Region
}

func (e *ECS) getInstanceMetrics(instance string) (InstanceMetrics, error) {
	resp, err := e.ecs.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(e.cluster()),
		ContainerInstances: []*string{aws.String(instance)},
	})
	if err != nil {
		return InstanceMetrics{}, err
	}
	id := resp.ContainerInstances[0].Ec2InstanceId
	dim := cloudwatch.Dimension{
		Name:  aws.String("InstanceId"),
		Value: id,
	}
	period := int64(60)
	start := time.Now().Add(-2 * time.Minute)
	end := time.Now()
	cpuResp, err := e.cloudwatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: []*cloudwatch.Dimension{&dim},
		MetricName: aws.String("CPUUtilization"),
		Statistics: []*string{aws.String("Average")},
		Period:     &period,
		StartTime:  &start,
		EndTime:    &end,
		Namespace:  aws.String("AWS/EC2"),
	})
	if err != nil {
		return InstanceMetrics{}, err
	}

	var cpu float64
	if len(cpuResp.Datapoints) > 0 {
		cpu = *cpuResp.Datapoints[0].Average
	} else {
		cpu = 0
	}
	return InstanceMetrics{
		CPU: cpu,
		ID:  aws.StringValue(id),
	}, nil
}

func (e *ECS) ListTasks(serviceName string) ([]Task, error) {
	resp, err := e.ecs.ListTasks(&ecs.ListTasksInput{
		Cluster:     aws.String(e.cluster()),
		ServiceName: aws.String(serviceName),
	})
	if err != nil {
		return nil, err
	}
	if len(resp.TaskArns) == 0 {
		return nil, nil
	}

	respt, err := e.ecs.DescribeTasks(&ecs.DescribeTasksInput{
		Cluster: aws.String(e.cluster()),
		Tasks:   resp.TaskArns,
	})
	if err != nil {
		return nil, err
	}

	var res []Task
	for _, t := range respt.Tasks {
		var im *InstanceMetrics
		if len(aws.StringValue(t.ContainerInstanceArn)) > 0 {
			im = new(InstanceMetrics)
			if *im, err = e.getInstanceMetrics(aws.StringValue(t.ContainerInstanceArn)); err != nil {
				return res, err
			}
		}
		res = append(res, Task{
			Arn:             aws.StringValue(t.TaskArn),
			InstanceArn:     aws.StringValue(t.ContainerInstanceArn),
			Status:          aws.StringValue(t.LastStatus),
			DesiredStatus:   aws.StringValue(t.DesiredStatus),
			TaskDefinition:  aws.StringValue(t.TaskDefinitionArn),
			CreatedAt:       aws.TimeValue(t.CreatedAt).Local(),
			InstanceMetrics: im,
		})
	}

	return res, nil
}

func (e *ECS) getServiceMetrics(svcname string) (ServiceMetrics, error) {
	dims := []*cloudwatch.Dimension{
		&cloudwatch.Dimension{
			Name:  aws.String("ClusterName"),
			Value: aws.String(e.cluster()),
		},
		&cloudwatch.Dimension{
			Name:  aws.String("ServiceName"),
			Value: aws.String(svcname),
		},
	}
	period := int64(60)
	start := time.Now().Add(-2 * time.Minute)
	end := time.Now()
	cpuResp, err := e.cloudwatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: dims,
		MetricName: aws.String("CPUUtilization"),
		Statistics: []*string{aws.String("Average")},
		Period:     &period,
		StartTime:  &start,
		EndTime:    &end,
		Namespace:  aws.String("AWS/ECS"),
	})
	if err != nil {
		return ServiceMetrics{}, err
	}

	memResp, err := e.cloudwatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: dims,
		MetricName: aws.String("MemoryUtilization"),
		Statistics: []*string{aws.String("Average")},
		Period:     &period,
		StartTime:  &start,
		EndTime:    &end,
		Namespace:  aws.String("AWS/ECS"),
	})
	if err != nil {
		return ServiceMetrics{}, err
	}

	var cpu, mem float64
	if len(cpuResp.Datapoints) > 0 {
		cpu = *cpuResp.Datapoints[0].Average
	} else {
		cpu = 0
	}
	if len(memResp.Datapoints) > 0 {
		mem = *memResp.Datapoints[0].Average
	} else {
		mem = 0
	}
	return ServiceMetrics{
		CPU:    cpu,
		Memory: mem,
	}, nil
}

func (e *ECS) getServiceMetricGraph(svcname, metric string, duration time.Duration) (io.Reader, error) {
	metrics := fmt.Sprintf(`
		{
			"title": "%s %s",
			"period": 1,
			"yAxis":{
				"left":{
				   "min":0,
				   "max":100
				}
			 },
			"metrics": [
				[
					"AWS/ECS", 
					"%s", 
					"ClusterName", 
					"%s",
					"ServiceName",
					"%s",
					{
						"id": "m1",
						"stat": "Average"
					}
				],
				[
					".", 
					".", 
					".", 
					".",
					".",
					".",
					{
						"id": "m2",
						"stat": "Maximum"
					}
				],
				[
					".", 
					".", 
					".", 
					".",
					".",
					".",
					{
						"id": "m3",
						"stat": "Minimum"
					}
				]
			]
		}
	`, svcname, metric, metric, e.cluster(), svcname)
	res, err := e.cloudwatch.GetMetricWidgetImage(&cloudwatch.GetMetricWidgetImageInput{
		MetricWidget: aws.String(metrics),
		OutputFormat: aws.String("png"),
	})
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(res.MetricWidgetImage), nil
}

func (e *ECS) GetServiceCPUGraph(svcname string, duration time.Duration) (io.Reader, error) {
	return e.getServiceMetricGraph(svcname, "CPUUtilization", duration)
}

func (e *ECS) GetServiceMemoryGraph(svcname string, duration time.Duration) (io.Reader, error) {
	return e.getServiceMetricGraph(svcname, "MemoryUtilization", duration)
}

func (e *ECS) ListServices() ([]Service, error) {

	// Fetch all service ARNS
	params := &ecs.ListServicesInput{
		Cluster:    aws.String(e.cluster()),
		MaxResults: aws.Int64(100),
	}
	resp, err := e.ecs.ListServices(params)
	if err != nil {
		return nil, err
	}

	var res []Service
	batchSize := 10
	for batchIndex := 0; batchIndex < len(resp.ServiceArns); batchIndex += batchSize {
		var arns []*string
		lim := batchIndex + batchSize
		if lim >= len(resp.ServiceArns) {
			lim = len(resp.ServiceArns)
		}
		for i := batchIndex; i < lim; i++ {
			arns = append(arns, resp.ServiceArns[i])
		}

		// Fetch service descriptions
		sresp, err := e.ecs.DescribeServices(&ecs.DescribeServicesInput{
			Services: arns,
			Cluster:  aws.String(e.cluster()),
		})
		if err != nil {
			return nil, err
		}

		for _, svc := range sresp.Services {
			metrics, err := e.getServiceMetrics(aws.StringValue(svc.ServiceName))
			if err != nil {
				return nil, err
			}
			s := Service{
				Name:           aws.StringValue(svc.ServiceName),
				Arn:            aws.StringValue(svc.ServiceArn),
				RunningCount:   int(aws.Int64Value(svc.RunningCount)),
				PendingCount:   int(aws.Int64Value(svc.PendingCount)),
				TaskDefinition: aws.StringValue(svc.TaskDefinition),
				Metrics:        metrics,
			}
			if s.Tasks, err = e.ListTasks(aws.StringValue(svc.ServiceName)); err != nil {
				return nil, err
			}
			res = append(res, s)
		}
	}

	return res, nil
}

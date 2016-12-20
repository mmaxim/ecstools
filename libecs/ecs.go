package libecs

import (
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
	TaskDefinition  string
	CreatedAt       time.Time
	InstanceMetrics InstanceMetrics
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

	cpu := cpuResp.Datapoints[0].Average
	return InstanceMetrics{
		CPU: *cpu,
		ID:  aws.StringValue(id),
	}, nil
}

func (e *ECS) listTasks(svc *ecs.Service) ([]Task, error) {
	resp, err := e.ecs.ListTasks(&ecs.ListTasksInput{
		Cluster:     aws.String(e.cluster()),
		ServiceName: svc.ServiceName,
	})
	if err != nil {
		return nil, err
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
		im, err := e.getInstanceMetrics(aws.StringValue(t.ContainerInstanceArn))
		if err != nil {
			return res, err
		}
		res = append(res, Task{
			Arn:             aws.StringValue(t.TaskArn),
			InstanceArn:     aws.StringValue(t.ContainerInstanceArn),
			Status:          aws.StringValue(t.LastStatus),
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

	cpu := cpuResp.Datapoints[0].Average
	mem := memResp.Datapoints[0].Average
	return ServiceMetrics{
		CPU:    *cpu,
		Memory: *mem,
	}, nil
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
	var arns []*string
	for _, a := range resp.ServiceArns {
		arns = append(arns, a)
	}

	// Fetch service descriptions
	sresp, err := e.ecs.DescribeServices(&ecs.DescribeServicesInput{
		Services: arns,
		Cluster:  aws.String(e.cluster()),
	})
	if err != nil {
		return nil, err
	}

	var res []Service
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
		if s.Tasks, err = e.listTasks(svc); err != nil {
			return nil, err
		}
		res = append(res, s)
	}

	return res, nil
}

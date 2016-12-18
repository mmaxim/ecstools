package libecs

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
)

type ECS struct {
	ecs     *ecs.ECS
	cluster string
	region  string
}

type Service struct {
	Name           string
	Arn            string
	RunningCount   int
	PendingCount   int
	TaskDefinition string
}

func New(clusterName string, region string) (*ECS, error) {
	ret := &ECS{
		cluster: clusterName,
		region:  region,
	}

	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		return nil, err
	}
	ret.ecs = ecs.New(sess)

	return ret, nil
}

func (e *ECS) ListServices() ([]Service, error) {
	params := &ecs.DescribeClustersInput{
		Clusters: []*string{aws.String(e.cluster)},
	}
	resp, err := e.ecs.DescribeClusters(params)
	if err != nil {
		return nil, err
	}

	var res []Service
	for _, c := range resp.Clusters {
		res = append(res, Service{
			Name: *c.ClusterName,
			Arn:  *c.ClusterArn,
		})
	}

	return res, nil
}

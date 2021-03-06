package aws

import (
	"testing"
	"time"

	awsgo "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/cloud-nuke/util"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/stretchr/testify/assert"
)

func getSubnetsInDifferentAZs(t *testing.T, session *session.Session) (*ec2.Subnet, *ec2.Subnet) {
	subnetOutput, err := ec2.New(session).DescribeSubnets(&ec2.DescribeSubnetsInput{})
	if err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	if len(subnetOutput.Subnets) < 2 {
		assert.Fail(t, "Needs at least 2 subnets to create ELBv2")
	}

	subnet1 := subnetOutput.Subnets[0]

	for i := 1; i < len(subnetOutput.Subnets); i++ {
		subnet2 := subnetOutput.Subnets[i]
		if *subnet1.AvailabilityZone != *subnet2.AvailabilityZone && *subnet1.SubnetId != *subnet2.SubnetId && *subnet1.VpcId == *subnet2.VpcId {
			return subnet1, subnet2
		}
	}

	assert.Fail(t, "Unable to find 2 subnets in different Availability Zones")
	return nil, nil
}

func createTestELBv2(t *testing.T, session *session.Session, name string) elbv2.LoadBalancer {
	svc := elbv2.New(session)

	subnet1, subnet2 := getSubnetsInDifferentAZs(t, session)

	param := &elbv2.CreateLoadBalancerInput{
		Name: awsgo.String(name),
		Subnets: []*string{
			subnet1.SubnetId,
			subnet2.SubnetId,
		},
	}

	result, err := svc.CreateLoadBalancer(param)

	if err != nil {
		assert.Failf(t, "Could not create test ELBv2", errors.WithStackTrace(err).Error())
	}

	if len(result.LoadBalancers) == 0 {
		assert.Failf(t, "Could not create test ELBv2", errors.WithStackTrace(err).Error())
	}

	balancer := *result.LoadBalancers[0]

	err = svc.WaitUntilLoadBalancerAvailable(&elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{balancer.LoadBalancerArn},
	})

	if err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	return balancer
}

func TestListELBv2(t *testing.T) {
	t.Parallel()

	region := getRandomRegion()
	session, err := session.NewSession(&awsgo.Config{
		Region: awsgo.String(region)},
	)

	if err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	elbName := "cloud-nuke-test-" + util.UniqueID()
	balancer := createTestELBv2(t, session, elbName)
	// clean up after this test
	defer nukeAllElbv2Instances(session, []*string{balancer.LoadBalancerArn})

	arns, err := getAllElbv2Instances(session, region, time.Now().Add(1*time.Hour*-1))
	if err != nil {
		assert.Fail(t, "Unable to fetch list of v2 ELBs")
	}

	assert.NotContains(t, awsgo.StringValueSlice(arns), awsgo.StringValue(balancer.LoadBalancerArn))

	arns, err = getAllElbv2Instances(session, region, time.Now().Add(1*time.Hour))
	if err != nil {
		assert.Fail(t, "Unable to fetch list of v2 ELBs")
	}

	assert.Contains(t, awsgo.StringValueSlice(arns), awsgo.StringValue(balancer.LoadBalancerArn))
}

func TestNukeELBv2(t *testing.T) {
	t.Parallel()

	region := getRandomRegion()
	session, err := session.NewSession(&awsgo.Config{
		Region: awsgo.String(region)},
	)
	svc := elbv2.New(session)

	if err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	elbName := "cloud-nuke-test-" + util.UniqueID()
	balancer := createTestELBv2(t, session, elbName)

	_, err = svc.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{
			balancer.LoadBalancerArn,
		},
	})

	if err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	if err := nukeAllElbv2Instances(session, []*string{balancer.LoadBalancerArn}); err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	err = svc.WaitUntilLoadBalancersDeleted(&elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{balancer.LoadBalancerArn},
	})

	if err != nil {
		assert.Fail(t, errors.WithStackTrace(err).Error())
	}

	arns, err := getAllElbv2Instances(session, region, time.Now().Add(1*time.Hour))
	if err != nil {
		assert.Fail(t, "Unable to fetch list of v2 ELBs")
	}

	assert.NotContains(t, awsgo.StringValueSlice(arns), awsgo.StringValue(balancer.LoadBalancerArn))
}

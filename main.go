package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func catchError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {

	/*
		We need to specify data to be passed into the AMI?
		We didn't get the expected web service running when we got to the Autoscaling group...
		Auto Scaling groups come into service quickly! (Less than a second...)
		When using the RunInstances command, you need to specify a min/max count
		When creating a security group, need to specify a description.

	*/
	// Create an EC2 instance that has a web service running

	// Create service configuration
	config := &aws.Config{
		Region: aws.String("us-east-1"),
	}
	// Create universal session
	sess := session.New(config)

	// Create EC2 Service
	svc := ec2.New(sess)

	// Create Key-Pair
	fmt.Println("Creating the Key pair value")
	keyPairOutput, err := svc.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: aws.String("dopeDays"),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("key-pair"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("Name"),
						Value: aws.String("simpleServerKey"),
					},
				},
			},
		},
	})
	catchError(err)

	// Write Key-Pair to files
	keyName := *keyPairOutput.KeyName
	fmt.Println("Creating KEYS")
	keyFile, err := os.Create(keyName + ".pem")
	catchError(err)
	fmt.Println("Writing KEYS!")
	_, err = keyFile.Write([]byte(*keyPairOutput.KeyMaterial))
	catchError(err)

	// Create Sercurity Group
	fmt.Println("Creating Security group.")
	securityGroupOutput, err := svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   aws.String("SimpleHttpService"),
		Description: aws.String("Simple HTTP Security Group"),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("security-group"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("Name"),
						Value: aws.String("simpleServerSG"),
					},
				},
			},
		},
	})
	catchError(err)
	securityGroupID := aws.String(*securityGroupOutput.GroupId)

	//Create Security group rules
	fmt.Println("Adding Security Group rules!")
	_, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: securityGroupID,
		IpPermissions: []*ec2.IpPermission{
			&ec2.IpPermission{
				FromPort:   aws.Int64(80),
				ToPort:     aws.Int64(80),
				IpProtocol: aws.String("tcp"),
				IpRanges: []*ec2.IpRange{
					&ec2.IpRange{
						Description: aws.String("Everyone can access HTTPD"),
						CidrIp:      aws.String("0.0.0.0/0"),
					},
				},
			},
			&ec2.IpPermission{
				FromPort:   aws.Int64(443),
				ToPort:     aws.Int64(443),
				IpProtocol: aws.String("tcp"),
				IpRanges: []*ec2.IpRange{
					&ec2.IpRange{
						Description: aws.String("Everyone can access HTTPD"),
						CidrIp:      aws.String("0.0.0.0/0"),
					},
				},
			},
			&ec2.IpPermission{
				FromPort:   aws.Int64(22),
				ToPort:     aws.Int64(22),
				IpProtocol: aws.String("tcp"),
				IpRanges: []*ec2.IpRange{
					&ec2.IpRange{
						Description: aws.String("Everyone can access HTTPD"),
						CidrIp:      aws.String("0.0.0.0/0"),
					},
				},
			},
		},
	})
	// Create data to be passed to instance.
	data := []byte("#!/bin/bash\nsudo su\nyum update -y\nyum upgrade -y\nyum install httpd -y\nsystemctl start httpd\nsystemctl enable httpd\nchown ec2-user /var/www/*\nchown ec2-user /var/www")
	fmt.Println("Encoding Data!")
	userData := base64.StdEncoding.EncodeToString(data)

	// Creating instance
	fmt.Println("Creating instance")
	runInstanceOutput, err := svc.RunInstances(&ec2.RunInstancesInput{
		ImageId:      aws.String("ami-0c94855ba95c71c99"),
		InstanceType: aws.String("t2.micro"),
		SecurityGroupIds: []*string{
			securityGroupID,
		},
		KeyName: aws.String(keyName),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("instance"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("Name"),
						Value: aws.String("SimpleHTTPServer"),
					},
				},
			},
		},
		UserData: aws.String(userData),
		MinCount: aws.Int64(1),
		MaxCount: aws.Int64(1),
	})
	catchError(err)
	instanceID := *runInstanceOutput.Instances[0].InstanceId
	fmt.Println("Waiting for instance to start running...")
	err = svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	})
	fmt.Println("Instance is running!")

	// Get Instance AZ(Availability Zone)
	describeSubnetOutput, err := svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: []*string{
			aws.String(*runInstanceOutput.Instances[0].SubnetId),
		},
	})
	instanceAZ := *describeSubnetOutput.Subnets[0].AvailabilityZone

	// Crete AMI from running instance
	createAmiOutput, err := svc.CreateImage(&ec2.CreateImageInput{
		Name:       aws.String("SimpleHTTPServer"),
		InstanceId: aws.String(instanceID),
	})
	imageID := *createAmiOutput.ImageId
	catchError(err)
	fmt.Println("Waiting for image to be availbale...")
	err = svc.WaitUntilImageAvailable(&ec2.DescribeImagesInput{
		ImageIds: []*string{
			aws.String(imageID),
		},
	})
	catchError(err)
	fmt.Println("Image is available!")

	// Terminate current EC2 Image
	terminateInstanceOutput, err := svc.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	})
	fmt.Printf("Terminating instance...\n Instance State: %v\n", terminateInstanceOutput.TerminatingInstances[0].CurrentState.String())

	// Create Launch Template with Image
	launchTemplateOutput, err := svc.CreateLaunchTemplate(&ec2.CreateLaunchTemplateInput{
		LaunchTemplateName: aws.String("SimpleHTTPServerTemplate"),
		LaunchTemplateData: &ec2.RequestLaunchTemplateData{
			ImageId: aws.String(imageID),
			SecurityGroupIds: []*string{
				securityGroupID,
			},
			KeyName:      aws.String(keyName),
			InstanceType: aws.String("t2.micro"),
		},
	})
	catchError(err)
	launchTemplateID := *launchTemplateOutput.LaunchTemplate.LaunchTemplateId

	// Create Autoscaling service
	autoScalingSvc := autoscaling.New(sess)
	_, err = autoScalingSvc.CreateAutoScalingGroup(&autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String("SimpleHTTPServerGroup"),
		AvailabilityZones: []*string{
			aws.String(instanceAZ),
		},
		LaunchTemplate: &autoscaling.LaunchTemplateSpecification{
			LaunchTemplateId: aws.String(launchTemplateID),
		},
		MinSize: aws.Int64(2),
		MaxSize: aws.Int64(3),
	})
	catchError(err)
	fmt.Println("Waiting for Group to be in service...")
	err = autoScalingSvc.WaitUntilGroupInService(&autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{
			aws.String("SimpleHTTPServerGroup"),
		},
	})
	catchError(err)
	fmt.Println("Autoscaling group is in service!")

	describeInstancesOutput, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{})
	catchError(err)
	for _, a := range describeInstancesOutput.Reservations {
		for _, instance := range a.Instances {
			fmt.Println(instance.State.String())
		}
	}
}

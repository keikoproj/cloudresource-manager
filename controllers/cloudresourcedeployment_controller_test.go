package controllers

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/golang/mock/gomock"
	cloudresourcev1alpha1 "github.com/keikoproj/cloudresource-manager/api/v1alpha1"
	"github.com/keikoproj/cloudresource-manager/pkg/awsapi"
	mock_awsapi "github.com/keikoproj/cloudresource-manager/pkg/awsapi/mocks"
	"gopkg.in/check.v1"
	"hash/adler32"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"testing"
	"time"
)

var c client.Client

var testStackName = "test-cfn1"
var testchangeSetName = "changeset-12345"
var testCapability = "test_capability"
var testTagKey = "test_tag_key"
var testTagValue = "test_tag_value"
var testParameterKey = "test_parameter_key"
var testParameterValue = "test_parameter_value"
var testExportKey = "testKey"
var testExportValue = "testValue"
var mockCFN = awsapi.CFN{}
var err error
var cfnResult *CfnValidationResult
var awscfnparams []*cloudformation.Parameter

type CloudformationAPISuite struct {
	t        *testing.T
	ctx      context.Context
	mockCtrl *gomock.Controller
	mockI    *mock_awsapi.MockCloudFormationAPI
	//mockCFN  awsapi.CFN
}

func TestCloudformationAPISuite(t *testing.T) {
	check.Suite(&CloudformationAPISuite{t: t})
	check.TestingT(t)
}

func (s *CloudformationAPISuite) SetUpTest(c *check.C) {
	s.ctx = context.Background()
	s.mockCtrl = gomock.NewController(s.t)
	s.mockI = mock_awsapi.NewMockCloudFormationAPI(s.mockCtrl)
	mockCFN = awsapi.CFN{
		Client: s.mockI,
	}
}

func (s *CloudformationAPISuite) TearDownTest(c *check.C) {
	s.mockCtrl.Finish()
}

//############

func TestMain(m *testing.M) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	if cfg, err = testEnv.Start(); err != nil {
		fmt.Println(err)
		fmt.Println(cfg)
	}
	fmt.Println(cfg)
	fmt.Println(err)

	os.Exit(m.Run())
}

func buildManager() (manager.Manager, error) {
	err := cloudresourcev1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	if cfg, err = testEnv.Start(); err != nil {
		fmt.Println(err)
		fmt.Println(cfg)
	}
	fmt.Println(cfg)
	fmt.Println(err)

	return manager.New(cfg, manager.Options{
		MetricsBindAddress: ":0",
		LeaderElection:     false,
	})
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameReviewInprogress(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789123-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()



	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("REVIEW_IN_PROGRESS"),
				StackStatusReason:           aws.String("User Initiated"),
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("106f7550-ea98-11ea-916b-0294f1fd1ce8"),
				LogicalResourceId:    aws.String("test-cfn1"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String(""),
				ResourceStatus:       aws.String("REVIEW_IN_PROGRESS"),
				ResourceStatusReason: aws.String("User Initiated"),
				ResourceType:         aws.String("AWS::CloudFormation::Stack"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}
	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("FAILED"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("CREATE_COMPLETE"),
			},
		},
	}

	inputExecuteChangeSet := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String(""),
		StackName:     aws.String(testStackName),
	}

	cfnoutputExecuteChangeSet := &cloudformation.ExecuteChangeSetOutput{}
	s.mockI.EXPECT().ExecuteChangeSet(inputExecuteChangeSet).Times(1).Return(cfnoutputExecuteChangeSet, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)

	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameReviewInprogressFailed(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("REVIEW_IN_PROGRESS"),
				StackStatusReason:           aws.String("User Initiated"),
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("106f7550-ea98-11ea-916b-0294f1fd1ce8"),
				LogicalResourceId:    aws.String("test-cfn1"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String(""),
				ResourceStatus:       aws.String("REVIEW_IN_PROGRESS"),
				ResourceStatusReason: aws.String("User Initiated"),
				ResourceType:         aws.String("AWS::CloudFormation::Stack"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}
	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("AVAILABLE"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("FAILED"),
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)

	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameInprogress(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_IN_PROGRESS"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_IN_PROGRESS"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)

	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.IsNil)
	t.Assert(result, check.Equals, true)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsComplete(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	input1 := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput11 := &cloudformation.GetTemplateOutput{
		TemplateBody: aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"),
	}
	s.mockI.EXPECT().GetTemplate(input1).Times(1).Return(cfnoutput11, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsCompleteUpdateInProgressChangesetCreatePending(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Update InProgress",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("AVAILABLE"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("CREATE_PENDING"),
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.IsNil)
	t.Assert(result, check.Equals, true)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsCompleteUpdateInProgressChangesetCreateInProgress(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Update InProgress",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("AVAILABLE"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("CREATE_IN_PROGRESS"),
			},
		},
	}
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.IsNil)
	t.Assert(result, check.Equals, true)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsRollBackComplete(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "InProgress",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("UPDATE_ROLLBACK_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("UPDATE_ROLLBACK_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	input1 := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput11 := &cloudformation.GetTemplateOutput{
		TemplateBody: aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"),
	}
	s.mockI.EXPECT().GetTemplate(input1).Times(1).Return(cfnoutput11, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	cfnResult.rollbackComplete = true
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsCompleteUpdateInProgress(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Update InProgress",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("AVAILABLE"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("CREATE_COMPLETE"),
			},
		},
	}
	inputExecuteChangeSet := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String("test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc"),
		StackName:     aws.String(testStackName),
	}

	cfnoutputExecuteChangeSet := &cloudformation.ExecuteChangeSetOutput{}
	s.mockI.EXPECT().ExecuteChangeSet(inputExecuteChangeSet).Times(1).Return(cfnoutputExecuteChangeSet, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsCompleteUpdateInProgressFailed(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Update InProgress",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("AVAILABLE"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("FAILED"),
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameIsFailed(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	//g.Expect(err).NotTo(gomega.HaveOccurred())
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc/6296246d-ef6a-4bd9-872a-a3a7b0afd8f7"),
				CreationTime:    aws.Time(time.Now().UTC()),
				Description:     aws.String("Dummy EC2"),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("ROLLBACK_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("ROLLBACK_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	input1 := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput11 := &cloudformation.GetTemplateOutput{
		TemplateBody: aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"),
	}
	s.mockI.EXPECT().GetTemplate(input1).Times(1).Return(cfnoutput11, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)

	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameParamsTags(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl":  "true",
					"CreatedBy": "CloudResource-Controller",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	input1 := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput11 := &cloudformation.GetTemplateOutput{
		TemplateBody: aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"),
	}
	s.mockI.EXPECT().GetTemplate(input1).Times(1).Return(cfnoutput11, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameChangeSetExecutionFailed(t *check.C) {
	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b",
			Checksum:      "cbea40b1",
			Message:       "Stack id: arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/eb0a0ee0-ead5-11ea-880b-066798daf74a",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Update InProgress",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("ROLLBACK_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("106f7550-ea98-11ea-916b-0294f1fd1ce8"),
				LogicalResourceId:    aws.String("test-cfn1"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String(""),
				ResourceStatus:       aws.String("ROLLBACK_COMPLETE"),
				ResourceStatusReason: aws.String("User Initiated"),
				ResourceType:         aws.String("AWS::CloudFormation::Stack"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}
	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("FAILED"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("CREATE_COMPLETE"),
			},
		},
	}

	inputExecuteChangeSet := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
		StackName:     aws.String("test-cfn1"),
	}
	cfnoutputExecuteChangeSet := &cloudformation.ExecuteChangeSetOutput{}
	s.mockI.EXPECT().ExecuteChangeSet(inputExecuteChangeSet).Times(1).Return(cfnoutputExecuteChangeSet, nil)

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameChangeSetExecutionError(t *check.C) {
	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("REVIEW_IN_PROGRESS"),
				StackStatusReason:           aws.String("User Initiated"),
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("106f7550-ea98-11ea-916b-0294f1fd1ce8"),
				LogicalResourceId:    aws.String("test-cfn1"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String(""),
				ResourceStatus:       aws.String("REVIEW_IN_PROGRESS"),
				ResourceStatusReason: aws.String("User Initiated"),
				ResourceType:         aws.String("AWS::CloudFormation::Stack"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}
	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput3 := &cloudformation.ListChangeSetsOutput{
		Summaries: []*cloudformation.ChangeSetSummary{
			{
				ChangeSetId:     aws.String("arn:aws:cloudformation:us-west-2:123456789123:changeSet/test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b/96ee5b0f-48e9-4f79-9276-f7af8c629474"),
				ChangeSetName:   aws.String("test-cfn1-changeset-636528ff-ba99-48c3-a9b0-ef66a569bc3b"),
				CreationTime:    aws.Time(time.Now().UTC()),
				ExecutionStatus: aws.String("FAILED"),
				StackId:         aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:       aws.String(testStackName),
				Status:          aws.String("CREATE_COMPLETE"),
			},
		},
	}

	inputExecuteChangeSet := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String(""),
		StackName:     aws.String(testStackName),
	}

	cfnoutputExecuteChangeSet := &cloudformation.ExecuteChangeSetOutput{}
	execChangeSetError := awserr.New("400", "cannot be executed in its current status of CREATE_IN_PROGRESS", nil)
	s.mockI.EXPECT().ExecuteChangeSet(inputExecuteChangeSet).Times(1).Return(cfnoutputExecuteChangeSet, execChangeSetError)

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput3, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameDiffTemplate(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl":  "true",
					"CreatedBy": "CloudResource-Controller",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("t2.nano"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("cfn-ctrl"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	input1 := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput11 := &cloudformation.GetTemplateOutput{
		TemplateBody: aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType"),
	}
	s.mockI.EXPECT().GetTemplate(input1).Times(1).Return(cfnoutput11, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackExistsWithSameNameDiffParams(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl":  "true",
					"CreatedBy": "CloudResource-Controller",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn1"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("m5.xlarge"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("AWS"),
						Value: aws.String("True"),
					},
					{
						Key:   aws.String("CFN"),
						Value: aws.String("True"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput1 := &cloudformation.DescribeStackEventsOutput{
		StackEvents: []*cloudformation.StackEvent{
			{
				ClientRequestToken:   aws.String(""),
				EventId:              aws.String("LinuxEC2Instance-CREATE_IN_PROGRESS-2020-08-30T15:31:57.943Z"),
				LogicalResourceId:    aws.String("LinuxEC2Instance"),
				PhysicalResourceId:   aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				ResourceProperties:   aws.String("{'ImageId': 'ami-0002624afe2bc9324', 'InstanceType': 't2.nano'}"),
				ResourceStatus:       aws.String("CREATE_COMPLETE"),
				ResourceStatusReason: aws.String(""),
				ResourceType:         aws.String("AWS::EC2::Instance"),
				StackId:              aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:            aws.String(testStackName),
				Timestamp:            aws.Time(time.Now().UTC()),
			},
		},
	}

	input1 := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput11 := &cloudformation.GetTemplateOutput{
		TemplateBody: aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"),
	}

	s.mockI.EXPECT().GetTemplate(input1).Times(1).Return(cfnoutput11, nil)
	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput1, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestValidateIfStackDoesnotExistsWithSameName(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl":  "true",
					"CreatedBy": "CloudResource-Controller",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	ctx := context.TODO()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String("test-cfn1"),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Capabilities:    []*string{aws.String("CAPABILITY_NAMED_IAM")},
				CreationTime:    aws.Time(time.Now().UTC()),
				DisableRollback: aws.Bool(false),
				DriftInformation: &cloudformation.StackDriftInformation{
					StackDriftStatus: aws.String("NOT_CHECKED"),
				},
				EnableTerminationProtection: aws.Bool(false),
				RoleARN:                     aws.String("arn:aws:iam::123456789123:role/clf-service-role"),
				RollbackConfiguration:       &cloudformation.RollbackConfiguration{},
				StackId:                     aws.String("arn:aws:cloudformation:us-west-2:123456789123:stack/test-cfn1/10701190-ea98-11ea-916b-0294f1fd1ce8"),
				StackName:                   aws.String("test-cfn2"),
				StackStatus:                 aws.String("CREATE_COMPLETE"),
				StackStatusReason:           aws.String("User Initiated"),
				LastUpdatedTime:             aws.Time(time.Now().UTC()),
				Parameters: []*cloudformation.Parameter{
					{
						ParameterKey:   aws.String("AMIid"),
						ParameterValue: aws.String("ami-0002624afe2bc9324"),
					},
					{
						ParameterKey:   aws.String("InstanceType"),
						ParameterValue: aws.String("m5.xlarge"),
					},
				},
				Tags: []*cloudformation.Tag{
					{
						Key:   aws.String("AWS"),
						Value: aws.String("True"),
					},
					{
						Key:   aws.String("CFN"),
						Value: aws.String("True"),
					},
					{
						Key:   aws.String("CreatedBy"),
						Value: aws.String("CloudResource-Controller"),
					},
				},
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	cfnResult, awscfnparams = clr.validateIfStackExistsWithSameName(ctx, created, mockCFN)
	result, err := clr.CheckForRequeue(context.TODO(), created, cfnResult, awscfnparams, mockCFN)
	t.Assert(err, check.NotNil)
	t.Assert(result, check.Equals, false)
}

//==============================================
//	controller tests
//==============================================

func (s *CloudformationAPISuite) TestValidateORUpdateCheckSum(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "cbea40b1",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result := clr.ValidateORUpdateCheckSum(created)
	t.Assert(result, check.Equals, "Updated")
}

func (s *CloudformationAPISuite) TestValidateORUpdateCheckSumNoUpdate(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:  "123456789123",
					ExternalID: "cloudresource-123456789-ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "a8a40f4d",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	fmt.Printf("checksum: %x\n", adler32.Checksum([]byte(fmt.Sprintf("%+v", created.Spec.Cloudformation))))

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result := clr.ValidateORUpdateCheckSum(created)
	t.Assert(result, check.Equals, "NotUpdated")
}

func (s *CloudformationAPISuite) TestSetAssumeRoleProviderUc1(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					AccountID:   "123456789123",
					ExternalID:  "cloudresource-123456789-ppd",
					Environment: "ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "5e581838",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()

	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result, err := clr.SetAssumeRoleProvider(context.TODO(), created)
	t.Assert(result, check.Equals, true)
}

func (s *CloudformationAPISuite) TestSetAssumeRoleProviderUc2(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					RoleARN:     "arn:aws:iam::123456789123:role/cfn-master-role",
					Environment: "ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "5e581838",
			Message:       "Stack is in Failed status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result, err := clr.SetAssumeRoleProvider(context.TODO(), created)
	t.Assert(result, check.Equals, true)
}

func (s *CloudformationAPISuite) TestSetAssumeRoleProviderUc3(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					RoleARN:     "",
					Environment: "ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "5e581838",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result, err := clr.SetAssumeRoleProvider(context.TODO(), created)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestSetAssumeRoleProviderUc4(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					RoleARN:     "arn:aws:iam:::role/cfn-master-role",
					Environment: "ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "5e581838",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result, err := clr.SetAssumeRoleProvider(context.TODO(), created)
	t.Assert(result, check.Equals, false)
}

func (s *CloudformationAPISuite) TestSetAssumeRoleProviderUc5(t *check.C) {

	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	created := &cloudresourcev1alpha1.CloudResourceDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: cloudresourcev1alpha1.CloudResourceDeploymentSpec{
			Cloudformation: &cloudresourcev1alpha1.StackSpec{
				Parameters: map[string]string{
					"InstanceType": "t2.nano",
					"AMIid":        "ami-0002624afe2bc9324",
				},
				Tags: map[string]string{
					"cfn-ctrl": "true",
				},
				Template:  template,
				Stackname: "test-cfn1",
				Region:    "us-west-2",
				CARole: cloudresourcev1alpha1.AssumeRoleProvider{
					RoleARN:     " ",
					Environment: "ppd",
				},
				Timeout: 50,
			},
		},
		Status: cloudresourcev1alpha1.CloudResourceDeploymentStatus{
			Changesetname: "test-cfn1-changeset-c559dc34-9b15-4ac8-9cc1-2730f922c5cc",
			Checksum:      "5e581838",
			Message:       "Stack is in Success status for resource=gitops-sample-1 with state=ROLLBACK_COMPLETE",
			StartedAt:     &metav1.Time{Time: time.Now()},
			Status:        "Success",
		},
	}

	mgr, err := buildManager()
	t.Assert(err, check.IsNil)
	c = mgr.GetClient()
	clr := &CloudResourceDeploymentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("unit-test").WithName("CloudResourceDeployment"),
		//CfClient:    &s.mockCFN,
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CloudResourceDeployment"),
		MaxParallel: 10,
	}

	result, err := clr.SetAssumeRoleProvider(context.TODO(), created)
	t.Assert(result, check.Equals, false)
}

package awsapi_test

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/awserr"
	mock_awsapi "github.com/keikoproj/cloudresource-manager/pkg/awsapi/mocks"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/golang/mock/gomock"
	awsapi "github.com/keikoproj/cloudresource-manager/pkg/awsapi"

	"gopkg.in/check.v1"
)

var testStackName = "test1234"
var testchangeSetName = "changeset-12345"
var testCapability = "test_capability"
var testTagKey = "test_tag_key"
var testTagValue = "test_tag_value"
var testParameterKey = "test_parameter_key"
var testParameterValue = "test_parameter_value"
var testExportKey = "testKey"
var testExportValue = "testValue"
var mockCFN = awsapi.CFN{}

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

func (s *CloudformationAPISuite) TestCreateChangeSet(c *check.C) {
	template := "---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"
	capabilities := []*string{
		aws.String("CAPABILITY_NAMED_IAM"),
	}
	svcRole := "arn:aws:iam::1234567890:role/clf-service-role"

	input := &cloudformation.CreateChangeSetInput{
		ChangeSetName: aws.String(testchangeSetName),
		ChangeSetType: aws.String("CREATE"),
		StackName:     aws.String(testStackName),
		TemplateBody:  aws.String("---\n AWSTemplateFormatVersion: '2010-09-09'\n Description: Dummy EC2\n Parameters:\n   InstanceType:\n     Description: EC2 instance sizes.\n     Type: String\n     ConstraintDescription: must be a valid EC2 instance type that supports the chosen\n       AMI.\n   AMIid:\n     Type: String\n     Description: 'Use the latest HVM AMI ID from the AMI Catalog: amiCatalog'\n Resources:\n   LinuxEC2Instance:\n     Type: AWS::EC2::Instance\n     Properties:\n       ImageId:\n         Ref: AMIid\n       InstanceType:\n         Ref: InstanceType\n"),
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
		RoleARN:      aws.String(svcRole),
		Capabilities: capabilities,
	}
	var params map[string]string
	params = map[string]string{"AMIid": "ami-0002624afe2bc9324", "InstanceType": "m5.xlarge"}
	tags := map[string]string{"CFN": "True", "AWS": "True"}

	cfnoutput := &cloudformation.CreateChangeSetOutput{}
	s.mockI.EXPECT().CreateChangeSet(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.CreateChangeSet(s.ctx, testchangeSetName, testStackName, template, params, tags, svcRole, "CREATE")
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestExecuteChangeSet(c *check.C) {
	input := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String(testchangeSetName),
		StackName:     aws.String(testStackName),
	}

	cfnoutput := &cloudformation.ExecuteChangeSetOutput{}
	s.mockI.EXPECT().ExecuteChangeSet(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.ExecuteChangeSet(s.ctx, testStackName, testchangeSetName)
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestGetTemplate(c *check.C) {
	input := &cloudformation.GetTemplateInput{
		StackName: aws.String(testStackName),
	}

	cfnoutput := &cloudformation.GetTemplateOutput{}
	s.mockI.EXPECT().GetTemplate(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.GetTemplate(s.ctx, testStackName, "")
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestGetTemplateWithChangeSet(c *check.C) {
	input := &cloudformation.GetTemplateInput{
		StackName:     aws.String(testStackName),
		ChangeSetName: aws.String(testchangeSetName),
	}

	cfnoutput := &cloudformation.GetTemplateOutput{}
	s.mockI.EXPECT().GetTemplate(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.GetTemplate(s.ctx, testStackName, testchangeSetName)
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestGetTemplateFailed(c *check.C) {
	input := &cloudformation.GetTemplateInput{
		StackName:     aws.String(testStackName),
		ChangeSetName: aws.String(testchangeSetName),
	}

	cfnoutput := &cloudformation.GetTemplateOutput{}
	awsError := awserr.New("400", "Stack with same name doesn't exist", nil)
	s.mockI.EXPECT().GetTemplate(input).Times(1).Return(cfnoutput, awsError)
	_, err := mockCFN.GetTemplate(s.ctx, testStackName, testchangeSetName)
	c.Assert(err, check.DeepEquals, awsError)
}

func (s *CloudformationAPISuite) TestDeleteStack(c *check.C) {
	input := &cloudformation.DeleteStackInput{
		StackName: aws.String(testStackName),
	}
	cfnoutput := &cloudformation.DeleteStackOutput{}

	s.mockI.EXPECT().DeleteStack(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.DeleteStack(s.ctx, testStackName)
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestDescribeCloudformationStacks(c *check.C) {

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(testStackName),
	}
	cfnoutput := &cloudformation.DescribeStacksOutput{}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.DescribeCloudformationStacks(s.ctx, testStackName)
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestDescribeChangeset(c *check.C) {
	input := &cloudformation.DescribeChangeSetInput{
		StackName:     aws.String(testStackName),
		ChangeSetName: aws.String(testchangeSetName),
	}

	cfnoutput := &cloudformation.DescribeChangeSetOutput{}

	s.mockI.EXPECT().DescribeChangeSet(input).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.DescribeChangeSet(s.ctx, testStackName, testchangeSetName)
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestFailedDescribeChangeset(c *check.C) {
	input := &cloudformation.DescribeChangeSetInput{
		StackName:     aws.String(testStackName),
		ChangeSetName: aws.String(testchangeSetName),
	}

	cfnoutput := &cloudformation.DescribeChangeSetOutput{}

	describeChangeSetFailedError := awserr.New("400", "Changeset name doesn't exist", nil)
	s.mockI.EXPECT().DescribeChangeSet(input).Times(1).Return(cfnoutput, describeChangeSetFailedError)
	_, err := mockCFN.DescribeChangeSet(s.ctx, testStackName, testchangeSetName)
	c.Assert(err, check.DeepEquals, describeChangeSetFailedError)
}

func (s *CloudformationAPISuite) TestGetResourceFromStackOutput(c *check.C) {
	referenceStackName := "StackVpcIdExport"
	referenceResourceName := "VpcId"

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(referenceStackName),
	}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Outputs: []*cloudformation.Output{
					{
						Description: aws.String("Security Group ID"),
						ExportName:  aws.String("StackVpcIdExport-VpcId"),
						OutputKey:   aws.String(referenceResourceName),
						OutputValue: aws.String("vpc-034a8b31fbda16940"),
					},
				},
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)

	referenceResourceValue, err := mockCFN.GetResourceFromStackOutput(s.ctx, referenceStackName, referenceResourceName)
	c.Assert(err, check.IsNil)
	c.Assert(aws.String(referenceResourceValue), check.DeepEquals, cfnoutput.Stacks[0].Outputs[0].OutputValue)
}

func (s *CloudformationAPISuite) TestFetchReferenceResources(c *check.C) {

	referenceStackName := "StackVpcIdExport"
	referenceResourceName := "VpcId"

	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(referenceStackName),
	}

	params := map[string]string{"VpcId": "!stack_output StackVpcIdExport::VpcId"}

	cfnoutput := &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				Outputs: []*cloudformation.Output{
					{
						Description: aws.String("Security Group ID"),
						ExportName:  aws.String("StackVpcIdExport-VpcId"),
						OutputKey:   aws.String(referenceResourceName),
						OutputValue: aws.String("vpc-034a8b31fbda16940"),
					},
				},
			},
		},
	}

	s.mockI.EXPECT().DescribeStacks(input).Times(1).Return(cfnoutput, nil)
	referenceResources, err := mockCFN.FetchReferenceResources(s.ctx, params)

	c.Assert(err, check.IsNil)
	c.Assert(referenceResources[0].ParameterValue, check.DeepEquals, cfnoutput.Stacks[0].Outputs[0].OutputValue)
}

func (s *CloudformationAPISuite) TestCompareStackParams(c *check.C) {
	cfnParameters1 := []*cloudformation.Parameter{
		{
			ParameterKey:   aws.String("VpcId"),
			ParameterValue: aws.String("vpc-0c4c134d99457655e"),
		},
	}
	cfnParameters2 := []*cloudformation.Parameter{
		{
			ParameterKey:   aws.String("VpcId"),
			ParameterValue: aws.String("vpc-0c4c134d99457655f"),
		},
	}
	comparison1 := mockCFN.CompareStackParams(s.ctx, cfnParameters1, cfnParameters1)
	comparison2 := mockCFN.CompareStackParams(s.ctx, cfnParameters1, cfnParameters2)

	c.Assert(comparison1, check.DeepEquals, true)
	c.Assert(comparison2, check.DeepEquals, false)

}

func (s *CloudformationAPISuite) TestWrapper(c *check.C) {
	// Testing List Change Set Set
	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &testStackName,
	}
	cfnoutput := &cloudformation.ListChangeSetsOutput{}
	s.mockI.EXPECT().ListChangeSets(changeSetListInput).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.WrapperFunc(context.TODO(), testStackName)
	c.Assert(err, check.IsNil)
}

func (s *CloudformationAPISuite) TestPostValidation(c *check.C) {
	// Testing List Change Set Set
	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: &testStackName,
	}
	cfnoutput := &cloudformation.DescribeStackEventsOutput{}
	s.mockI.EXPECT().DescribeStackEvents(paramsListStackEvents).Times(1).Return(cfnoutput, nil)
	_, err := mockCFN.PostValidationFunc(context.TODO(), testStackName)
	c.Assert(err, check.IsNil)
}

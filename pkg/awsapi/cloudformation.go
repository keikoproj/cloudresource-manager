package awsapi

//go:generate mockgen -destination=./mocks/mock_clouformationiface.go -package=mock_awsapi github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface CloudFormationAPI

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	cloudresourcev1alpha1 "github.com/keikoproj/cloudresource-manager/api/v1alpha1"
	"os"
	"sort"
	"strings"

	//"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface"
	ctrl "sigs.k8s.io/controller-runtime"
)

type CFN struct {
	Client cloudformationiface.CloudFormationAPI
}

func GetRegion() (string, error) {

	if os.Getenv("AWS_REGION") != "" {
		return os.Getenv("AWS_REGION"), nil
	}
	// Try Derive
	var config aws.Config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            config,
	}))
	f := ec2metadata.New(sess)
	region, err := f.Region()
	if err != nil {
		return "", err
	}
	return region, nil
}

func GetCloudformationClient(region string) *CFN {

	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		panic(err)
	}
	return &CFN{
		Client: cloudformation.New(sess, &aws.Config{
			Region: aws.String(region),
		}),
	}
}

func (c *CFN) GetTargetAccountSession(context context.Context, cloudResource *cloudresourcev1alpha1.CloudResourceDeployment) (*CFN, error) {
	roleARN := cloudResource.Spec.Cloudformation.CARole.RoleARN
	rsn := cloudResource.Spec.Cloudformation.CARole.RoleSessionName
	eid := cloudResource.Spec.Cloudformation.CARole.ExternalID
	region := cloudResource.Spec.Cloudformation.Region

	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		panic(err)
	}
	if roleARN != "" {
		log.Info("run assume")
		creds := stscreds.NewCredentials(sess, roleARN, func(o *stscreds.AssumeRoleProvider) {
			if rsn != "" {
				o.RoleSessionName = rsn
			}
			if eid != "" {
				o.ExternalID = &eid
			}
		})
		log.Info("calling creds.Get()")
		_, err := creds.Get()
		if err != nil {
			log.Error(err, "Unable to get credentials with given assumeroleprovider")
			message := fmt.Sprintf("Credentials are not valid")
			log.Info(message)
			err = errors.New(message)
			return nil, err
		}
		//log.Info("credentials value obtained", "value:", value)
		return &CFN{
			Client: cloudformation.New(sess, &aws.Config{
				Credentials: creds,
				Region:      aws.String(region),
			}),
		}, nil
	} else {
		message := fmt.Sprintf("RoleArn not provided")
		log.Info(message)
		err = errors.New(message)
		return &CFN{
			Client: cloudformation.New(sess, &aws.Config{
				Region: aws.String(region),
			}),
		}, err
	}
}

func (c *CFN) CreateChangeSet(context context.Context, changeSetName string, Stackname string, Template string, params map[string]string, tags map[string]string, svcRole string, changeSetType string) (cloudformation.CreateChangeSetOutput, error) {
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))
	paramsinput, err := c.StackParameters(context, params)
	if err != nil {
		log.Error(err, "Error while creating Params")
		return cloudformation.CreateChangeSetOutput{}, err
	}
	tagsinput, err := c.StackTags(context, tags)
	if err != nil {
		log.Error(err, "Error while creating Tags")
		return cloudformation.CreateChangeSetOutput{}, err
	}
	capabilities := []*string{
		aws.String("CAPABILITY_NAMED_IAM"),
	}
	log.Info("svcRole is passed", "svcRole:", svcRole, "length:", len(svcRole))
	var input *cloudformation.CreateChangeSetInput

	if len(svcRole) == 0 {
		log.Error(err, "Error while creating changeset, svcRole is not defined")
		return cloudformation.CreateChangeSetOutput{}, err
	}

	input = &cloudformation.CreateChangeSetInput{
		ChangeSetName: aws.String(changeSetName),
		ChangeSetType: aws.String(changeSetType),
		StackName:     aws.String(Stackname),
		TemplateBody:  aws.String(Template),
		Parameters:    paramsinput,
		Tags:          tagsinput,
		RoleARN:       aws.String(svcRole),
		Capabilities:  capabilities,
	}

	if err := input.Validate(); err != nil {
		log.Error(err, "input validation failed")
		return cloudformation.CreateChangeSetOutput{}, err
	}

	d, err := c.Client.CreateChangeSet(input)
	if err != nil {
		log.Error(err, "Error while creating changeset")
		return cloudformation.CreateChangeSetOutput{}, err
	}

	log.Info("Change-set created successfully")
	return *d, nil
}

func (c *CFN) StackParameters(context context.Context, paraminputs map[string]string) ([]*cloudformation.Parameter, error) {

	var params []*cloudformation.Parameter

	ParameterKey := make([]string, 0, len(paraminputs))
	for key := range paraminputs {
		ParameterKey = append(ParameterKey, key)
	}
	sort.Strings(ParameterKey)

	for _, k := range ParameterKey {
		params = append(params, &cloudformation.Parameter{
			ParameterKey:   aws.String(k),
			ParameterValue: aws.String(paraminputs[k]),
		})
	}
	return params, nil
}

func (c *CFN) GetResourceFromStackOutput(context context.Context, stackName string, outputResourceName string) (string, error) {
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))

	stackDetails, err := c.DescribeCloudformationStacks(context, stackName)
	if err != nil {
		log.Error(err, "Error while looking up reference stack")
		return "", err
	}
	if stackDetails.Stacks != nil {
		stackOutput := stackDetails.Stacks[0].Outputs

		for _, key := range stackOutput {
			if *key.OutputKey == outputResourceName {
				return *key.OutputValue, nil
			}

		}
	}
	log.Error(err, "Could not find the given reference resource on the reference stack")
	return "", err
}

func (c *CFN) FetchReferenceResources(context context.Context, referenceResourceInputs map[string]string) ([]*cloudformation.Parameter, error) {
	var referenceResources []*cloudformation.Parameter
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))

	for key, value := range referenceResourceInputs {
		referenceResource := false
		var referenceResourceDetails []string

		if strings.Contains(value, "::") {
			if strings.HasPrefix(value, "!stack_output ") {
				referenceResource = true
				referenceResourceDetails = strings.Split(strings.Split(value, "!stack_output ")[1], "::")
			} else if strings.HasPrefix(value, "!stack_output_external ") {
				referenceResource = true
				referenceResourceDetails = strings.Split(strings.Split(value, "!stack_output_external ")[1], "::")
			}
		}

		if referenceResource {
			stackOutputValue, err := c.GetResourceFromStackOutput(context, referenceResourceDetails[0], referenceResourceDetails[1])
			if err != nil || stackOutputValue == "" {
				log.Error(err, "Error while fetching the reference resource value from reference stack")
				return []*cloudformation.Parameter{}, err
			}
			referenceResources = append(referenceResources, &cloudformation.Parameter{
				ParameterKey:   aws.String(key),
				ParameterValue: aws.String(stackOutputValue),
			})
		}
	}

	return referenceResources, nil
}

func (c *CFN) StackTags(context context.Context, Tagsinput map[string]string) ([]*cloudformation.Tag, error) {

	var tg []*cloudformation.Tag

	TagKey := make([]string, 0, len(Tagsinput))
	for key := range Tagsinput {
		TagKey = append(TagKey, key)
	}
	sort.Strings(TagKey)

	for _, k := range TagKey {
		tg = append(tg, &cloudformation.Tag{
			Key:   aws.String(k),
			Value: aws.String(Tagsinput[k]),
		})
	}

	tg = append(tg, &cloudformation.Tag{
		Key:   aws.String("CreatedBy"),
		Value: aws.String("CloudResource-Controller"),
	})

	return tg, nil
}

func (c *CFN) ExecuteChangeSet(context context.Context, Stackname string, changeSetName string) (cloudformation.ExecuteChangeSetOutput, error) {
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))

	input := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String(changeSetName),
		StackName:     aws.String(Stackname),
	}

	d, err := c.Client.ExecuteChangeSet(input)
	if err != nil {
		log.Error(err, "Error while executing change set")
		return cloudformation.ExecuteChangeSetOutput{}, err
	}

	log.Info("Executed change set successfully")
	return *d, nil
}

func (c *CFN) GetTemplate(context context.Context, Stackname string, ChangesetName string) (cloudformation.GetTemplateOutput, error) {
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))

	input := &cloudformation.GetTemplateInput{}
	if ChangesetName != "" {
		input = &cloudformation.GetTemplateInput{
			StackName:     aws.String(Stackname),
			ChangeSetName: aws.String(ChangesetName),
		}

		log.Info("Template body of CR is compared with changeset body")
	} else {
		input = &cloudformation.GetTemplateInput{
			StackName: aws.String(Stackname),
		}
	}

	d, err := c.Client.GetTemplate(input)
	if err != nil {
		log.Error(err, "Error while Getting Template")
		return cloudformation.GetTemplateOutput{}, err
	}

	log.Info("Got Template successfully")
	return *d, nil
}

func (c *CFN) DeleteStack(context context.Context, Stackname string) (cloudformation.DeleteStackOutput, error) {

	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", context.Value("request_id"))

	input := &cloudformation.DeleteStackInput{
		StackName: aws.String(Stackname),
	}

	d, err := c.Client.DeleteStack(input)
	if err != nil {
		log.Error(err, "Error while Getting Template")
		return cloudformation.DeleteStackOutput{}, err
	}

	log.Info("Got Template successfully")
	return *d, nil

}

func (c *CFN) DescribeChangeSet(context context.Context, StackName string, ChangesetName string) (cloudformation.DescribeChangeSetOutput, error) {
	log := ctrl.Log.Logger
	log.Info("DescribeChangeSet with input parameters", "ctx", context, "ChangetsetName", ChangesetName)

	input := &cloudformation.DescribeChangeSetInput{
		StackName:     aws.String(StackName),
		ChangeSetName: aws.String(ChangesetName),
	}

	d, err := c.Client.DescribeChangeSet(input)
	if err != nil {
		log.Error(err, "Error while fetching changeset info")
		return cloudformation.DescribeChangeSetOutput{}, err
	}

	log.Info("Successfully fetched changeset information")
	return *d, nil
}

func (c *CFN) DescribeCloudformationStacks(context context.Context, Stackname string) (cloudformation.DescribeStacksOutput, error) {
	log := ctrl.Log.Logger
	log.Info("describeCloudformationStacks with input parameters", "ctx", context, "Stackname", Stackname)

	input := &cloudformation.DescribeStacksInput{StackName: aws.String(Stackname)}
	err := input.Validate()
	if err != nil {
		log.Error(err, "DescribeStacksInput is invalid")
		return cloudformation.DescribeStacksOutput{}, err
	}
	out, err := c.Client.DescribeStacks(input)
	if err != nil {

		if awserrmessage, ok := err.(awserr.Error); ok {
			if awserrmessage.Code() == "ValidationError" {
				log.Info("AWS ValidationError", "message:", awserrmessage.Message())
				return cloudformation.DescribeStacksOutput{}, err
			}
		}
		log.Error(err, "Error calling DescribeStacks")
		return cloudformation.DescribeStacksOutput{}, err
	}

	return *out, nil
}

func (c *CFN) WrapperFunc(ctx context.Context, StackName string) (cloudformation.ChangeSetSummary, error) {
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", ctx.Value("request_id"))
	if StackName == "" {
		return cloudformation.ChangeSetSummary{}, errors.New("No Stack name found")
	}
	changeSetListInput := &cloudformation.ListChangeSetsInput{
		StackName: &StackName,
	}
	response, err := c.Client.ListChangeSets(changeSetListInput)
	if err != nil {
		log.Error(err, "Error while listing changeset")
		result := cloudformation.ChangeSetSummary{}
		return result, err
	}
	response.Summaries = append(response.Summaries, &cloudformation.ChangeSetSummary{})
	if len(response.Summaries) > 0 {
		return *response.Summaries[0], nil
	} else {
		err := errors.New("no change set summary returned for given stack")
		return cloudformation.ChangeSetSummary{}, err
	}
}

func (c *CFN) PostValidationFunc(ctx context.Context, StackName string) (cloudformation.DescribeStackEventsOutput, error) {
	if StackName == "" {
		return cloudformation.DescribeStackEventsOutput{}, errors.New("No Stack name found")
	}
	paramsListStackEvents := &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(StackName),
	}
	response, err := c.Client.DescribeStackEvents(paramsListStackEvents)
	response.StackEvents = append(response.StackEvents, &cloudformation.StackEvent{})
	if err != nil {
		return cloudformation.DescribeStackEventsOutput{}, err
	}
	return *response, nil
}

func (c *CFN) CompareStackParams(ctx context.Context, awscfnparams, crparams []*cloudformation.Parameter) bool {
	log := ctrl.Log.Logger
	log = log.WithValues("uuid:", ctx.Value("request_id"))
	for _, v := range awscfnparams {
		log.Info("parameters to Compare", "AWSParameter:", v)
		for _, s := range crparams {
			if *v.ParameterKey == *s.ParameterKey {
				log.Info("ParameterKey matched", "AWSParameterKey:", *v.ParameterKey, "CRParameterKey:", *s.ParameterKey)
				if *v.ParameterValue != *s.ParameterValue {
					log.Info("ParameterValue did not match", "ParameterKey:", *v.ParameterKey, "AWSParameterValue:", *v.ParameterValue, "CRParameterValue:", *s.ParameterValue)
					return false
				}
				log.Info("ParameterValue matched", "AWSParameterValue:", *v.ParameterValue, "CRParameterValue:", *s.ParameterValue)
				break
			} else {
				log.Info("ParameterKey did not match iterating over next Parameter", "AWSParameterkey:", *v.ParameterKey, "CRParameterkey:", *s.ParameterKey)
			}
		}
	}
	return true
}

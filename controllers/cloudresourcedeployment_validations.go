package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/tidwall/gjson"
	"github.com/keikoproj/cloudresource-manager/pkg/awsapi"
	v1 "k8s.io/api/core/v1"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudformation"

	cloudresourcev1alpha1 "github.com/keikoproj/cloudresource-manager/api/v1alpha1"
)

type CfnValidationResult struct {
	//has the AND operation of all the validations result
	result           bool
	exists           bool
	requeue          bool
	exech            bool
	rollbackComplete bool
	status           string
	//If all validations are success, stackId is updated by validators
	stackId string

	//if result is false, error and message will be populated
	//This result can carry the state of other validators also
	errors   []error
	messages []string

	// Changeset name
	latestChangeSetName string
}

type TemplateValidator interface {
	newCfnValidationResult() *CfnValidationResult
	validateIfStackExistsWithSameName(ctx context.Context, cfn *cloudresourcev1alpha1.CloudResourceDeployment) *CfnValidationResult
	validateTemplateStructure(ctx context.Context, cfn *cloudresourcev1alpha1.CloudResourceDeployment, result *CfnValidationResult)
	validateTemplateDefinition(context context.Context, cfn *cloudresourcev1alpha1.CloudResourceDeployment, result *CfnValidationResult) *CfnValidationResult
}

//Validates if a there is a stack already with the same name for a given CR. If there is no stack, then we can create one.
// If CR status is any of the terminal states, it goes one step ahead and checks the state of the stack. If it's anything but REVIEW_IN_PROGRESS, it proceeds with other validations.
// For Example, During the CR submission, change set was created but timed out while polling. Next reconcilation, it will be executed as it must have reached REVIEW_IN_PROGRESS state by now.
func (w *CloudResourceDeploymentReconciler) validateIfStackExistsWithSameName(context context.Context, cloudResource *cloudresourcev1alpha1.CloudResourceDeployment, CfClient awsapi.CFN) (*CfnValidationResult, []*cloudformation.Parameter) {
	log := w.Log

	cfnResult := w.newCfnValidationResult()

	describeStackOutput, err := CfClient.DescribeCloudformationStacks(context, cloudResource.Spec.Cloudformation.Stackname)
	if err != nil {
		if awserr, ok := err.(awserr.Error); ok {
			if awserr.Code() == AWS_VALIDATION_ERROR {
				log.Info("No stack exist for this particular resource, so we're proceeding with other validations")
				return cfnResult, nil
			}
		}
		message := "Error while retrieving stack information using cloudformation client for resource"
		log.Error(err, message, "resource", cloudResource.ObjectMeta.Name)
		w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", message)
		cfnResult.messages = append(cfnResult.messages, message)
		cfnResult.errors = append(cfnResult.errors, err)
		cfnResult.result = false
		cfnResult.exists = false
		return cfnResult, nil
	}
	log.Info("DescribeStackOutput:", "stackoutput:", describeStackOutput)

	stacks := describeStackOutput.Stacks
	awscfnparams := describeStackOutput.Stacks[0].Parameters

	//if there is stack with same name in Cloudformation then return false
	// if the status is REVIEW_IN_PROGRESS,we can go ahead
	if *stacks[0].StackName == cloudResource.Spec.Cloudformation.Stackname {
		cfnResult.exists = true
		EventResponse, err := CfClient.PostValidationFunc(context, cloudResource.Spec.Cloudformation.Stackname)
		if err != nil {
			log.Info("Failed StackEvent", "Event:", EventResponse)
			log.Error(err, "Unable to describe the stack events ", "StackName:", cloudResource.Spec.Cloudformation.Stackname)
			log.Info("Exiting post validation as error occurred while describing the stack", "StackName:", cloudResource.Spec.Cloudformation.Stackname)
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Error getting Stack Events")

			if awserr, ok := err.(awserr.Error); ok {
				if awserr.Code() == AWS_VALIDATION_ERROR {
					log.Info(awserr.Message())
				}
			}
		}
		resJson, err := json.Marshal(EventResponse.StackEvents[0])
		if err != nil {
			log.Error(err, "Unable to convert Event response to JSON , so skip Info logging ", "StackName", cloudResource.Spec.Cloudformation.Stackname)
		} else {
			log.Info("Event Response from the Session is successful", "stackEvents", string(resJson))
		}
		Response := string(resJson)
		log.Info("Latest StackEvent", "Event:", Response)
		w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Status", Response)
		pvstatus := gjson.Get(Response, "ResourceStatus").String()
		log.Info("Status from PostValidation", "is:", pvstatus)

		if IsComplete(*stacks[0].StackStatus) {
			cfnResult.stackId = gjson.Get(Response, "PhysicalResourceId").String()
			cfnResult.status = "Complete"
			message := fmt.Sprintf("Stack Execution Completed successfully for resource=%s", cloudResource.ObjectMeta.Name)
			log.Info(message)
			cfnResult.messages = append(cfnResult.messages, message)
			if cloudResource.Status.Status == "Update InProgress" {
				cfncrchoutput, err := CfClient.WrapperFunc(context, cloudResource.Spec.Cloudformation.Stackname)
				if err != nil {
					log.Info("Error", "Getting Events from Changeset: ", err)
					cfnResult.status = "Failed"
					cfnResult.requeue = false
					message := fmt.Sprintf("Error Getting Events from Changeset: ")
					log.Info(message)
					err = errors.New(message)
					cfnResult.messages = append(cfnResult.messages, message)
					cfnResult.errors = append(cfnResult.errors, err)
				}

				log.Info("Create changeset state during this poll is: ", "status", cfncrchoutput.String())
				w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Status", cfncrchoutput.String())

				if cmp.Equal(cfncrchoutput.Status, aws.String("CREATE_COMPLETE")) {
					cfnResult.status = "CREATE_COMPLETE"
					cfnResult.exech = true
					cfnResult.requeue = false
					log.Info("Stack ready for getting executed..")
				} else if cmp.Equal(cfncrchoutput.Status, aws.String("CREATE_PENDING")) {
					cfnResult.status = "CREATE_PENDING"
					cfnResult.exech = false
					cfnResult.requeue = true
					log.Info("Changeset in pending state..")
				} else if cmp.Equal(cfncrchoutput.Status, aws.String("CREATE_IN_PROGRESS")) {
					cfnResult.status = "CREATE_IN_PROGRESS"
					cfnResult.exech = false
					cfnResult.requeue = true
					log.Info("Changeset in progress..")
				} else if cmp.Equal(cfncrchoutput.Status, aws.String("FAILED")) {
					log.Info("Changeset creation unsuccessful")
					cfnResult.status = "Failed"
					cfnResult.requeue = false
					message := fmt.Sprintf("Changeset creation unsuccessful")
					log.Info(message)
					err = errors.New(message)
					cfnResult.messages = append(cfnResult.messages, message)
					cfnResult.errors = append(cfnResult.errors, err)
				}
			} else if cloudResource.Status.Status == "InProgress" {
				cfnResult.rollbackComplete = IsRollBackComplete(*stacks[0].StackStatus)
			}
		} else if IsInProgress(*stacks[0].StackStatus) {
			/*log.Info("Time in CR", "Time:", cloudResource.Status.StartedAt.Time)
			if time.Since(cloudResource.Status.StartedAt.Time).Minutes() > TOTAL_TIME_POLLING_IN_MINUTES {
				message := fmt.Sprintf("Stack Execution Timedout for resource=%s", cloudResource.ObjectMeta.Name)
				log.Info(message)
				cfnResult.status = "Failed"
				err = errors.New(message)
				cfnResult.messages = append(cfnResult.messages, message)
				cfnResult.errors = append(cfnResult.errors, err)
			}*/
			cfnResult.stackId = gjson.Get(Response, "PhysicalResourceId").String()
			cfnResult.status = "InProgress"
			cfnResult.requeue = true
			if IsReviewInProgress(*stacks[0].StackStatus) {
				cfnResult.status = "ReviewInProgress"
				cfncrchoutput, err := CfClient.WrapperFunc(context, cloudResource.Spec.Cloudformation.Stackname)
				if err != nil {
					log.Info("Error", "Getting Events from Changeset: ", err)
					cfnResult.status = "Failed"
					cfnResult.requeue = false
					message := fmt.Sprintf("Error Getting Events from Changeset: ")
					log.Info(message)
					err = errors.New(message)
					cfnResult.messages = append(cfnResult.messages, message)
					cfnResult.errors = append(cfnResult.errors, err)
				}

				//if cfncrchoutput.ChangeSetName != nil {
				//	cfnResult.latestChangeSetName = aws.StringValue(cfncrchoutput.ChangeSetName)
				//}

				log.Info("Create changeset state during this poll is: ", "status", cfncrchoutput.String())
				w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Status", cfncrchoutput.String())
				if cmp.Equal(cfncrchoutput.Status, aws.String("CREATE_COMPLETE")) {
					cfnResult.status = "CREATE_COMPLETE"
					cfnResult.exech = true
					cfnResult.requeue = false
					log.Info("Stack ready for getting executed..")
				} else if cmp.Equal(cfncrchoutput.Status, aws.String("FAILED")) {
					log.Info("Changeset creation unsuccessful")
					cfnResult.status = "Failed"
					cfnResult.requeue = false
					message := fmt.Sprintf("Changeset creation unsuccessful")
					log.Info(message)
					err = errors.New(message)
					cfnResult.messages = append(cfnResult.messages, message)
					cfnResult.errors = append(cfnResult.errors, err)
				}
			}
		} else if IsFailed(*stacks[0].StackStatus) {
			message := fmt.Sprintf("Stack is in Failed status for resource=%s with state=%s", cloudResource.ObjectMeta.Name, pvstatus)
			log.Info(message)
			cfnResult.status = "Failed"
			err = errors.New(message)
			cfnResult.messages = append(cfnResult.messages, message)
			cfnResult.errors = append(cfnResult.errors, err)
		}
	}

	return cfnResult, awscfnparams

}

//func (w *CloudResourceDeploymentReconciler) validateForDeleteIfStackExistsWithSameName(context context.Context, cloudResource *cloudresourcev1alpha1.CloudResourceDeployment) *CfnValidationResult {
//	log := w.Log
//
//	cfnResult := w.newCfnValidationResult()
//
//	describeStackOutput, err := w.CfClient.DescribeCloudformationStacks(context, cloudResource.Spec.Cloudformation.Stackname)
//	if err != nil {
//		if awserr, ok := err.(awserr.Error); ok {
//			if awserr.Code() == AWS_VALIDATION_ERROR {
//				log.Info("No stack exist for this particular resource, so we're proceeding with deleting CR")
//				//if awserr.Message() == "Stack ["+cloudResource.Spec.Cloudformation.Stackname+"] does not exist" {
//				if awserr.Message() == "Stack with id "+cloudResource.Spec.Cloudformation.Stackname+" does not exist" {
//					log.Info("Stack was deleted")
//					message := "Cloudformation Stack was deleted"
//					cfnResult.messages = append(cfnResult.messages, message)
//					cfnResult.exists = false
//					cfnResult.status = "Deleted"
//					return cfnResult
//				}
//			}
//		}
//		message := "Error while retrieving stack information using cloudformation client for resource"
//		log.Error(err, message, "resource", cloudResource.ObjectMeta.Name)
//		cfnResult.messages = append(cfnResult.messages, message)
//		cfnResult.errors = append(cfnResult.errors, err)
//		cfnResult.result = false
//		cfnResult.exists = false
//		cfnResult.status = "Failed"
//		return cfnResult
//	}
//
//	stacks := describeStackOutput.Stacks
//	//awscfnparams := describeStackOutput.Stacks[0].Parameters
//	//if there is stack with same name in Cloudformation then return false
//	// if the status is REVIEW_IN_PROGRESS,we can go ahead
//	if *stacks[0].StackName == cloudResource.Spec.Cloudformation.Stackname {
//		cfnResult.exists = true
//		EventResponse, err := w.CfClient.PostValidationFunc(context, cloudResource.Spec.Cloudformation.Stackname)
//		if err != nil {
//			if awserr, ok := err.(awserr.Error); ok {
//				if awserr.Code() == AWS_VALIDATION_ERROR {
//					log.Info("No stack exist for this particular resource, so we're proceeding with deleting CR")
//					if awserr.Message() == "Stack with id "+cloudResource.Spec.Cloudformation.Stackname+" does not exist" {
//						log.Info("Stack was deleted")
//						message := "Cloudformation Stack was deleted"
//						cfnResult.messages = append(cfnResult.messages, message)
//						cfnResult.exists = false
//						cfnResult.status = "Deleted"
//						return cfnResult
//					}
//				}
//			}
//			message := "Error while retrieving stack information using cloudformation client for resource"
//			log.Error(err, message, "resource", cloudResource.ObjectMeta.Name)
//			cfnResult.messages = append(cfnResult.messages, message)
//			cfnResult.errors = append(cfnResult.errors, err)
//			cfnResult.result = false
//			cfnResult.exists = false
//			cfnResult.status = "Failed"
//
//			return cfnResult
//		}
//		resJson, err := json.Marshal(EventResponse.StackEvents[0])
//		if err != nil {
//			log.Error(err, "Unable to convert Event response to JSON , so skip Info logging ", "StackName", cloudResource.Spec.Cloudformation.Stackname)
//		} else {
//			log.Info("Event Response from the Session is successful", "stackEvents", string(resJson))
//		}
//		Response := string(resJson)
//		log.Info("Latest StackEvent", "Event:", Response)
//		w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Status", Response)
//		pvstatus := gjson.Get(Response, "ResourceStatus").String()
//		log.Info("Status from PostValidation", "is:", pvstatus)
//		if IsComplete(*stacks[0].StackStatus) {
//			cfnResult.stackId = gjson.Get(Response, "PhysicalResourceId").String()
//			//successfulTerminalState := true
//			cfnResult.status = "Complete"
//			message := fmt.Sprintf("Stack Execution Completed successfully for resource=%s", cloudResource.ObjectMeta.Name)
//			log.Info(message)
//			//w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Normal", message)
//			cfnResult.messages = append(cfnResult.messages, message)
//		} else if IsInProgress(*stacks[0].StackStatus) {
//			//log.Info("Time in CR", "Time:", cloudResource.Status.StartedAt)
//			/*if time.Since(cloudResource.Status.StartedAt.Time).Minutes() > TOTAL_TIME_POLLING_IN_MINUTES {
//				message := fmt.Sprintf("Stack Execution Timedout for resource=%s", cloudResource.ObjectMeta.Name)
//				log.Info(message)
//				cfnResult.status = "Failed"
//				err = errors.New(message)
//				cfnResult.messages = append(cfnResult.messages, message)
//				cfnResult.errors = append(cfnResult.errors, err)
//			}*/
//			cfnResult.stackId = gjson.Get(Response, "PhysicalResourceId").String()
//			cfnResult.status = "InProgress"
//			cfnResult.requeue = true
//			if IsReviewInProgress(*stacks[0].StackStatus) {
//				cfnResult.status = "ReviewInProgress"
//				cfncrchoutput, err := w.CfClient.WrapperFunc(context, cloudResource.Spec.Cloudformation.Stackname)
//				if err != nil {
//					log.Info("Error", "Getting Events from Changeset: ", err)
//					cfnResult.status = "Failed"
//					cfnResult.requeue = false
//					message := fmt.Sprintf("Error Getting Events from Changeset: ")
//					log.Info(message)
//					err = errors.New(message)
//					cfnResult.messages = append(cfnResult.messages, message)
//					cfnResult.errors = append(cfnResult.errors, err)
//				}
//				log.Info("Create changeset state during this poll is: ", "status", cfncrchoutput.String())
//				w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Status", cfncrchoutput.String())
//				if cmp.Equal(cfncrchoutput.Status, aws.String("CREATE_COMPLETE")) {
//					cfnResult.status = "CREATE_COMPLETE"
//					cfnResult.exech = true
//					cfnResult.requeue = false
//					log.Info("Stack ready for getting executed..")
//				} else if cmp.Equal(cfncrchoutput.Status, aws.String("FAILED")) {
//					log.Info("Changeset creation unsuccessful")
//					cfnResult.status = "Failed"
//					cfnResult.requeue = false
//					message := fmt.Sprintf("Changeset creation unsuccessful")
//					log.Info(message)
//					err = errors.New(message)
//					cfnResult.messages = append(cfnResult.messages, message)
//					cfnResult.errors = append(cfnResult.errors, err)
//				}
//			}
//		} else if IsFailed(*stacks[0].StackStatus) {
//			message := fmt.Sprintf("Stack is in Failed status for resource=%s with state=%s", cloudResource.ObjectMeta.Name, pvstatus)
//			log.Info(message)
//			cfnResult.status = "Failed"
//			err = errors.New(message)
//			cfnResult.messages = append(cfnResult.messages, message)
//			cfnResult.errors = append(cfnResult.errors, err)
//		}
//	}
//
//	return cfnResult
//}

//Create Validation template result
func (w *CloudResourceDeploymentReconciler) newCfnValidationResult() *CfnValidationResult {
	var messages []string
	var errors []error

	//Initial value is true, will be updated by validators to false(if there is any)
	return &CfnValidationResult{
		result:   true,
		errors:   errors,
		messages: messages,
		exists:   false,
		exech:    false,
		requeue:  false,
		rollbackComplete: false,
	}
}

func IsInProgress(category string) bool {
	switch category {
	case
		"CREATE_IN_PROGRESS",
		"ROLLBACK_IN_PROGRESS",
		"DELETE_IN_PROGRESS",
		"UPDATE_IN_PROGRESS",
		"UPDATE_ROLLBACK_IN_PROGRESS",
		"IMPORT_IN_PROGRESS",
		"REVIEW_IN_PROGRESS",
		"IMPORT_ROLLBACK_IN_PROGRESS",
		"UPDATE_COMPLETE_CLEANUP_IN_PROGRESS",
		"UPDATE_ROLLBACK_COMPLETE_CLEANUP_IN_PROGRESS":
		return true
	}
	return false
}

func IsFailed(category string) bool {
	switch category {
	case
		"IMPORT_ROLLBACK_FAILED",
		"UPDATE_ROLLBACK_FAILED",
		"DELETE_FAILED",
		"ROLLBACK_FAILED",
		"CREATE_FAILED",
		"IMPORT_ROLLBACK_COMPLETE":
		return true
	}
	return false
}

func IsComplete(category string) bool {
	switch category {
	case
		"CREATE_COMPLETE",
		"DELETE_COMPLETE",
		"UPDATE_COMPLETE",
		"UPDATE_ROLLBACK_COMPLETE",
		"ROLLBACK_COMPLETE",
		"IMPORT_COMPLETE":
		return true
	}
	return false
}

func IsRollBackComplete(category string) bool {
	switch category {
	case
		"UPDATE_ROLLBACK_COMPLETE",
		"ROLLBACK_COMPLETE":
		return true
	}
	return false
}

func IsReviewInProgress(category string) bool {
	switch category {
	case
		"REVIEW_IN_PROGRESS":
		return true
	}
	return false
}

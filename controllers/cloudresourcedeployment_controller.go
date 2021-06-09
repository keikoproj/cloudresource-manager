/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	cloudresourcev1alpha1 "github.com/keikoproj/cloudresource-manager/api/v1alpha1"
	"github.com/keikoproj/cloudresource-manager/metrics"
	awsapi "github.com/keikoproj/cloudresource-manager/pkg/awsapi"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"
	"time"
)

type ControllerEventState int

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}

const (
	requestId = "request_id"

	//This role is used for dynamically constructing roleArn.
	//Role need to be present in the target account before CR execution
	cfnMasterRole = "cfn-master-role"

	//This role is used for dynamically constructing role session name.
	//Role need to be present in the target account before CR execution
	assumeSession = "assumesession"

	//This role is used for dynamically attaching serviceRoleArn to Cloudformation stack.
	//ServiceRole need to be present in the target account before CR execution
	cfnserviceRole = "clf-service-role"
)

// CfnReconciler reconciles a Cfn object
type CloudResourceDeploymentReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	//CfClient    *awsapi.CFN
	MaxParallel int
}

// NewCloudResourceDeploymentReconciler returns an instance of HealthCheckReconciler
func NewCloudResourceDeploymentReconciler(mgr manager.Manager, log logr.Logger, MaxParallel int, region string) *CloudResourceDeploymentReconciler {
	return &CloudResourceDeploymentReconciler{
		Client:   mgr.GetClient(),
		Log:      log,
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("CloudResourceDeployment"),
		//CfClient:    awsapi.GetCloudformationClient(region),
		MaxParallel: MaxParallel,
	}
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch;
// +kubebuilder:rbac:groups=cloudresource.keikoproj.io,resources=cloudresourcedeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudresource.keikoproj.io,resources=cloudresourcedeployments/status,verbs=get;update;patch
func (w *CloudResourceDeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	log := w.Log.WithValues("cloudResource", req.NamespacedName)
	CfClient := &awsapi.CFN{}

	//  sleep is introduced here so that in a status update when the reconcile is called again it will wait for the status to be reflected in the CR
	time.Sleep(5 * time.Second)
	cloudResource := &cloudresourcev1alpha1.CloudResourceDeployment{}
	if err := w.Get(context.Background(), req.NamespacedName, cloudResource); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("cloudResource document not found ", "Resource:", req.Name)
			log.Info("reconcile completed")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get CloudResource, reconcile failed")
		return ctrl.Result{}, err
	}

	context := context.WithValue(context.Background(), requestId, cloudResource.ObjectMeta.UID)
	log = w.Log.WithValues("cloudResource", req.NamespacedName, "uid:", cloudResource.ObjectMeta.UID)
	//Handles the panic inside the controller execution
	defer func() {
		if err := recover(); err != nil {
			log.Info("Panic occured during controller reconcilation", "stackname", cloudResource.Spec.Cloudformation.Stackname, "namespace", cloudResource.ObjectMeta.Namespace, "Error:", err)
		}
	}()

	reconcileStatus := w.ValidateORUpdateCheckSum(cloudResource)
	switch reconcileStatus {
	case "NotUpdated":
		return ctrl.Result{}, nil
	case "Updated":
		log.Info("NewChecksum is set for CR", "checksum:", cloudResource.Status.Checksum)
	}

	/*timeoutInSpec := cloudResource.Spec.Cloudformation.Timeout
	if timeoutInSpec > 0 {
		TOTAL_TIME_POLLING_IN_MINUTES = float64(timeoutInSpec)
	}*/

	// name of our custom finalizer
	//myFinalizerName := "cloudresource.keikoproj.io"
	_, err := w.SetAssumeRoleProvider(context, cloudResource)
	if err != nil {
		log.Info("AssumeRoleProvider Values are not set successfully")
		return ctrl.Result{}, nil
	}

	CfClient, err = CfClient.GetTargetAccountSession(context, cloudResource)
	if err != nil {
		w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Error Assuming Role Please Check RoleARN, AccountId, RoleSessionName and Externalid")
		log.Error(err, "Error Assuming Role")
		return ctrl.Result{}, nil
	}

	log.Info("AWS Stack Session is obtained")

	// if any reference resources are to be fetched from the reference stack, extract them and update parameter value
	if cloudResource.Spec.Cloudformation.Parameters != nil && len(cloudResource.Spec.Cloudformation.Parameters) > 0 {
		referenceResources, err := CfClient.FetchReferenceResources(context, cloudResource.Spec.Cloudformation.Parameters)
		if err != nil {
			cloudResource.Status.Message = "Error while fetching the values for reference resources"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Failed Fetching Reference Resources")

			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				return ctrl.Result{}, err
			}
		}
		if referenceResources != nil && len(referenceResources) > 0 {
			for _, key := range referenceResources {
				cloudResource.Spec.Cloudformation.Parameters[*key.ParameterKey] = *key.ParameterValue
			}
			log.Info("Successfully fetched the values for reference resources and updated them in the parameters..")
		}
	}

	cfnResult, awscfnparams := w.validateIfStackExistsWithSameName(context, cloudResource, *CfClient)
	log.Info("Status from validations", "cfnResult.status:", cfnResult.status, "cfnResult.requeque", cfnResult.requeue, "cfnResult.result", cfnResult.result, "cfnResult.exists:", cfnResult.exists, "cfnResult.exech", cfnResult.exech, "cfnResult.messages:", cfnResult.messages)

	_, err = w.CheckForRequeue(context, cloudResource, cfnResult, awscfnparams, *CfClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

}

func (r *CloudResourceDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudresourcev1alpha1.CloudResourceDeployment{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxParallel}).
		Complete(r)
}

//Helper function to dynamically get the Role if absent
func getRoleArn(accountId string) (string, error) {
	accountId = strings.TrimSpace(accountId)
	if len(accountId) == 0 {
		return "", fmt.Errorf("AccountId field empty")
	}

	var roleArn string
	roleArn = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, cfnMasterRole)

	return roleArn, nil
}

//Helper function to dynamically default role session name if absent
func getRoleSessionName() string {
	uuid := uuid.New()
	return fmt.Sprintf("%s-%s", uuid.String(), assumeSession)
}

//Helper function to dynamically get the Role if absent
func getServiceRoleArn(accountId string) (string, error) {
	accountId = strings.TrimSpace(accountId)
	if len(accountId) == 0 {
		return "", fmt.Errorf("AccountId field empty")
	}

	var roleArn string
	roleArn = fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, cfnserviceRole)

	return roleArn, nil
}

//Helper function to get accountid deom roleArn
func getAccountidFromRoleArn(role string) (string, error) {
	role = strings.TrimSpace(role)
	if len(role) == 0 {
		return "", fmt.Errorf("role field empty")
	}

	var acct string
	res := strings.Split(role, "::")
	res1 := strings.Split(res[1], ":")
	acct = res1[0]

	return acct, nil
}

func IsValidState(category string) bool {
	switch category {
	case
		SUCCESS_MESSAGE,
		FAILURE_MESSAGE:
		return true
	}
	return false
}

func (w *CloudResourceDeploymentReconciler) ValidateORUpdateCheckSum(cloudResource *cloudresourcev1alpha1.CloudResourceDeployment) string {
	log := w.Log
	newChecksum := cloudResource.CalculateChecksum()
	log.Info("Checksum generated for current spec:", "CheckSum:", newChecksum, "CRChecksum:", cloudResource.Status.Checksum)
	if cloudResource.Status.Checksum == newChecksum {
		if IsValidState(cloudResource.Status.Status) && cloudResource.ObjectMeta.DeletionTimestamp.IsZero() {
			log.Info("This Spec was already run so not running it again as checksum matches", "Checksum", newChecksum)
			return "NotUpdated"
		}
	}

	cloudResource.Status.Checksum = newChecksum
	log.Info("NewChecksum is set for CR", "checksum:", newChecksum)
	log.Info("Status of CloudResource is:", "status:", cloudResource.Status.Status, "Message:", cloudResource.Status.Message)
	return "Updated"
}

func (w *CloudResourceDeploymentReconciler) SetAssumeRoleProvider(context context.Context, cloudResource *cloudresourcev1alpha1.CloudResourceDeployment) (bool, error) {
	log := w.Log
	var err error

	roleARN := cloudResource.Spec.Cloudformation.CARole.RoleARN
	rsn := cloudResource.Spec.Cloudformation.CARole.RoleSessionName
	eid := cloudResource.Spec.Cloudformation.CARole.ExternalID
	acctid := cloudResource.Spec.Cloudformation.CARole.AccountID
	svcRoleARN := cloudResource.Spec.Cloudformation.CARole.ServiceRoleARN
	env := cloudResource.Spec.Cloudformation.CARole.Environment
	region := cloudResource.Spec.Cloudformation.Region

	if len(region) == 0 {
		region, err = awsapi.GetRegion()
		if err != nil {
			log.Error(err, "failed to detect region")
			return false, err
		}
		log.Info("Region is set", "region:", region)
		cloudResource.Spec.Cloudformation.Region = region
	}

	if len(roleARN) == 0 {
		roleARN, err = getRoleArn(acctid)
		if err != nil {
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Error Assuming Role, AccountId is null")
			log.Error(err, "Error Assuming Role")
			//return ctrl.Result{}, nil
			cloudResource.Status.Message = "Error while fetching the RoleARN for Target Account"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Failed Fetching RoleARN for Target Account")

			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				//return ctrl.Result{}, err
				return false, err
			}
		}
		cloudResource.Spec.Cloudformation.CARole.RoleARN = roleARN
	}
	if len(rsn) == 0 {
		rsn = getRoleSessionName()
	}

	if len(acctid) == 0 {
		acctid, err = getAccountidFromRoleArn(roleARN)
		if err != nil {
			log.Error(err, "Error Getting AccountId from roleARN")
			cloudResource.Status.Message = "Error Getting AccountId from roleARN"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Error Getting AccountId from roleARN")

			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				return false, err
			}
			//return ctrl.Result{}, nil
		}
		cloudResource.Spec.Cloudformation.CARole.AccountID = acctid
	}

	if len(svcRoleARN) == 0 {
		svcRoleARN, err = getServiceRoleArn(acctid)
		if err != nil {
			log.Error(err, "Error getting ServiceRoleArn")
			cloudResource.Status.Message = "Error getting ServiceRoleArn"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Error getting ServiceRoleArn")

			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				//return ctrl.Result{}, err
				return false, err
			}
			//return ctrl.Result{}, nil
		}
		cloudResource.Spec.Cloudformation.CARole.ServiceRoleARN = svcRoleARN
	}

	if len(eid) == 0 {
		eid = "cloudresource-" + acctid + "-" + env
		cloudResource.Spec.Cloudformation.CARole.ExternalID = eid
	}
	return true, nil
}

func (w *CloudResourceDeploymentReconciler) CheckForRequeue(context context.Context, cloudResource *cloudresourcev1alpha1.CloudResourceDeployment, cfnResult *CfnValidationResult, awscfnparams []*cloudformation.Parameter, CfClient awsapi.CFN) (bool, error) {

	log := w.Log
	if cfnResult.requeue {
		return true, nil
	}

	if cfnResult.exists && cfnResult.status == "Failed" {
		w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Cloudformation Stack Failed")
		cloudResource.Status.Message = fmt.Sprintf("Failed Creating Cloudformation Stack: %s, reason: %s", cloudResource.Spec.Cloudformation.Stackname, cfnResult.messages)
		cloudResource.Status.Status = FAILURE_MESSAGE
		metrics.MonitorFailedCount.With(prometheus.Labels{"account_number": cloudResource.Spec.Cloudformation.CARole.AccountID, "cloudResource_name": cloudResource.Spec.Cloudformation.Stackname, "operation": "Create/Update"}).Inc()
		if err := w.Update(context, cloudResource); err != nil {
			log.Info("Error", "syncing status error: ", err)
			return false, err
		}
		return false, errors.New("Cloudformation Stack Failed")
	}

	if cfnResult.exists && cfnResult.status == "Complete" {
		awscfntemplate, err := CfClient.GetTemplate(context, cloudResource.Spec.Cloudformation.Stackname, cfnResult.latestChangeSetName)
		if err != nil {
			log.Error(err, "Error Getting Template to validate CR")
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Error Getting Template to validate CR")
			metrics.MonitorFailedCount.With(prometheus.Labels{"account_number": cloudResource.Spec.Cloudformation.CARole.AccountID, "cloudResource_name": cloudResource.Spec.Cloudformation.Stackname, "operation": "Create/Update"}).Inc()
			cloudResource.Status.Message = "Error Getting Template to validate CR"
			cloudResource.Status.Status = FAILURE_MESSAGE
			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				return false, err
			}
		}

		if *awscfntemplate.TemplateBody == cloudResource.Spec.Cloudformation.Template {
			log.Info("Template Body in the AWS cloudformation Stack and CloudResource CR are same", "ISTEMPLATEBODYSAME:", "True")

			crparams, err := CfClient.StackParameters(context, cloudResource.Spec.Cloudformation.Parameters)
			if err != nil {
				log.Error(err, "Error while getting Params")
				return false, err
			}
			log.Info("Cloudformation Parameters from CR Spec", "CR-Parameters:", crparams)
			log.Info("Cloudformation Parameters from AWS", "AWS-Parameters:", awscfnparams)
			eq := CfClient.CompareStackParams(context, awscfnparams, crparams)
			if eq {
				log.Info("Cloudformation parameters and CR Parameters are equal.", "ISPARAMETERSSAME:", eq)
				log.Info("Cloudformation Stack with the same StackName, Template and Parameters created/exists", "Reconcilation?", "No Not Reconciling")
				w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Complete", "Cloudformation Stack with the StackName, Template and Parameters created/exists")
				cloudResource.Status.Message = fmt.Sprintf("Stack id: %s", cfnResult.stackId)
				cloudResource.Status.Status = SUCCESS_MESSAGE
				metrics.MonitorSuccessCount.With(prometheus.Labels{"account_number": cloudResource.Spec.Cloudformation.CARole.AccountID, "cloudResource_name": cloudResource.Spec.Cloudformation.Stackname, "operation": "Create/Update"}).Inc()
				if err := w.Update(context, cloudResource); err != nil {
					log.Info("Error", "syncing status error: ", err)
					return false, err
				}
				return true, nil
			} else {
				log.Info("Cloudformation parameters and CR Parameters are not equal.", "ISPARAMETERSSAME:", eq)
				log.Info("Cloudformation Stack with the same StackName, Template and Parameters Doesnot exist", "Reconcilation?", "Yes Reconciling")
			}
		}
		log.Info("Template Body in the AWS cloudformation Stack and CloudResource CR are not same", "ISCLOUDRESOURCESAME:", "False")
		log.Info("Cloudformation Stack with the same StackName, Template and Parameters Doesnot exist", "Reconcilation?", "Yes Reconciling")
	}

	log.Info("status of  cfnResult.rollbackComplete is", "rollbackComplete:",cfnResult.rollbackComplete)

	if cfnResult.rollbackComplete {
		cloudResource.Status.Message = fmt.Sprintf("Stack id: %s", cfnResult.stackId)
		cloudResource.Status.Status = SUCCESS_MESSAGE
		log.Info("Stop Reconciling as Stack reached one of RollBack_Complete states")
		if err := w.Update(context, cloudResource); err != nil {
			log.Info("Error", "syncing status error: ", err)
			return false, err
		}
		return true, nil
	}

	if cfnResult.exists && !cfnResult.exech {
		createChangesetStartTime := time.Now()
		cloudResource.Status.StartedAt = &metav1.Time{Time: createChangesetStartTime}
		id := uuid.New()
		changeSetName := cloudResource.Spec.Cloudformation.Stackname + "-changeset-" + id.String()
		cloudResource.Status.Changesetname = changeSetName
		w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Creating", "Creating Changeset")
		_, err := CfClient.CreateChangeSet(context, changeSetName, cloudResource.Spec.Cloudformation.Stackname, cloudResource.Spec.Cloudformation.Template, cloudResource.Spec.Cloudformation.Parameters, cloudResource.Spec.Cloudformation.Tags, cloudResource.Spec.Cloudformation.CARole.ServiceRoleARN, "UPDATE")

		if err != nil {
			cloudResource.Status.Message = "Error while creating change set"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Failed Creating Changeset")
			metrics.MonitorFailedCount.With(prometheus.Labels{"account_number": cloudResource.Spec.Cloudformation.CARole.AccountID, "cloudResource_name": cloudResource.Spec.Cloudformation.Stackname, "operation": "Create/Update"}).Inc()
			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				return false, err
			}
			return false, err
		}
		cloudResource.Status.Message = "UpdateChangeset InProgress"
		cloudResource.Status.Status = "Update InProgress"
		// this update is to update startedat in status.
		if err := w.Update(context, cloudResource); err != nil {
			log.Info("Error", "syncing status error: ", err)
			return false, err
		}
	} else if !cfnResult.exists && !cfnResult.exech {
		createChangesetStartTime := time.Now()
		cloudResource.Status.StartedAt = &metav1.Time{Time: createChangesetStartTime}
		id := uuid.New()
		changeSetName := cloudResource.Spec.Cloudformation.Stackname + "-changeset-" + id.String()
		cloudResource.Status.Changesetname = changeSetName
		w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Normal", "Creating Changeset")
		_, err := CfClient.CreateChangeSet(context, changeSetName, cloudResource.Spec.Cloudformation.Stackname, cloudResource.Spec.Cloudformation.Template, cloudResource.Spec.Cloudformation.Parameters, cloudResource.Spec.Cloudformation.Tags, cloudResource.Spec.Cloudformation.CARole.ServiceRoleARN, "CREATE")
		if err != nil {
			cloudResource.Status.Message = "Error while creating change set"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Failed Creating Changeset")
			metrics.MonitorFailedCount.With(prometheus.Labels{"account_number": cloudResource.Spec.Cloudformation.CARole.AccountID, "cloudResource_name": cloudResource.Spec.Cloudformation.Stackname, "operation": "Create/Update"}).Inc()
			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				return false, err
			}
			return false, err
		}
		cloudResource.Status.Message = "CreateChangeset InProgress"
		cloudResource.Status.Status = "InProgress"
		// this update is to update startedat in status.
		if err := w.Update(context, cloudResource); err != nil {
			log.Info("Error", "syncing status error: ", err)
			return false, err
		}
	} else if cfnResult.exists && cfnResult.exech {
		executeChangesetStartTime := time.Now()
		cloudResource.Status.StartedAt = &metav1.Time{Time: executeChangesetStartTime}
		_, err := CfClient.ExecuteChangeSet(context, cloudResource.Spec.Cloudformation.Stackname, cloudResource.Status.Changesetname)
		if err != nil {
			cloudResource.Status.Message = "Error while executing change set"
			cloudResource.Status.Status = FAILURE_MESSAGE
			w.Recorder.Event(cloudResource, v1.EventTypeWarning, "Failed", "Executing Cloudformation Changeset")
			metrics.MonitorFailedCount.With(prometheus.Labels{"account_number": cloudResource.Spec.Cloudformation.CARole.AccountID, "cloudResource_name": cloudResource.Spec.Cloudformation.Stackname, "operation": "Create/Update"}).Inc()
			if err := w.Update(context, cloudResource); err != nil {
				log.Info("Error", "syncing status error: ", err)
				return false, err
			}
			return false, err
		}
		cloudResource.Status.Message = "ExecuteChangeset InProgress"
		cloudResource.Status.Status = "InProgress"

		// this update is to update startedat in status.
		if err := w.Update(context, cloudResource); err != nil {
			log.Info("Error", "syncing status error: ", err)
			return false, err
		}
		w.Recorder.Event(cloudResource, v1.EventTypeNormal, "Normal", "Executing Changeset")
	}
	return true, nil
}
